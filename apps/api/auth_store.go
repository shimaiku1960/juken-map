package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
)

// authStore はログイン（/api/auth/*）の SQL をまとめる（JUK-134）。入口（auth_handlers.go・auth_mfa.go・
// auth_oauth.go・auth_recovery.go・auth_delete_account.go）は、リクエストを読み、ここを呼び、応答を書く。
// セッション・回数制限・メールの送信記録は、それぞれ sessionStore・throttle・authMailer が持つ。
//
// 1回だけ使えるもの（トークン・予備コード・2段階認証の途中の状態・TOTP のステップ）は、条件つきの DELETE か
// UPDATE で消せた・書けたときだけ使えたことにする。同じものが同時に2回送られても1回しか通らない。
type authStore struct {
	db  *sql.DB
	now func() time.Time
}

func (st *authStore) clock() time.Time {
	return st.now().UTC().Truncate(time.Millisecond)
}

// ---- 利用者 ----

// authUser はログインの判定に使う利用者の値。
type authUser struct {
	ID            string
	Email         string
	Name          string
	Role          string
	EmailVerified bool
	Banned        bool
	MFAEnabled    bool
	PasswordHash  sql.NullString
}

const authUserColumns = "u.id, COALESCE(u.email, ''), COALESCE(u.name, ''), u.role, u.emailVerified, u.bannedAt IS NOT NULL," +
	" EXISTS(SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL), p.hash" +
	" FROM `user` AS u LEFT JOIN AuthPassword AS p ON p.userId = u.id"

func scanAuthUser(row *sql.Row) (*authUser, error) {
	var u authUser
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified, &u.Banned, &u.MFAEnabled, &u.PasswordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (st *authStore) findUserByEmail(ctx context.Context, email string) (*authUser, error) {
	return scanAuthUser(st.db.QueryRowContext(ctx, "SELECT "+authUserColumns+" WHERE u.email = ?", email))
}

func (st *authStore) findUserByID(ctx context.Context, id string) (*authUser, error) {
	return scanAuthUser(st.db.QueryRowContext(ctx, "SELECT "+authUserColumns+" WHERE u.id = ?", id))
}

// createUserWithPassword はメール＋パスワードの登録で、まだ確認していない利用者とパスワードを作る。
// 利用者の行の作成・変更は持ち主の internal/write/account（JUK-154）。
func (st *authStore) createUserWithPassword(ctx context.Context, id, email, hash string, now time.Time) error {
	return account.CreateUserWithPassword(ctx, st.db, id, email, hash, now)
}

// markEmailVerified はメールアドレスを確認済みにする。初めて確認済みにしたときだけ true。
func (st *authStore) markEmailVerified(ctx context.Context, userID string, now time.Time) (bool, error) {
	return account.MarkEmailVerified(ctx, st.db, userID, now)
}

// deleteUser は退会で利用者とデータを消す。消し方は管理者の削除と同じ deleteUserAndData。
func (st *authStore) deleteUser(ctx context.Context, userID string) error {
	return database.InTx(ctx, st.db, func(tx *sql.Tx) error {
		return deleteUserAndData(ctx, tx, userID)
	})
}

// sessionUser は GET /api/auth/session の利用者と、セッションの期限（DB の DATETIME の文字列）。
// セッションか利用者が無ければ nil。
func (st *authStore) sessionUser(ctx context.Context, sessionID string) (*SessionUser, string, error) {
	var u SessionUser
	var name sql.NullString
	var createdAt, expiresAt string
	err := st.db.QueryRowContext(ctx,
		"SELECT u.id, COALESCE(u.email, ''), u.name, u.nickname, u.image, u.role, u.emailVerified,"+
			" EXISTS(SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL), u.createdAt, s.expiresAt"+
			" FROM AuthSession AS s JOIN `user` AS u ON u.id = s.userId WHERE s.id = ?", sessionID,
	).Scan(&u.ID, &u.Email, &name, &u.Nickname, &u.Image, &u.Role,
		&u.EmailVerified, &u.TwoFactorEnabled, &createdAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	u.Name = name.String
	u.CreatedAt = database.ISOFromDatetime(createdAt)
	return &u, expiresAt, nil
}

// loginMethods はログインの手段（パスワードの有無と、連携している外部サービスの名前の一覧）。
func (st *authStore) loginMethods(ctx context.Context, userID string) (hasPassword bool, providers []string, err error) {
	if err := st.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM AuthPassword WHERE userId = ?)", userID).Scan(&hasPassword); err != nil {
		return false, nil, err
	}
	rows, err := st.db.QueryContext(ctx, "SELECT provider FROM AuthIdentity WHERE userId = ? ORDER BY provider", userID)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	providers = []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return false, nil, err
		}
		providers = append(providers, p)
	}
	return hasPassword, providers, rows.Err()
}

// ---- パスワード ----

