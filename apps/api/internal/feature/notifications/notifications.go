// Package notifications は通知の入口。通知設定の読み取りと保存（/api/notification-preferences）と、
// 毎日の学習通知の送信（cron）を持つ。書き込みは internal/write/notification が持つ。
package notifications

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/notification"
)

// 毎日の学習通知の送信（JUK-74）。Node の routes/cron.ts と services/sendDailyNotifications.ts にあたる。
// GitHub Actions（.github/workflows/daily-study-notifications.yml）が朝7時と夜21時に呼ぶ。
//
// Node は1人ずつ順番に送っていた。送信は外部 API（Resend・LINE）を待つ時間がほとんどなので、
// Go では goroutine で同時に送る。ただし Resend の上限（チーム全体で毎秒10リクエスト。登録確認の
// メールやシミュレーションと分け合う）を超えないよう、メールは毎秒5通までに間隔を空ける。

const (
	// notificationWorkers は同時に送る数の上限。
	notificationWorkers = 5
	// emailInterval はメールを送る間隔（毎秒5通）。
	emailInterval = time.Second / 5
	// notificationBudget は1回の実行にかけてよい時間。nginx は応答を60秒で打ち切るので、その手前で止める。
	// 打ち切ったら 500 を返す。GitHub Actions の curl は 5xx を再試行するので、2回目は送り済みを飛ばして残りを送る。
	notificationBudget = 50 * time.Second
	// notificationFrom は送り主（Node と同じ）。
	notificationFrom = "受験マップ <noreply@juken-map.com>"
)

type deliveryChannel string

const (
	channelEmail deliveryChannel = "email"
	channelLine  deliveryChannel = "line"
)

// recipient はその時間帯に通知を受け取る設定のユーザーと、その日の予定・実績。
type recipient struct {
	ID         string
	Email      *string
	Name       *string
	Nickname   *string
	Morning    bool // メール（朝）
	Evening    bool // メール（夜）
	LineMorn   bool
	LineEven   bool
	LineUserID *string
	Plans      []planSummary
	LogMinutes []int64
}

// notificationStore は DB への読み書き。テストでは DB の代わりに偽物を渡す（Go の CI には DB が無い）。
type notificationStore interface {
	findRecipients(ctx context.Context, slot apischema.NotificationSlot, start, end time.Time) ([]recipient, error)
	// markDelivery は「この日・この時間帯・この経路は送った」印を先に入れる。
	// 同じ組み合わせが既にあれば duplicate が true（UNIQUE 制約）。
	markDelivery(ctx context.Context, userID string, date time.Time, slot apischema.NotificationSlot, channel deliveryChannel) (id int64, duplicate bool, err error)
	// unmarkDelivery は送れなかった印を消し、次の実行で再び送れるようにする。
	unmarkDelivery(ctx context.Context, id int64) error
}

// Messenger は外部サービスへの送信。テストでは偽物を渡す。
type Messenger interface {
	sendEmail(ctx context.Context, to string, m dailyMessage) error
	pushLine(ctx context.Context, lineUserID, text string) error
}

type dailyNotifier struct {
	store     notificationStore
	messenger Messenger
	workers   int
	emailPace *pacer
}

func newDailyNotifier(store notificationStore, m Messenger) *dailyNotifier {
	return &dailyNotifier{store: store, messenger: m, workers: notificationWorkers, emailPace: newPacer(emailInterval)}
}

type delivery struct {
	user    recipient
	channel deliveryChannel
	message dailyMessage
}

