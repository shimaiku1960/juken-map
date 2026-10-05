package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// 認証のメール（確認・再設定・本人への知らせ・運営者への通知）。Node の infra/email.ts と
// email-limits.ts を移したもの（JUK-115）。
//
// 送信には宛先ごと・全体の上限をかける（06 E1）。確認メールの再送と再設定は、ログインしていなくても
// 宛先を指定して送らせられるので、上限が無いと同じ人へ送り続けたり（嫌がらせ）、Resend の無料枠
// （1日100通）を使い切らせたりできる。上限を超えたら送らずにログとメトリクスに残す。
//
// 送るのは応答を返したあと（別の goroutine）。送り終わりを待つと、登録済みのメールアドレスだけ
// Resend の分だけ遅く返り、応答の時間で登録の有無が分かる（10 H2）。

type emailKind string

const (
	emailVerification      emailKind = "verification"
	emailPasswordReset     emailKind = "password-reset"
	emailPasswordChanged   emailKind = "password-changed"
	emailAlreadyRegistered emailKind = "already-registered"
	emailMFAEnabled        emailKind = "mfa-enabled"
	emailAccountLinked     emailKind = "account-linked"
	emailAccountDeleted    emailKind = "account-deleted"
	emailAdminNewUser      emailKind = "admin-new-user"
)

// emailKinds はメトリクスを 0 で作っておく種類の一覧。
var emailKinds = []emailKind{
	emailVerification, emailPasswordReset, emailPasswordChanged, emailAlreadyRegistered,
	emailMFAEnabled, emailAccountLinked, emailAccountDeleted, emailAdminNewUser,
}

const (
	// emailPerRecipientPerHour は同じ宛先へ1時間に送る数の上限。再設定を何度か頼み直す本人は困らない数。
	emailPerRecipientPerHour = 5
	// emailGlobalPerDay はアプリ全体で24時間に送る数の上限。Resend の無料枠は1日100通で、毎日の通知と分け合う。
	emailGlobalPerDay = 80
	// emailSendLock は「数えてから記録する」までを1件ずつ通す MySQL の名前付きロック（Node と同じ名前）。
	emailSendLock        = "juken-map:email-send"
	emailLockWaitSeconds = 5
	authEmailFrom        = "受験マップ <noreply@juken-map.com>"
	// authEmailTimeout は、応答を返したあとに送る1通にかける時間の上限。
	authEmailTimeout = 30 * time.Second
)

// emailSender は Resend へ1通送る。応答のヘッダー（送信枠の残り）を返す。テストでは偽物に差し替える。
type emailSender interface {
	send(ctx context.Context, to, subject, html string) (http.Header, error)
}

type authMailer struct {
	db      *sql.DB
	sender  emailSender
	metrics *metrics
	adminTo string
	now     func() time.Time
	// async は送る処理を動かす。本番は goroutine、テストはその場で動かして結果を確かめる。
	async func(func())
}

func (m *authMailer) clock() time.Time {
	return m.now().UTC().Truncate(time.Millisecond)
}

// sendLater は応答を返したあとに1通送る。上限で止めたときも、失敗したときも、ログとメトリクスに残すだけ。
func (m *authMailer) sendLater(kind emailKind, to, subject, body string) {
	m.async(func() {
		ctx, cancel := context.WithTimeout(context.Background(), authEmailTimeout)
		defer cancel()
		if err := m.send(ctx, kind, to, subject, body); err != nil {
			slog.Error("[auth-email] Failed to send.", "kind", string(kind), "err", err.Error())
		}
	})
}

func (m *authMailer) send(ctx context.Context, kind emailKind, to, subject, body string) error {
	allowed, err := m.reserve(ctx, kind, to)
	if err != nil {
		m.metrics.countEmail(kind, "failed")
		return err
	}
	if !allowed {
		m.metrics.countEmail(kind, "blocked")
		return nil
	}
	headers, err := m.sender.send(ctx, to, subject, body)
	if err != nil {
		m.metrics.countEmail(kind, "failed")
		return err
	}
	m.metrics.observeResendQuota(headers, m.now())
	m.metrics.countEmail(kind, "sent")
	return nil
}

