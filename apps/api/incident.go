package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
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

// opsAudit は運用のコマンドの記録に付ける実行場所。
func opsAudit() account.OpsAudit {
	host, _ := os.Hostname() // 本番ではコンテナ ID
	return account.OpsAudit{Host: host}
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

// revokeSessions はその人のセッションをすべて消し、記録を残す（account.RevokeUserSessions）。
// 止めはしないので、パスワードを知っていればまた入れる。
func (st incidentStore) revokeSessions(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	audit := opsAudit()
	return account.RevokeUserSessions(ctx, st.db, u.ID, st.now(), &audit)
}

// ban はその人を止め、セッションをすべて消し、記録を残す（account.Suspend が1つのトランザクションで行う）。
// 止め直しても最初に止めた日時を保つ（管理画面の停止と同じ操作）。
func (st incidentStore) ban(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	audit := opsAudit()
	// DATETIME(3) は端数を丸めるので、先にミリ秒で切っておく（dates.NowMillis と同じ）。
	banned, err := account.Suspend(ctx, st.db, u.ID, st.now().UTC().Truncate(time.Millisecond), &audit)
	return banned.SessionsRemoved, err
}

// unban は止めたのを戻し、記録を残す（account.Unsuspend）。
func (st incidentStore) unban(ctx context.Context, email string) error {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return err
	}
	audit := opsAudit()
	return account.Unsuspend(ctx, st.db, u.ID, st.now().UTC().Truncate(time.Millisecond), &audit)
}

// revokeAll は全員のセッションを消す（C5 の4）。ログインの不具合や、セッションを読める立場（DB）からの
// 漏えいが疑われるときに使う。全員がログインし直しになる。
func (st incidentStore) revokeAll(ctx context.Context) (int64, error) {
	audit := opsAudit()
	return account.RevokeAllSessions(ctx, st.db, st.now(), &audit)
}

// revokeAdmins は管理者全員のセッションを消す。管理者のアカウントが1つでも乗っ取られたかもしれないときに使う。
// 消した管理者（メールアドレス。無ければ ID）と、消したセッションの数を返す。
func (st incidentStore) revokeAdmins(ctx context.Context) ([]string, int64, error) {
	audit := opsAudit()
	admins, removed, err := account.RevokeAdminSessions(ctx, st.db, st.now(), &audit)
	if err != nil {
		return nil, 0, err
	}
	names := []string{}
	for _, a := range admins {
		if a.Email != "" {
			names = append(names, a.Email)
		} else {
			names = append(names, a.ID)
		}
	}
	return names, removed, nil
}

// resetTwoFactor は2段階認証を設定する前に戻し、セッションをすべて消し、記録を残す（account.ResetTwoFactor）。
// 認証アプリの秘密が漏れたかもしれないときや、本人が認証アプリも予備コードも無くしたときに使う。
// 次に /admin を開くと設定し直しになる。
func (st incidentStore) resetTwoFactor(ctx context.Context, email string) (int64, error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return 0, err
	}
	audit := opsAudit()
	return account.ResetTwoFactor(ctx, st.db, u.ID, st.now(), &audit)
}

// setRole はメールアドレスで利用者を探して role を付け替え、記録を残す（grant-admin。account.SetRole）。
// 前の role と、消したセッションの数を返す。メール確認前の人には admin を付けない（account.ErrUnverified）。
func (st incidentStore) setRole(ctx context.Context, email, role string) (previous string, removed int64, err error) {
	u, err := st.findByEmail(ctx, email)
	if err != nil {
		return "", 0, err
	}
	audit := opsAudit()
	return account.SetRole(ctx, st.db, u.ID, role, st.now().UTC(), &audit)
}

type adminSummary struct {
	Email            string
	TwoFactorEnabled bool
	HasPassword      bool
}

// listAdmins は管理者の一覧。管理画面に入るには2段階認証が要る（internal/httpx/router.go の Admin）ので、その有無も出す。
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