// send はその時間帯の通知を全員へ送り、件数をまとめて返す。
// 印を入れる SQL が失敗したとき（重複以外）は、残りを止めてエラーを返す（Node も例外で 500 になる）。
func (n *dailyNotifier) send(ctx context.Context, slot apischema.NotificationSlot, now time.Time) (apischema.NotificationSummary, error) {
	day := tokyoDateRange(now)
	summary := apischema.NotificationSummary{Date: day.date, Slot: slot}
	users, err := n.store.findRecipients(ctx, slot, day.start, day.end)
	if err != nil {
		return summary, err
	}

	var jobs []delivery
	for _, u := range users {
		nickname := "ユーザー"
		if u.Nickname != nil {
			nickname = *u.Nickname
		} else if u.Name != nil {
			nickname = *u.Name
		}
		message := buildDailyNotification(slot, nickname, u.Plans, u.LogMinutes)
		emailOn, lineOn := u.Morning, u.LineMorn
		if slot == apischema.NotificationSlotEvening {
			emailOn, lineOn = u.Evening, u.LineEven
		}
		if emailOn && u.Email != nil {
			jobs = append(jobs, delivery{u, channelEmail, message})
		}
		// LINE 通知だけオンでも、未連携なら送らない
		if lineOn && u.LineUserID != nil {
			jobs = append(jobs, delivery{u, channelLine, message})
		}
	}
	summary.Eligible = int64(len(jobs))

	var sent, skipped, failed atomic.Int64
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(n.workers)
	for _, job := range jobs {
		// ほかの goroutine がエラーで止めたら、残りは始めない。
		if gctx.Err() != nil {
			break
		}
		g.Go(func() error {
			id, duplicate, err := n.store.markDelivery(gctx, job.user.ID, day.start, slot, job.channel)
			if err != nil {
				return fmt.Errorf("mark delivery: %w", err)
			}
			if duplicate {
				skipped.Add(1)
				return nil
			}
			if err := n.deliver(gctx, job); err != nil {
				failed.Add(1)
				// 送れなかったので印を消す。止められた後でも消せるよう、取り消されない context で行う。
				undoCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), httpx.ExternalTimeout)
				defer cancel()
				if uerr := n.store.unmarkDelivery(undoCtx, id); uerr != nil {
					return fmt.Errorf("unmark delivery: %w", uerr)
				}
				slog.ErrorContext(ctx, "[daily-notification] Delivery failed.",
					"err", err.Error(), "slot", slot, "channel", job.channel, "userId", job.user.ID)
				return nil
			}
			sent.Add(1)
			return nil
		})
	}
	err = g.Wait()
	summary.Sent, summary.Skipped, summary.Failed = sent.Load(), skipped.Load(), failed.Load()
	if err == nil && ctx.Err() != nil {
		// 時間切れで、始めなかった分がある
		err = ctx.Err()
	}
	return summary, err
}

func (n *dailyNotifier) deliver(ctx context.Context, job delivery) error {
	if job.channel == channelEmail {
		if err := n.emailPace.wait(ctx); err != nil {
			return err
		}
		return n.messenger.sendEmail(ctx, *job.user.Email, job.message)
	}
	return n.messenger.pushLine(ctx, *job.user.LineUserID, job.message.Text)
}

// pacer は呼び出しの間隔を interval 以上に空ける。同時に何本の goroutine から呼ばれても、
// 順番に時刻の枠を1つずつ取っていくので、全体で interval に1回になる。
type pacer struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
	now      func() time.Time
}

func newPacer(interval time.Duration) *pacer {
	return &pacer{interval: interval, now: time.Now}
}