// reserve は送ってよければ記録して true、上限を超えるなら記録せず false を返す。
// 数えてから記録するまでを名前付きロックで1件ずつ通す（同時に来た送信がどれも「まだ上限前」を見て
// 超えないように。JUK-107）。ロックは接続に付くので、1本の接続を借りて最後まで同じ接続で流す。
func (m *authMailer) reserve(ctx context.Context, kind emailKind, to string) (bool, error) {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	locked := false
	defer func() {
		if locked {
			if _, err := conn.ExecContext(context.WithoutCancel(ctx), "DO RELEASE_LOCK(?)", emailSendLock); err != nil {
				// 解けなかった接続はロックを持ったまま残りうるので、プールへ戻さずに捨てる
				// （接続が切れれば MySQL がロックを解く）。
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		conn.Close()
	}()

	var acquired sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", emailSendLock, emailLockWaitSeconds).Scan(&acquired); err != nil {
		return false, err
	}
	recipient := recipientHash(to)
	if acquired.Int64 != 1 {
		slog.Warn("[email-limits] Email not sent: send limit reached.", "kind", string(kind), "reason", "lock", "recipient", recipient[:12])
		return false, nil
	}
	locked = true

	now := m.clock()
	if err := sweepEmailSends(ctx, conn, now.Add(-24*time.Hour)); err != nil {
		return false, err
	}
	var global int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM EmailSend WHERE sentAt > ?", now.Add(-24*time.Hour)).Scan(&global); err != nil {
		return false, err
	}
	reason := ""
	if global >= emailGlobalPerDay {
		reason = "global"
	} else if kind != emailAdminNewUser {
		// 運営者への通知は宛先が1つなので、宛先ごとの上限にはかけない（全体の数には入れる）。
		var perRecipient int
		if err := conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM EmailSend WHERE recipientHash = ? AND sentAt > ? AND kind <> 'admin-new-user'",
			recipient, now.Add(-time.Hour)).Scan(&perRecipient); err != nil {
			return false, err
		}
		if perRecipient >= emailPerRecipientPerHour {
			reason = "recipient"
		}
	}
	if reason != "" {
		// 宛先そのものはログに出さない（誰が狙われたかは、ハッシュの先頭で突き合わせられれば足りる）。
		slog.Warn("[email-limits] Email not sent: send limit reached.", "kind", string(kind), "reason", reason, "recipient", recipient[:12])
		return false, nil
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO EmailSend (recipientHash, kind, sentAt) VALUES (?, ?, ?)", recipient, string(kind), now)
	return err == nil, err
}

