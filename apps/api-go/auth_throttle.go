package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"time"
)

// 回数制限（認証基準 10 の H1、06 の B4）。推測できる入口ごとに、何を単位に何回まで受けるかを決めて数える。
//
// 数は DB（AuthThrottle）に置く。デプロイの入れ替えでコンテナが2つ並ぶあいだも、同じ数を見るため。
// 数えるのは失敗ではなく試行。失敗を応答のあとで数えると、同時に投げられた試行が全部「まだ上限前」を
// 見て通ってしまう。試行を先に1つ足してから判定すれば、同時でも上限を超えた分は止まる（Node の
// sign-in-throttle.ts と同じ考え方）。成功したら、その単位の数を消す（clear）。

// throttleRule は1つの入口の決まり。
type throttleRule struct {
	name   string // バケットの種類。対象と組み合わせて SHA-256 にする
	max    int
	window time.Duration
}

var (
	// サインインのアカウント単位。IP を変えながら同じアカウントを狙う総当たりを止める。
	// 存在しないメールアドレスも同じように数える（止まるかどうかで登録の有無が分からないように）。
	// 窓が終わるまでは本人もパスワードでは入れないので、窓は短めにし、再設定と外部ログインはこれを通らない。
	throttleSignInAccount = throttleRule{name: "sign-in:account", max: 10, window: 15 * time.Minute}
	// サインインの IP 単位。いろいろなアカウントへ少しずつ試す（パスワードスプレー）のを止める。
	// Better Auth の既定（10 秒に 3 回）より窓を長くし、学校などで同じ IP を共有しても困らない数にする。
	throttleSignInIP = throttleRule{name: "sign-in:ip", max: 30, window: 10 * time.Minute}
	// 2段階認証のコードのアカウント単位。6 桁は 100 万通りしかない。途中の状態ごと（5 回）とは別に、
	// 途中の状態を作り直しながら試すのを止める。
	throttleMFAAccount = throttleRule{name: "mfa:account", max: 10, window: 15 * time.Minute}
	// 再認証（2段階認証の設定・パスワードの変更で、今のパスワードを入れ直す）のアカウント単位。
	throttleReauthAccount = throttleRule{name: "reauth:account", max: 10, window: 15 * time.Minute}
	// ログインしていなくても呼べる入口の IP 単位：登録・確認メールの再送・再設定の依頼・メールのトークンの使用。
	// メールの送信そのものの上限（宛先ごと・全体）は auth_email.go が別に数える（06 E1）。
	throttleAnonymousIP = throttleRule{name: "anonymous:ip", max: 20, window: 10 * time.Minute}
)

// mfaChallengeMaxAttempts は、2段階認証の途中の状態1つで試せるコードの数。
const mfaChallengeMaxAttempts = 5

type throttle struct {
	db  *sql.DB
	now func() time.Time
}

// hit は試行を1つ数え、上限の内なら allowed=true を返す。止めるときは何秒後に窓が終わるかも返す。
func (t *throttle) hit(ctx context.Context, rule throttleRule, subject string) (allowed bool, retryAfter time.Duration, err error) {
	now := t.now().UTC().Truncate(time.Millisecond)
	cutoff := now.Add(-rule.window)
	bucket := throttleBucket(rule, subject)

	// 窓が終わった行は、ここで少しずつ消す（行が溜まり続けないように）。日時の範囲で DELETE すると
	// 索引の隙間までロックし、同時に来た INSERT とぶつかりうるので、ロックを取らない SELECT で主キーを拾う。
	// 窓は種類ごとに違うので、いちばん長い窓（15 分）より古いものだけを消す。
	if err := t.sweep(ctx, now.Add(-throttleLongestWindow)); err != nil {
		return false, 0, err
	}

	// 足すのと窓の切り替えを1文で行う。MySQL は SET を左から順に評価するので、count の IF は更新前の
	// windowStartedAt を見る（順番を入れ替えると壊れる）。足したあとの数は、別の SELECT ではなく
	// LAST_INSERT_ID(式) でこの文の応答から受け取る（読み直すと、同時に走った試行の分まで数えてしまう）。
	// 新しく行を作ったときは UPDATE 側を通らず、LastInsertId は 0（表に自動の番号が無いため）なので、数は 1。
	res, err := t.db.ExecContext(ctx,
		`INSERT INTO AuthThrottle (bucket, count, windowStartedAt) VALUES (?, 1, ?)
		 ON DUPLICATE KEY UPDATE
		   count = LAST_INSERT_ID(IF(windowStartedAt <= ?, 1, count + 1)),
		   windowStartedAt = IF(windowStartedAt <= ?, ?, windowStartedAt)`,
		bucket, now, cutoff, cutoff, now)
	if err != nil {
		return false, 0, err
	}
	count, err := res.LastInsertId()
	if err != nil {
		return false, 0, err
	}
	if count == 0 {
		count = 1
	}
	if count <= int64(rule.max) {
		return true, 0, nil
	}

	var startedRaw string
	if err := t.db.QueryRowContext(ctx, "SELECT windowStartedAt FROM AuthThrottle WHERE bucket = ?", bucket).Scan(&startedRaw); err != nil {
		return false, 0, err
	}
	started, err := parseDatetime(startedRaw)
	if err != nil {
		return false, 0, err
	}
	retryAfter = max(time.Second, started.Add(rule.window).Sub(now))
	return false, retryAfter, nil
}

// clear は成功したときに、その単位の数を消す。
func (t *throttle) clear(ctx context.Context, rule throttleRule, subject string) error {
	_, err := t.db.ExecContext(ctx, "DELETE FROM AuthThrottle WHERE bucket = ?", throttleBucket(rule, subject))
	return err
}

const throttleLongestWindow = 15 * time.Minute

func (t *throttle) sweep(ctx context.Context, cutoff time.Time) error {
	rows, err := t.db.QueryContext(ctx, "SELECT bucket FROM AuthThrottle WHERE windowStartedAt <= ? LIMIT 100", cutoff)
	if err != nil {
		return err
	}
	var expired [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// 主キーで1行ずつ消す。窓が終わった行が溜まるのは攻撃のあとくらいで、ふだんは 0〜数行。
	for _, b := range expired {
		if _, err := t.db.ExecContext(ctx, "DELETE FROM AuthThrottle WHERE bucket = ? AND windowStartedAt <= ?", b, cutoff); err != nil {
			return err
		}
	}
	return nil
}

// throttleBucket は「種類:対象」の SHA-256。メールアドレスや IP をそのまま DB に残さない。
// メールアドレスは大文字・小文字と前後の空白をそろえる（同じアカウントを別の数え方にさせない）。
func throttleBucket(rule throttleRule, subject string) []byte {
	sum := sha256.Sum256([]byte(rule.name + ":" + strings.ToLower(strings.TrimSpace(subject))))
	return sum[:]
}

// parseDatetime は DB の DATETIME(3)（接続は parseTime=false なので文字列で返る）を UTC の時刻にする。
func parseDatetime(raw string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02 15:04:05.999999", raw, time.UTC)
}