// setPassword はパスワードのハッシュを保存する（無ければ作る）。
func (st *authStore) setPassword(ctx context.Context, userID, hash string, now time.Time) error {
	return account.SetPassword(ctx, st.db, userID, hash, now)
}

// replacePassword はパスワードを置き換え、keepSessionID 以外のセッションと、まだ使われていない
// 再設定・確認のトークン、2段階認証の途中の状態を消す（06 B6・10 E3）。
func (st *authStore) replacePassword(ctx context.Context, userID, hash, keepSessionID string) error {
	return account.ReplacePassword(ctx, st.db, userID, hash, keepSessionID, st.clock())
}

// ---- メールで送るトークン（10 E1） ----

// issueToken は用途つきのトークンを作る。同じ人・同じ用途の古いものと、期限の切れたものは消す。
func (st *authStore) issueToken(ctx context.Context, userID, purpose string, ttl time.Duration) (string, error) {
	raw, hash := newToken()
	if err := account.IssueToken(ctx, st.db, userID, purpose, hash, ttl, st.clock()); err != nil {
		return "", err
	}
	return raw, nil
}

// peekToken はトークンを使わずに持ち主を返す（使う前にパスワードの規則を確かめたいとき）。
func (st *authStore) peekToken(ctx context.Context, raw, purpose string) (string, error) {
	hash := hashToken(raw)
	if hash == nil {
		return "", nil
	}
	var userID string
	err := st.db.QueryRowContext(ctx,
		"SELECT userId FROM AuthToken WHERE tokenHash = ? AND purpose = ? AND expiresAt > ?", hash, purpose, st.clock()).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// consumeToken はトークンを使う。用途が違う・期限切れ・使用済みなら空を返す。
func (st *authStore) consumeToken(ctx context.Context, raw, purpose string) (string, error) {
	hash := hashToken(raw)
	if hash == nil {
		return "", nil
	}
	return account.ConsumeToken(ctx, st.db, hash, purpose, st.clock())
}

// ---- 2段階認証の途中の状態（G3） ----

// createMFAChallenge は途中の状態を作り、Cookie に入れる値を返す。同じ人の前のものと、期限の切れたものは消す。
func (st *authStore) createMFAChallenge(ctx context.Context, userID string) (string, error) {
	raw, hash := newToken()
	if err := account.CreateMFAChallenge(ctx, st.db, userID, hash, mfaChallengeTTL, st.clock()); err != nil {
		return "", err
	}
	return raw, nil
}

// countMFAChallengeAttempt は途中の状態1つで試した数を1つ増やし、持ち主を返す（H1）。期限切れ・試行が
// 上限に達した・無いなら空。
func (st *authStore) countMFAChallengeAttempt(ctx context.Context, tokenHash []byte) (string, error) {
	return account.CountMFAChallengeAttempt(ctx, st.db, tokenHash, mfaChallengeMaxAttempts, st.clock())
}

func (st *authStore) deleteMFAChallenges(ctx context.Context, userID string) error {
	return account.DeleteMFAChallenges(ctx, st.db, userID)
}

// ---- TOTP と予備コード（G1・G2） ----

// startTOTPSetup は暗号化した秘密を、まだ有効にしていない状態で保存し、予備コードを作り直す。
// 作り直したら古い予備コードは全部無効になる（G2）。
func (st *authStore) startTOTPSetup(ctx context.Context, userID, sealed string, backupHashes [][]byte) error {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO AuthTotp (userId, secret, createdAt, enabledAt, lastUsedStep) VALUES (?, ?, ?, NULL, NULL)
		 ON DUPLICATE KEY UPDATE secret = VALUES(secret), createdAt = VALUES(createdAt), enabledAt = NULL, lastUsedStep = NULL`,
		userID, sealed, st.clock()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM AuthBackupCode WHERE userId = ?", userID); err != nil {
		return err
	}
	for _, hash := range backupHashes {
		if _, err := tx.ExecContext(ctx, "INSERT INTO AuthBackupCode (userId, codeHash) VALUES (?, ?)", userID, hash); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// pendingTOTPSecret は設定の途中（まだ有効にしていない）の、暗号化した秘密。無ければ空。
func (st *authStore) pendingTOTPSecret(ctx context.Context, userID string) (string, error) {
	var sealed string
	err := st.db.QueryRowContext(ctx, "SELECT secret FROM AuthTotp WHERE userId = ? AND enabledAt IS NULL", userID).Scan(&sealed)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sealed, err
}

// enableTOTP は設定の途中の TOTP を有効にし、確かめたステップを使用済みにする。途中のものが無ければ false。
func (st *authStore) enableTOTP(ctx context.Context, userID string, step int64, now time.Time) (bool, error) {
	res, err := st.db.ExecContext(ctx, "UPDATE AuthTotp SET enabledAt = ?, lastUsedStep = ? WHERE userId = ? AND enabledAt IS NULL", now, step, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// enabledTOTP は有効な TOTP の、暗号化した秘密と最後に使ったステップ。有効でなければ sealed が空。
func (st *authStore) enabledTOTP(ctx context.Context, userID string) (sealed string, lastStep *int64, err error) {
	var last sql.NullInt64
	err = st.db.QueryRowContext(ctx, "SELECT secret, lastUsedStep FROM AuthTotp WHERE userId = ? AND enabledAt IS NOT NULL", userID).Scan(&sealed, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	if last.Valid {
		lastStep = &last.Int64
	}
	return sealed, lastStep, nil
}

// markTOTPStepUsed はステップを使用済みにする。同じステップ以前がもう使われていれば false。
func (st *authStore) markTOTPStepUsed(ctx context.Context, userID string, step int64) (bool, error) {
	res, err := st.db.ExecContext(ctx,
		"UPDATE AuthTotp SET lastUsedStep = ? WHERE userId = ? AND (lastUsedStep IS NULL OR lastUsedStep < ?)", step, userID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (st *authStore) resealTOTP(ctx context.Context, userID, sealed string) error {
	_, err := st.db.ExecContext(ctx, "UPDATE AuthTotp SET secret = ? WHERE userId = ?", sealed, userID)
	return err
}

// useBackupCode は予備コードを確かめ、通ったら消す（1回だけ使える。G2）。
func (st *authStore) useBackupCode(ctx context.Context, userID, code string) (bool, error) {
	res, err := st.db.ExecContext(ctx, "DELETE FROM AuthBackupCode WHERE userId = ? AND codeHash = ?", userID, hashBackupCode(code))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (st *authStore) countBackupCodes(ctx context.Context, userID string) (int, error) {
	var remaining int
	err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM AuthBackupCode WHERE userId = ?", userID).Scan(&remaining)
	return remaining, err
}

// ---- 外部ログイン（F1・F2） ----

// saveOAuthState は外部ログインを始めるときの state を保存する。期限の切れたものは消す。
func (st *authStore) saveOAuthState(ctx context.Context, stateHash []byte, provider string, s authguard.OAuthState) error {
	return authguard.SaveOAuthState(ctx, st.db, stateHash, provider, s, st.clock())
}

// consumeOAuthState は state の行を読んで消す（1回だけ使える）。プロバイダーが違えば使わない。
func (st *authStore) consumeOAuthState(ctx context.Context, state, provider string) (*authguard.OAuthState, error) {
	hash := hashToken(state)
	if hash == nil {
		return nil, nil
	}
	return authguard.ConsumeOAuthState(ctx, st.db, hash, provider, st.clock())
}

// identityUser は、プロバイダーとプロバイダー側の ID の組に結びついた利用者。無ければ空。
func (st *authStore) identityUser(ctx context.Context, provider, subject string) (string, error) {
	var userID string
	err := st.db.QueryRowContext(ctx, "SELECT userId FROM AuthIdentity WHERE provider = ? AND providerUserId = ?", provider, subject).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

// linkOAuthIdentity は、確認済みのメールアドレス email の利用者に外部ログインを結びつける（resolveOAuthUser の
// 3〜5）。同じメールアドレスの利用者の行をロックして（FOR UPDATE）、1つのトランザクションで次のどれかをする。
//
//   - いなければ新しく作る（event は "created"）
//   - いて確認済みなら、そのまま結びつける（"linked"）
//   - いて未確認なら、パスワード・セッション・トークンを消してから結びつけ、確認済みにする（"claimed_unverified"）
func (st *authStore) linkOAuthIdentity(ctx context.Context, provider, email string, ident *oauthIdentity, now time.Time) (userID, event string, err error) {
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var verified bool
	err = tx.QueryRowContext(ctx, "SELECT id, emailVerified FROM `user` WHERE email = ? FOR UPDATE", email).Scan(&userID, &verified)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		userID = newUserID()
		name := ident.Name
		if name == "" {
			name = email
		}
		var image any
		if ident.Image != "" {
			image = truncate(ident.Image, 191)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO `user` (id, name, email, image, emailVerified, createdAt, updatedAt) VALUES (?, ?, ?, ?, true, ?, ?)",
			userID, truncate(name, 191), email, image, now, now); err != nil {
			return "", "", err
		}
		event = "created"
	case err != nil:
		return "", "", err
	case verified:
		event = "linked"
	default:
		for _, q := range []string{
			"DELETE FROM AuthPassword WHERE userId = ?",
			"DELETE FROM AuthSession WHERE userId = ?",
			"DELETE FROM AuthToken WHERE userId = ?",
		} {
			if _, err := tx.ExecContext(ctx, q, userID); err != nil {
				return "", "", err
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE `user` SET emailVerified = true, updatedAt = ? WHERE id = ?", now, userID); err != nil {
			return "", "", err
		}
		event = "claimed_unverified"
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO AuthIdentity (provider, providerUserId, userId, createdAt) VALUES (?, ?, ?, ?)",
		provider, ident.Subject, userID, now); err != nil {
		return "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return userID, event, nil
}
