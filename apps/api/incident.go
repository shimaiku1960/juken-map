package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 乗っ取りが起きたときの操作と、管理者の付け外し（cli.go のコマンドが使う。手順は docs/incident-response.md）。
//
// 管理画面の「停止」は、自分自身・ほかの管理者を止められない（admin_users.go）。管理者が乗っ取られたときや、
// 管理画面に入れないときでも使えるよう、ここではその守りを置かない。そのぶん画面からは呼べず、本番では
// EC2 で動いている Go のコンテナの中からしか実行できない（RDS には外から繋げない）。
//
// JUK-109 で本番の Node のコンテナが無くなったので、Node の incident-service.ts・user-service.ts から移した（JUK-122）。

// errUserNotFound は、そのメールアドレスの利用者がいないとき。
var errUserNotFound = errors.New("user not found")

type incidentTarget struct {
	ID       string
	Email    string
	Role     string
	BannedAt *string // DATETIME の文字列（ISO にしたもの）。止めていなければ nil
}

type incidentSession struct {
	CreatedAt         string
	ExpiresAt         string
	LastUsedAt        string
	IPAddress         *string
	UserAgent         *string
	TwoFactorVerified bool
}

type incidentStore struct {
	db  *sql.DB
	now func() time.Time
}

func (st incidentStore) findByEmail(ctx context.Context, email string) (incidentTarget, error) {
	var (
		u        incidentTarget
		userMail sql.NullString
		banned   sql.NullString
	)
	err := st.db.QueryRowContext(ctx, "SELECT id, email, role, bannedAt FROM `user` WHERE email = ?", email).
		Scan(&u.ID, &userMail, &u.Role, &banned)
	if errors.Is(err, sql.ErrNoRows) {
		return u, errUserNotFound
	}
	if err != nil {
		return u, err
	}
	u.Email = userMail.String
	if banned.Valid {
		iso := database.ISOFromDatetime(banned.String)
		u.BannedAt = &iso
	}
	return u, nil
}

