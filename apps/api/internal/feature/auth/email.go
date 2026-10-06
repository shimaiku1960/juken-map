package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
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

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/authguard"
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
	emailAdminNewUser      emailKind = authguard.KindAdminNewUser
)

// emailKinds はメトリクスを 0 で作っておく種類の一覧。
var emailKinds = []emailKind{
	emailVerification, emailPasswordReset, emailPasswordChanged, emailAlreadyRegistered,
	emailMFAEnabled, emailAccountLinked, emailAccountDeleted, emailAdminNewUser,
}

// NewMetrics はメールの種類ごとの系列を 0 で用意した計測（internal/telemetry）。
func NewMetrics() *telemetry.Metrics {
	kinds := make([]string, len(emailKinds))
	for i, kind := range emailKinds {
		kinds[i] = string(kind)
	}
	return telemetry.NewMetrics(kinds)
}

const (
	authEmailFrom = "受験マップ <noreply@juken-map.com>"
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
	metrics *telemetry.Metrics
	adminTo string
	now     func() time.Time
	// Async は送る処理を動かす。本番は goroutine、テストはその場で動かして結果を確かめる。
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
		countEmail(m.metrics, kind, "failed")
		return err
	}
	if !allowed {
		countEmail(m.metrics, kind, "blocked")
		return nil
	}
	headers, err := m.sender.send(ctx, to, subject, body)
	if err != nil {
		countEmail(m.metrics, kind, "failed")
		return err
	}
	observeResendQuota(m.metrics, headers, m.now())
	countEmail(m.metrics, kind, "sent")
	return nil
}

// reserve は送ってよければ記録して true、上限を超えるなら記録せず false を返す。
// 上限の数え方（宛先ごと・全体、名前付きロックで1件ずつ通す）は持ち主の internal/write/authguard にある（JUK-154）。
func (m *authMailer) reserve(ctx context.Context, kind emailKind, to string) (bool, error) {
	recipient := recipientHash(to)
	block, err := authguard.ReserveEmail(ctx, m.db, recipient, string(kind), m.clock())
	if err != nil {
		return false, err
	}
	if block != authguard.EmailAllowed {
		// 宛先そのものはログに出さない（誰が狙われたかは、ハッシュの先頭で突き合わせられれば足りる）。
		slog.Warn("[email-limits] Email not sent: send limit reached.", "kind", string(kind), "reason", string(block), "recipient", recipient[:12])
		return false, nil
	}
	return true, nil
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
	registeredAt := createdAt.In(dates.Tokyo).Format("2006年1月2日 15:04")
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

// ResendSender は Resend の API を直接呼ぶ（SDK は使わない。internal/feature/notifications の HTTPMessenger と同じ）。
type ResendSender struct {
	Client *http.Client
	Base   string
	Key    string
}

func (s *ResendSender) send(ctx context.Context, to, subject, body string) (http.Header, error) {
	if s.Key == "" {
		return nil, errors.New("RESEND_API_KEY is not configured")
	}
	raw, err := json.Marshal(map[string]any{"from": authEmailFrom, "to": to, "subject": subject, "html": body})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, httpx.ExternalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Base+"/emails", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := s.Client.Do(req)
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

func observeResendQuota(m *telemetry.Metrics, headers http.Header, now time.Time) {
	for period, name := range resendQuotaHeaders {
		used, err := strconv.ParseFloat(headers.Get(name), 64)
		if err != nil {
			continue
		}
		m.ResendQuotaUsed.WithLabelValues(period).Set(used)
		m.ResendQuotaObservedAt.WithLabelValues(period).Set(float64(now.UnixMilli()) / 1000)
	}
}

func countEmail(m *telemetry.Metrics, kind emailKind, result string) {
	m.EmailSends.WithLabelValues(string(kind), result).Inc()
}
