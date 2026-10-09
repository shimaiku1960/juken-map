// Package authguard はログインの守り（回数の制限・メールの送信の上限・外部ログインの途中の state）への
// 書き込みの持ち主。表は AuthThrottle・EmailSend・AuthOAuthState。持ち主の一覧と決まりは docs/architecture.md
// 「バックエンドの構成」（JUK-148・JUK-150）。
//
//   - このファイル：回数の制限（認証基準 10 の H1、06 の B4）
//   - email.go：メールの送信の上限（06 E1）
//   - oauthstate.go：外部ログインの state（F1・F2）
//
// どれもアカウント（internal/write/account）の表と一緒に変える操作は無い。
package authguard

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"strings"
	"time"
)

// 回数の制限。推測できる入口ごとに、何を単位に何回まで受けるかを決めて数える。
//
// 数は DB（AuthThrottle）に置く。デプロイの入れ替えでコンテナが2つ並ぶあいだも、同じ数を見るため。
// 数えるのは失敗ではなく試行。失敗を応答のあとで数えると、同時に投げられた試行が全部「まだ上限前」を
// 見て通ってしまう。試行を先に1つ足してから判定すれば、同時でも上限を超えた分は止まる。
// 成功したら、その単位の数を消す（Clear）。

// Rule は1つの入口の決まり。
type Rule struct {
	Name   string // バケットの種類。対象と組み合わせて SHA-256 にする
	Max    int
	Window time.Duration
}

var (
	// SignInAccount はサインインのアカウント単位。IP を変えながら同じアカウントを狙う総当たりを止める。
	// 存在しないメールアドレスも同じように数える（止まるかどうかで登録の有無が分からないように）。
	// 窓が終わるまでは本人もパスワードでは入れないので、窓は短めにし、再設定と外部ログインはこれを通らない。
	SignInAccount = Rule{Name: "sign-in:account", Max: 10, Window: 15 * time.Minute}
	// SignInIP はサインインの IP 単位。いろいろなアカウントへ少しずつ試す（パスワードスプレー）のを止める。
	// Better Auth の既定（10 秒に 3 回）より窓を長くし、学校などで同じ IP を共有しても困らない数にする。
	SignInIP = Rule{Name: "sign-in:ip", Max: 30, Window: 10 * time.Minute}
	// MFAAccount は2段階認証のコードのアカウント単位。6 桁は 100 万通りしかない。途中の状態ごと（5 回）とは別に、
	// 途中の状態を作り直しながら試すのを止める。
	MFAAccount = Rule{Name: "mfa:account", Max: 10, Window: 15 * time.Minute}
	// ReauthAccount は再認証（2段階認証の設定・パスワードの変更で、今のパスワードを入れ直す）のアカウント単位。
	ReauthAccount = Rule{Name: "reauth:account", Max: 10, Window: 15 * time.Minute}
	// AnonymousIP はログインしていなくても呼べる入口の IP 単位：登録・確認メールの再送・再設定の依頼・メールのトークンの使用。
	// メールの送信そのものの上限（宛先ごと・全体）は email.go が別に数える（06 E1）。
	AnonymousIP = Rule{Name: "anonymous:ip", Max: 20, Window: 10 * time.Minute}
)

// longestWindow は上の決まりのうち、いちばん長い窓。これより古い行は、どの決まりでも窓が終わっている。
const longestWindow = 15 * time.Minute

// Hit は試行を1つ数え、上限の内なら allowed=true を返す。止めるときは何秒後に窓が終わるかも返す。
func Hit(ctx context.Context, db *sql.DB, rule Rule, subject string, now time.Time) (allowed bool, retryAfter time.Duration, err error) {
	now = now.UTC().Truncate(time.Millisecond)
	cutoff := now.Add(-rule.Window)
	bucket := Bucket(rule, subject)

	// 窓が終わった行は、ここで少しずつ消す（行が溜まり続けないように）。日時の範囲で DELETE すると
	// 索引の隙間までロックし、同時に来た INSERT とぶつかりうるので、ロックを取らない SELECT で主キーを拾う。
	// 窓は種類ごとに違うので、いちばん長い窓より古いものだけを消す。
	if err := sweepThrottle(ctx, db, now.Add(-longestWindow)); err != nil {
		return false, 0, err
	}

	// 足すのと窓の切り替えを1文で行う。MySQL は SET を左から順に評価するので、count の IF は更新前の
	// windowStartedAt を見る（順番を入れ替えると壊れる）。足したあとの数は、別の SELECT ではなく
	// LAST_INSERT_ID(式) でこの文の応答から受け取る（読み直すと、同時に走った試行の分まで数えてしまう）。
	// 新しく行を作ったときは UPDATE 側を通らず、LastInsertId は 0（表に自動の番号が無いため）なので、数は 1。
	res, err := db.ExecContext(ctx,
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
	if count <= int64(rule.Max) {
		return true, 0, nil
	}

	var startedRaw string
	if err := db.QueryRowContext(ctx, "SELECT windowStartedAt FROM AuthThrottle WHERE bucket = ?", bucket).Scan(&startedRaw); err != nil {
		return false, 0, err
	}
	started, err := time.ParseInLocation("2006-01-02 15:04:05.999999", startedRaw, time.UTC)
	if err != nil {
		return false, 0, err
	}
	return false, max(time.Second, started.Add(rule.Window).Sub(now)), nil
}

// Clear は成功したときに、その単位の数を消す。
func Clear(ctx context.Context, db *sql.DB, rule Rule, subject string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM AuthThrottle WHERE bucket = ?", Bucket(rule, subject))
	return err
}

// Bucket は「種類:対象」の SHA-256。メールアドレスや IP をそのまま DB に残さない。
// メールアドレスは大文字・小文字と前後の空白をそろえる（同じアカウントを別の数え方にさせない）。
func Bucket(rule Rule, subject string) []byte {
	sum := sha256.Sum256([]byte(rule.Name + ":" + strings.ToLower(strings.TrimSpace(subject))))
	return sum[:]
}

func sweepThrottle(ctx context.Context, db *sql.DB, cutoff time.Time) error {
	rows, err := db.QueryContext(ctx, "SELECT bucket FROM AuthThrottle WHERE windowStartedAt <= ? LIMIT 100", cutoff)
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
		if _, err := db.ExecContext(ctx, "DELETE FROM AuthThrottle WHERE bucket = ? AND windowStartedAt <= ?", b, cutoff); err != nil {
			return err
		}
	}
	return nil
}