// deleteSessions はその人のセッションをすべて消し、消した数を返す（認証基準 10 の C5 の3）。
func (st incidentStore) deleteSessions(ctx context.Context, userID string) (int64, error) {
	res, err := st.db.ExecContext(ctx, "DELETE FROM AuthSession WHERE userId = ?", userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// listSessions はその人のログイン中のセッションを、作られた順に返す（どこから入られたかを見るため）。
func (st incidentStore) listSessions(ctx context.Context, email string) (incidentTarget, []incidentSession, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return u, nil, err
	}
	rows, err := st.db.QueryContext(ctx,
		`SELECT createdAt, expiresAt, lastUsedAt, ipAddress, userAgent, mfaVerifiedAt IS NOT NULL
		 FROM AuthSession WHERE userId = ? ORDER BY createdAt ASC`, u.ID)
	if err != nil {
		return u, nil, err
	}
	defer rows.Close()
	sessions := []incidentSession{}
	for rows.Next() {
		var s incidentSession
		if err := rows.Scan(&s.CreatedAt, &s.ExpiresAt, &s.LastUsedAt, &s.IPAddress, &s.UserAgent, &s.TwoFactorVerified); err != nil {
			return u, nil, err
		}
		s.CreatedAt, s.ExpiresAt, s.LastUsedAt = database.ISOFromDatetime(s.CreatedAt), database.ISOFromDatetime(s.ExpiresAt), database.ISOFromDatetime(s.LastUsedAt)
		sessions = append(sessions, s)
	}
	return u, sessions, rows.Err()
}

// opsAuditRetention は OpsAuditLog を残す期間。CloudTrail（S3 に1年）とそろえ、突き合わせられる期間を同じにする。
const opsAuditRetention = 365 * 24 * time.Hour

// recordOps は、コマンドが変えたことを OpsAuditLog に1行書く（JUK-138、セキュリティ基準 H4）。
// 本番では `docker exec` で動き、出力が実行した人の端末にしか出ないので、DB に残す。見るだけの操作
// （sessions・grant-admin --list）は書かない。変えた後に書くので、書けなかったときはエラーを返して
// コマンドを失敗（終了コード 1）にし、記録が無いことに実行した人が気づけるようにする。
// 書いたついでに、残す期間を過ぎた行を消す（運用コマンドはめったに使わないので、行は少ない）。
func (st incidentStore) recordOps(ctx context.Context, action, targetID string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	var target any // 全員が対象の操作（revoke-all など）は NULL
	if targetID != "" {
		target = targetID
	}
	host, _ := os.Hostname() // 本番ではコンテナ ID
	now := st.now().UTC()
	if _, err := st.db.ExecContext(ctx,
		"INSERT INTO OpsAuditLog (action, targetId, detail, host, createdAt) VALUES (?, ?, ?, ?, ?)",
		action, target, string(raw), host, now); err != nil {
		return fmt.Errorf("操作はしましたが、記録（OpsAuditLog）に書けませんでした: %w", err)
	}
	if _, err := st.db.ExecContext(ctx, "DELETE FROM OpsAuditLog WHERE createdAt < ?", now.Add(-opsAuditRetention)); err != nil {
		return fmt.Errorf("操作と記録はしましたが、1年より古い記録を消せませんでした: %w", err)
	}
	return nil
}

type opsAuditEntry struct {
	CreatedAt string
	Action    string
	TargetID  *string
	Host      string
	Detail    string
}

// listOps は OpsAuditLog を新しい順に limit 件返す（incident log）。本番の RDS にはマスターで入らないと
// SQL を打てないので、記録を見るのも運用コマンドと同じ docker exec でできるようにする。
func (st incidentStore) listOps(ctx context.Context, limit int) ([]opsAuditEntry, error) {
	rows, err := st.db.QueryContext(ctx,
		"SELECT createdAt, action, targetId, host, detail FROM OpsAuditLog ORDER BY createdAt DESC, id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []opsAuditEntry{}
	for rows.Next() {
		var e opsAuditEntry
		if err := rows.Scan(&e.CreatedAt, &e.Action, &e.TargetID, &e.Host, &e.Detail); err != nil {
			return nil, err
		}
		e.CreatedAt = database.ISOFromDatetime(e.CreatedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// revokeSessions はその人のセッションをすべて消す。止めはしないので、パスワードを知っていればまた入れる。
func (st incidentStore) revokeSessions(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	removed, err := st.deleteSessions(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	return removed, st.recordOps(ctx, "revoke", u.ID, map[string]any{"sessionsRemoved": removed})
}

// ban はその人を止め、セッションをすべて消す。止めた人は次のログインで断られる（auth_handlers.go）。
// bannedAt を先に書くので、消している間に新しく入られても、そのログインは断られる。
// 止め直しても最初に止めた日時を保つ（管理画面の停止と同じ）。
func (st incidentStore) ban(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	// DATETIME(3) は端数を丸めるので、先にミリ秒で切っておき、記録の after と DB の値をそろえる。
	now := st.now().UTC().Truncate(time.Millisecond)
	if _, err := st.db.ExecContext(ctx,
		"UPDATE `user` SET bannedAt = COALESCE(bannedAt, ?), updatedAt = ? WHERE id = ?", now, now, u.ID); err != nil {
		return 0, err
	}
	removed, err := st.deleteSessions(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	after := u.BannedAt
	if after == nil {
		iso := isoMillis(now)
		after = &iso
	}
	return removed, st.recordOps(ctx, "ban", u.ID, map[string]any{
		"before": map[string]any{"bannedAt": u.BannedAt}, "after": map[string]any{"bannedAt": after}, "sessionsRemoved": removed})
}

// unban は止めたのを戻す。
func (st incidentStore) unban(ctx context.Context, email string) error {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return err
	}
	if _, err := st.db.ExecContext(ctx, "UPDATE `user` SET bannedAt = NULL, updatedAt = ? WHERE id = ?", st.now().UTC(), u.ID); err != nil {
		return err
	}
	return st.recordOps(ctx, "unban", u.ID, map[string]any{
		"before": map[string]any{"bannedAt": u.BannedAt}, "after": map[string]any{"bannedAt": nil}})
}

// revokeAll は全員のセッションを消す（C5 の4）。ログインの不具合や、セッションを読める立場（DB）からの
// 漏えいが疑われるときに使う。全員がログインし直しになる。
func (st incidentStore) revokeAll(ctx context.Context) (int64, error) {
	res, err := st.db.ExecContext(ctx, "DELETE FROM AuthSession")
	if err != nil {
		return 0, err
	}
	removed, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return removed, st.recordOps(ctx, "revoke-all", "", map[string]any{"sessionsRemoved": removed})
}

// revokeAdmins は管理者全員のセッションを消す。管理者のアカウントが1つでも乗っ取られたかもしれないときに使う。
// 消した管理者（メールアドレス。無ければ ID）と、消したセッションの数を返す。
func (st incidentStore) revokeAdmins(ctx context.Context) ([]string, int64, error) {
	rows, err := st.db.QueryContext(ctx, "SELECT id, email FROM `user` WHERE role = 'admin' ORDER BY email ASC")
	if err != nil {
		return nil, 0, err
	}
	type admin struct {
		id    string
		email sql.NullString
	}
	var admins []admin
	for rows.Next() {
		var a admin
		if err := rows.Scan(&a.id, &a.email); err != nil {
			rows.Close()
			return nil, 0, err
		}
		admins = append(admins, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	names, ids := []string{}, []string{}
	var removed int64
	for _, a := range admins {
		n, err := st.deleteSessions(ctx, a.id)
		if err != nil {
			return nil, 0, err
		}
		removed += n
		ids = append(ids, a.id)
		if a.email.Valid {
			names = append(names, a.email.String)
		} else {
			names = append(names, a.id)
		}
	}
	// 対象が複数なので、targetId は空にして detail に userId を並べる（メールアドレスは残さない）。
	return names, removed, st.recordOps(ctx, "revoke-admins", "", map[string]any{"targetIds": ids, "sessionsRemoved": removed})
}

// resetTwoFactor は2段階認証を設定する前に戻し、セッションをすべて消す。認証アプリの秘密が漏れたかもしれない
// ときや、本人が認証アプリも予備コードも無くしたときに使う。次に /admin を開くと設定し直しになる。
func (st incidentStore) resetTwoFactor(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	for _, q := range []string{
		"DELETE FROM AuthTotp WHERE userId = ?",
		"DELETE FROM AuthBackupCode WHERE userId = ?",
		"DELETE FROM AuthMfaChallenge WHERE userId = ?",
	} {
		if _, err := st.db.ExecContext(ctx, q, u.ID); err != nil {
			return 0, err
		}
	}
	removed, err := st.deleteSessions(ctx, u.ID)
	if err != nil {
		return 0, err
	}
	return removed, st.recordOps(ctx, "reset-2fa", u.ID, map[string]any{"sessionsRemoved": removed})
}

// errUnverified は、メール確認前の人を管理者にしようとしたとき。
var errUnverified = errors.New("email not verified")

// setRole はメールアドレスで利用者を探して role を付け替える（grant-admin）。前の role と、消したセッションの数を返す。
//
// メール確認前の人には admin を付けない。他人のアドレスで登録されただけのアカウントを、確認前に管理者にして
// しまわないため。付け替えたら、その人のセッションをすべて消す（認証基準 10 の C4：権限の変更のたびに作り直す）。
// セッションの期限は role で決まる（一般30日・管理者24時間、auth_session.go）ので、ログインし直してもらう。
func (st incidentStore) setRole(ctx context.Context, email, role string) (previous string, removed int64, err error) {
	var (
		id       string
		verified bool
	)
	err = st.db.QueryRowContext(ctx, "SELECT id, emailVerified, role FROM `user` WHERE email = ?", email).
		Scan(&id, &verified, &previous)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, errUserNotFound
	}
	if err != nil {
		return "", 0, err
	}
	if role == "admin" && !verified {
		return "", 0, errUnverified
	}
	if _, err := st.db.ExecContext(ctx, "UPDATE `user` SET role = ?, updatedAt = ? WHERE id = ?", role, st.now().UTC(), id); err != nil {
		return "", 0, err
	}
	if removed, err = st.deleteSessions(ctx, id); err != nil {
		return "", 0, err
	}
	return previous, removed, st.recordOps(ctx, "set-role", id, map[string]any{
		"before": map[string]any{"role": previous}, "after": map[string]any{"role": role}, "sessionsRemoved": removed})
}

type adminSummary struct {
	Email            string
	TwoFactorEnabled bool
	HasPassword      bool
}

// listAdmins は管理者の一覧。管理画面に入るには2段階認証が要る（router.go の admin）ので、その有無も出す。
func (st incidentStore) listAdmins(ctx context.Context) ([]adminSummary, error) {
	rows, err := st.db.QueryContext(ctx, "SELECT u.email,"+
		" EXISTS (SELECT 1 FROM AuthTotp AS t WHERE t.userId = u.id AND t.enabledAt IS NOT NULL),"+
		" EXISTS (SELECT 1 FROM AuthPassword AS p WHERE p.userId = u.id)"+
		" FROM `user` AS u WHERE u.role = 'admin' ORDER BY u.email ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	admins := []adminSummary{}
	for rows.Next() {
		var (
			a     adminSummary
			email sql.NullString
		)
		if err := rows.Scan(&email, &a.TwoFactorEnabled, &a.HasPassword); err != nil {
			return nil, err
		}
		a.Email = email.String
		admins = append(admins, a)
	}
	return admins, rows.Err()
}