// sweepEmailSends は1日より古い行を少しずつ消す（主キーで消し、索引の隙間をロックしない）。
func sweepEmailSends(ctx context.Context, conn *sql.Conn, cutoff time.Time) error {
	rows, err := conn.QueryContext(ctx, "SELECT id FROM EmailSend WHERE sentAt <= ? LIMIT 100", cutoff)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := conn.ExecContext(ctx, "DELETE FROM EmailSend WHERE id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

// recipientHash は宛先を小文字にした SHA-256（宛先をそのまま DB に残さない）。Node と同じ値になる。
func recipientHash(to string) string {
	sum := sha256.Sum256([]byte(normalizeEmail(to)))
	return hex.EncodeToString(sum[:])
}

// ---- 文面 ----

func (m *authMailer) sendVerification(to, link string) {
	m.sendLater(emailVerification, to, "【受験マップ】メールアドレスの確認",
		`<p>受験マップへの登録ありがとうございます。以下のリンクを開き、画面のボタンを押してメールアドレスを確認してください。</p>`+
			`<p><a href="`+html.EscapeString(link)+`">メールアドレスを確認する</a></p>`+
			`<p>リンクの有効期限は24時間です。心当たりがない場合は、このメールを破棄してください。</p>`)
}

func (m *authMailer) sendPasswordReset(to, link string) {
	m.sendLater(emailPasswordReset, to, "【受験マップ】パスワードの再設定",
		`<p>以下のリンクからパスワードを再設定してください。</p>`+
			`<p><a href="`+html.EscapeString(link)+`">パスワードを再設定する</a></p>`+
			`<p>リンクの有効期限は1時間です。心当たりがない場合は、このメールを破棄してください（パスワードは変わりません）。</p>`)
}

// 「自分でなければ」の連絡先・手順を、本人への知らせのすべてに書く（10 E5）。
func (m *authMailer) ifNotYou(webOrigin string) string {
	return `<p>心当たりがない場合は、すぐに以下からパスワードを再設定してください。ほかの端末のログインはすべて解除されます。</p>` +
		`<p><a href="` + html.EscapeString(webOrigin+"/forgot-password") + `">パスワードを再設定する</a></p>`
}

func (m *authMailer) sendPasswordChanged(to, webOrigin string) {
	m.sendLater(emailPasswordChanged, to, "【受験マップ】パスワードが変更されました",
		`<p>受験マップのパスワードが変更されました。ほかの端末のログインはすべて解除しています。</p>`+m.ifNotYou(webOrigin))
}

func (m *authMailer) sendAlreadyRegistered(to, webOrigin string) {
	m.sendLater(emailAlreadyRegistered, to, "【受験マップ】このメールアドレスは登録済みです",
		`<p>このメールアドレスで新規登録の操作がありましたが、すでにアカウントがあります。</p>`+
			`<p><a href="`+html.EscapeString(webOrigin+"/login")+`">ログインする</a> ／ `+
			`<a href="`+html.EscapeString(webOrigin+"/forgot-password")+`">パスワードを忘れた方</a></p>`+
			`<p>心当たりがない場合は、このメールを破棄してください。アカウントは変わりません。</p>`)
}

func (m *authMailer) sendMFAEnabled(to, webOrigin string) {
	m.sendLater(emailMFAEnabled, to, "【受験マップ】2段階認証を有効にしました",
		`<p>受験マップのアカウントで、認証アプリによる2段階認証が有効になりました。</p>`+m.ifNotYou(webOrigin))
}

func (m *authMailer) sendAccountLinked(to, provider, webOrigin string) {
	m.sendLater(emailAccountLinked, to, "【受験マップ】外部サービスでのログインを連携しました",
		`<p>受験マップのアカウントに、`+html.EscapeString(providerLabel(provider))+` でのログインを連携しました。`+
			`次から `+html.EscapeString(providerLabel(provider))+` でもログインできます。</p>`+m.ifNotYou(webOrigin))
}

// sendAccountDeleted は退会を本人へ知らせる。アカウントはもう無いので、心当たりが無いときの連絡先は
// パスワードの再設定ではなく運営への問い合わせにする。
func (m *authMailer) sendAccountDeleted(to, webOrigin string) {
	m.sendLater(emailAccountDeleted, to, "【受験マップ】退会の手続きが完了しました",
		`<p>受験マップのアカウントと、学習記録・予定・志望校などのデータをすべて削除しました。ご利用ありがとうございました。</p>`+
			`<p>心当たりがない場合は、このメールに返信せず、利用規約の「お問い合わせ」にある連絡先までご連絡ください。</p>`+
			`<p><a href="`+html.EscapeString(webOrigin+"/terms")+`">利用規約</a></p>`)
}

// notifyAdminOfNewUser は新しい利用者を運営者へ知らせる。宛先の設定が無ければ、ログに残すだけ。
func (m *authMailer) notifyAdminOfNewUser(name, email string, createdAt time.Time) {
	if m.adminTo == "" {
		slog.Warn("[registration-notification] ADMIN_NOTIFICATION_EMAIL is not configured.")
		return
	}
	registeredAt := createdAt.In(tokyo).Format("2006年1月2日 15:04")
	m.sendLater(emailAdminNewUser, m.adminTo, "【受験マップ】新しいユーザーが登録しました",
		`<p>受験マップに新しいユーザーが登録しました。</p><dl>`+
			`<dt>登録日時</dt><dd>`+html.EscapeString(registeredAt)+`</dd>`+
			`<dt>表示名</dt><dd>`+html.EscapeString(name)+`</dd>`+
			`<dt>メールアドレス</dt><dd>`+html.EscapeString(email)+`</dd></dl>`)
}

func providerLabel(provider string) string {
	switch provider {
	case "google":
		return "Google"
	case "github":
		return "GitHub"
	}
	return provider
}

// ---- Resend ----

// resendSender は Resend の API を直接呼ぶ（SDK は使わない。notifications.go の httpMessenger と同じ）。
type resendSender struct {
	client *http.Client
	base   string
	key    string
}

func (s *resendSender) send(ctx context.Context, to, subject, body string) (http.Header, error) {
	if s.key == "" {
		return nil, errors.New("RESEND_API_KEY is not configured")
	}
	raw, err := json.Marshal(map[string]any{"from": authEmailFrom, "to": to, "subject": subject, "html": body})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+"/emails", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return nil, fmt.Errorf("resend %d: %s", res.StatusCode, detail)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return res.Header, nil
}

// resendQuotaHeaders は Resend の応答ヘッダーのうち、送信枠を使った数（日の分は無料プランにだけ付く）。
var resendQuotaHeaders = map[string]string{"daily": "X-Resend-Daily-Quota", "monthly": "X-Resend-Monthly-Quota"}

func (m *metrics) observeResendQuota(headers http.Header, now time.Time) {
	for period, name := range resendQuotaHeaders {
		used, err := strconv.ParseFloat(headers.Get(name), 64)
		if err != nil {
			continue
		}
		m.resendQuotaUsed.WithLabelValues(period).Set(used)
		m.resendQuotaObservedAt.WithLabelValues(period).Set(float64(now.UnixMilli()) / 1000)
	}
}

func (m *metrics) countEmail(kind emailKind, result string) {
	m.emailSends.WithLabelValues(string(kind), result).Inc()
}