func (p *pacer) wait(ctx context.Context) error {
	p.mu.Lock()
	now := p.now()
	at := p.next
	if at.Before(now) {
		at = now
	}
	p.next = at.Add(p.interval)
	p.mu.Unlock()

	d := at.Sub(now)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ここから下は本物の DB と外部サービス。

type sqlNotificationStore struct {
	db *sql.DB
}

// slotColumns は時間帯ごとの設定の列名。列名は ? で渡せない（値ではなく識別子なので）。
// 利用者の入力ではなく、この固定の名前だけを SQL に埋め込む。
var slotColumns = map[apischema.NotificationSlot][2]string{
	apischema.NotificationSlotMorning: {"morningEnabled", "lineMorningEnabled"},
	apischema.NotificationSlotEvening: {"eveningEnabled", "lineEveningEnabled"},
}

// findRecipients は Node と同じく SQL を3本に分ける。ユーザーから見て予定と実績はどちらも1対多なので、
// 1本の JOIN にすると（予定の数 × 実績の数）の行に膨らみ、学習時間が重複して数えられる。
func (st *sqlNotificationStore) findRecipients(ctx context.Context, slot apischema.NotificationSlot, start, end time.Time) ([]recipient, error) {
	cols, ok := slotColumns[slot]
	if !ok {
		return nil, fmt.Errorf("unknown slot %q", slot)
	}
	// 通知設定の無いユーザーは対象外なので、設定とは内部結合（JOIN）。
	// LINE は未連携でもメールだけ受け取れるので、外部結合（LEFT JOIN）。
	// #nosec G202 -- 列名は slotColumns に書いた固定の名前だけ。値は渡していない
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.email, u.name, u.nickname,
		        np.morningEnabled, np.eveningEnabled, np.lineMorningEnabled, np.lineEveningEnabled,
		        lc.lineUserId
		 FROM `+"`user`"+` AS u
		 JOIN NotificationPreference AS np ON np.userId = u.id
		 LEFT JOIN LineConnection AS lc ON lc.userId = u.id
		 WHERE np.`+cols[0]+` = TRUE OR np.`+cols[1]+` = TRUE
		 ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	var users []recipient
	byID := map[string]int{}
	for rows.Next() {
		var u recipient
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Nickname,
			&u.Morning, &u.Evening, &u.LineMorn, &u.LineEven, &u.LineUserID); err != nil {
			rows.Close()
			return nil, err
		}
		byID[u.ID] = len(users)
		users = append(users, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// IN () は空だと SQL の構文エラーになるので、先に抜ける。
	if len(users) == 0 {
		return nil, nil
	}

	// IN (?, ?, …) の ? を人数ぶん並べる（ドライバは配列を展開しない）。
	in := strings.TrimSuffix(strings.Repeat("?,", len(users)), ",")
	args := make([]any, 0, len(users)+2)
	for _, u := range users {
		args = append(args, u.ID)
	}
	args = append(args, start, end)

	// #nosec G202 -- in は人数ぶん並べた ? だけ。値は args で渡す
	planRows, err := st.db.QueryContext(ctx,
		`SELECT p.userId, p.done, p.content, p.subject, t.name AS textbookName
		 FROM StudyPlan AS p
		 LEFT JOIN Textbook AS t ON t.id = p.textbookId
		 WHERE p.userId IN (`+in+`) AND p.date >= ? AND p.date < ?
		 ORDER BY p.id`, args...)
	if err != nil {
		return nil, err
	}
	for planRows.Next() {
		var (
			userID string
			p      planSummary
		)
		if err := planRows.Scan(&userID, &p.Done, &p.Content, &p.Subject, &p.TextbookName); err != nil {
			planRows.Close()
			return nil, err
		}
		u := &users[byID[userID]]
		u.Plans = append(u.Plans, p)
	}
	planRows.Close()
	if err := planRows.Err(); err != nil {
		return nil, err
	}

	// #nosec G202 -- in は人数ぶん並べた ? だけ。値は args で渡す
	logRows, err := st.db.QueryContext(ctx,
		`SELECT userId, minutes FROM StudyLog
		 WHERE userId IN (`+in+`) AND date >= ? AND date < ?
		 ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer logRows.Close()
	for logRows.Next() {
		var (
			userID  string
			minutes int64
		)
		if err := logRows.Scan(&userID, &minutes); err != nil {
			return nil, err
		}
		u := &users[byID[userID]]
		u.LogMinutes = append(u.LogMinutes, minutes)
	}
	return users, logRows.Err()
}

// 送った印の書き込みは持ち主（internal/write/notification）の操作を呼ぶ（JUK-154）。
func (st *sqlNotificationStore) markDelivery(ctx context.Context, userID string, date time.Time, slot apischema.NotificationSlot, channel deliveryChannel) (int64, bool, error) {
	return notification.MarkDelivery(ctx, st.db,
		notification.Delivery{UserID: userID, Date: date, Slot: string(slot), Channel: string(channel)}, time.Now().UTC())
}

func (st *sqlNotificationStore) unmarkDelivery(ctx context.Context, id int64) error {
	return notification.UnmarkDelivery(ctx, st.db, id)
}

// HTTPMessenger は Resend と LINE の API を直接呼ぶ（どちらも SDK は使わない）。
type HTTPMessenger struct {
	Client     *http.Client
	ResendBase string // 既定は https://api.resend.com。手元の比較では偽のサーバーへ向ける（RESEND_BASE_URL、Node の SDK と同じ名前）
	ResendKey  string
	LineBase   string // 既定は https://api.line.me/v2/bot
	LineToken  string
}

func (m *HTTPMessenger) sendEmail(ctx context.Context, to string, msg dailyMessage) error {
	if m.ResendKey == "" {
		return errors.New("RESEND_API_KEY is not configured")
	}
	return m.post(ctx, m.ResendBase+"/emails", m.ResendKey, map[string]any{
		"from": notificationFrom, "to": to, "subject": msg.Subject, "html": msg.HTML, "text": msg.Text,
	})
}

func (m *HTTPMessenger) pushLine(ctx context.Context, lineUserID, text string) error {
	if m.LineToken == "" {
		return errors.New("LINE_CHANNEL_ACCESS_TOKEN is not configured")
	}
	return m.post(ctx, m.LineBase+"/message/push", m.LineToken, map[string]any{
		"to": lineUserID, "messages": []map[string]string{{"type": "text", "text": text}},
	})
}

func (m *HTTPMessenger) post(ctx context.Context, url, token string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	// 相手が応答を返さないまま接続を保つと、こちらも返らない。1回ごとに上限を付ける。
	ctx, cancel := context.WithTimeout(ctx, httpx.ExternalTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := m.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("%s %d: %s", url, res.StatusCode, detail)
	}
	// 本文を読み切ると、接続を使い回せる。
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}

// CronHandler は POST /api/cron/daily-study-notifications。トークンの確認はルーター（rt.Job）が行う。
type CronHandler struct {
	notifier *dailyNotifier
	now      func() time.Time
}

// NewCronHandler は毎日の通知の入口を組み立てる。送信先は m（本番は HTTPMessenger）。
func NewCronHandler(db *sql.DB, m Messenger) *CronHandler {
	return &CronHandler{notifier: newDailyNotifier(&sqlNotificationStore{db: db}, m), now: time.Now}
}

func (h *CronHandler) DailyNotifications(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Slot apischema.NotificationSlot `json:"slot"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil ||
		(body.Slot != apischema.NotificationSlotMorning && body.Slot != apischema.NotificationSlotEvening) {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid slot")
		return
	}

	// 全員へ送り終えるまで、ふだんのリクエストの上限（internal/app の requestTimeout・WriteTimeout）より長くかかる。
	// 呼び出し元（curl）が切れても送るのは続けたいので、リクエストの context から切り離して別の上限を付ける。
	// （テストの httptest.ResponseRecorder は書き込みの期限を持たないので、ErrNotSupported は気にしない）
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(notificationBudget + 10*time.Second))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		httpx.InternalError(w, r, fmt.Errorf("daily-notification: %w", err))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), notificationBudget)
	defer cancel()

	summary, err := h.notifier.send(ctx, body.Slot, h.now())
	// 1回の実行で何通送れて何通失敗したかを残す。失敗が続いていないかを後から追える。
	logAttrs := slog.Group("notification",
		"date", summary.Date, "slot", summary.Slot, "eligible", summary.Eligible,
		"sent", summary.Sent, "skipped", summary.Skipped, "failed", summary.Failed)
	if err != nil {
		slog.ErrorContext(r.Context(), "[daily-notification] Run stopped.", logAttrs)
		httpx.InternalError(w, r, fmt.Errorf("daily-notification: %w", err))
		return
	}
	slog.InfoContext(r.Context(), "[daily-notification] Run finished.", logAttrs)
	httpx.WriteJSON(w, http.StatusOK, summary)
}
