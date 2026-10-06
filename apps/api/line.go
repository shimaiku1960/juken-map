package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/notification"
)

// LINE 連携（JUK-79）。Node の routes/line.ts と services/line-connection-service.ts にあたる。
//
//   - 連携の確認・解除：GET・DELETE /api/line/connection
//   - トークからの連携（Account Link）：Webhook で「連携」と届く → リンクを返信 → 画面で
//     POST /api/line/account-link → LINE の画面 → Webhook の accountLink イベントで確定
//   - プロフィールからの連携（LINE Login）：GET /api/line/oauth/start → LINE の同意画面 → /callback
//
// /line/settings（LINE のメッセージが案内する行き先）はページの振り分けなので Node に残す。
//
// DB の書き込み（連携・解除・nonce・試行・Webhook の印）は持ち主の internal/write/notification にある（JUK-154）。

const (
	// lineCallbackPath は LINE Login の戻り先。LINE Developers に登録した URL と同じでないと LINE が断る。
	lineCallbackPath = "/api/line/oauth/callback"
	// lineConfirmationTimeout は、連携できたことを LINE へ送るのを待つ上限。
	// 送れなくても連携は済んでいるので、利用者を待たせ続けない。
	lineConfirmationTimeout = 3 * time.Second
	// lineWebhookBodyLimit は Webhook の本文の上限。LINE は1回に複数のイベントをまとめて送るが、1MB あれば十分。
	lineWebhookBodyLimit = 1 << 20
)

var lineConnectionCompletedMessage = strings.Join([]string{
	"受験マップとのLINE連携が完了しました！",
	"",
	"朝・夜の通知は、受験マップのプロフィールから設定できます。",
	siteURL + "/line/settings",
}, "\n")

// accountLinkResult は Account Link の nonce で連携を確定した結果（持ち主の notification.LinkResult と同じ値）。
type accountLinkResult string

const (
	accountLinkLinked  = accountLinkResult(notification.Linked)
	accountLinkTaken   = accountLinkResult(notification.Taken)   // その LINE は別のアカウントに連携済み
	accountLinkExpired = accountLinkResult(notification.Expired) // nonce が無い・期限切れ・使用済み
)

// oauthAttempt は LINE Login を始めたときに DB へ残す値。戻ってきたときに state で引き当てる。
type oauthAttempt struct {
	UserID       string
	Nonce        string
	CodeVerifier string
	RedirectURI  string
	Expired      bool // 期限は DB の時計（UTC）で判定する
}

// lineStore は LINE 連携の DB の読み書き。テストでは偽物を渡す（Go の CI には DB が無い）。
type lineStore interface {
	isConnected(ctx context.Context, userID string) (bool, error)
	isLineUserConnected(ctx context.Context, lineUserID string) (bool, error)
	issueLinkNonce(ctx context.Context, userID, nonce string) error
	completeAccountLink(ctx context.Context, nonce, lineUserID string) (accountLinkResult, error)
	disconnect(ctx context.Context, userID string) error
	startOAuthAttempt(ctx context.Context, state string, a oauthAttempt) error
	findOAuthAttempt(ctx context.Context, state string) (*oauthAttempt, error)
	discardOAuthAttempt(ctx context.Context, state string) error
	// linkVerifiedLineUser は LINE Login で確かめた LINE と連携する。false は別のアカウントに連携済み。
	linkVerifiedLineUser(ctx context.Context, userID, lineUserID string) (bool, error)
	// markWebhookEvent は処理する前のイベントに印を入れる。同じ ID が既にあれば duplicate が true。
	markWebhookEvent(ctx context.Context, eventID string) (duplicate bool, err error)
	unmarkWebhookEvent(ctx context.Context, eventID string) error
}

type lineHandlers struct {
	store         lineStore
	line          lineClient
	channelSecret string // LINE_CHANNEL_SECRET（Webhook の署名）
	// webOrigin は画面のオリジン。本番は nginx で API と同じ（siteURL）。手元は Vite（WEB_ORIGIN）。
	webOrigin string
}

// connection は GET /api/line/connection。プロフィール画面が連携の有無を出すのに使う。
func (h *lineHandlers) connection(w http.ResponseWriter, r *http.Request, s *session) {
	connected, err := h.store.isConnected(r.Context(), s.UserID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, apischema.LineConnectionStatus{Connected: connected})
}

// disconnect は DELETE /api/line/connection。LINE 通知の設定も一緒に落とす。
func (h *lineHandlers) disconnect(w http.ResponseWriter, r *http.Request, s *session) {
	if err := h.store.disconnect(r.Context(), s.UserID); err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, apischema.LineConnectionStatus{Connected: false})
}

// accountLink は POST /api/line/account-link。トークのリンクから開いた画面が、ログインしたあとに呼ぶ。
// 使い捨ての nonce をこの利用者に結びつけ、LINE の連携画面の URL を返す。
// LINE はそのあと Webhook の accountLink イベントで同じ nonce を送ってくる（completeAccountLink）。
func (h *lineHandlers) accountLink(w http.ResponseWriter, r *http.Request, s *session) {
	var body struct {
		LinkToken *string `json:"linkToken"`
	}
	// Node の z.object({ linkToken: z.string().min(1).max(255) }) と同じ条件。文言も同じ。
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil ||
		body.LinkToken == nil || len(*body.LinkToken) == 0 || len([]rune(*body.LinkToken)) > 255 {
		writeError(w, http.StatusBadRequest, "連携情報が正しくありません")
		return
	}

	nonce := randomToken(32)
	if err := h.store.issueLinkNonce(r.Context(), s.UserID, nonce); err != nil {
		internalError(w, r, err)
		return
	}
	redirect := url.URL{Scheme: "https", Host: "access.line.me", Path: "/dialog/bot/accountLink"}
	redirect.RawQuery = url.Values{"linkToken": {*body.LinkToken}, "nonce": {nonce}}.Encode()
	writeJSON(w, http.StatusOK, map[string]string{"redirectUrl": redirect.String()})
}

// notificationSettingsPath はプロフィールの通知設定の場所。
const notificationSettingsPath = "/profile#notification-settings"

// settings は GET /line/settings。LINE のメッセージ本文が案内する入口で、画面を持たずにログイン状態で行き先を変えるだけ
// （JUK-111 で Node の routes/line.ts から移した）。SPA のルートにしないのは、描画が要らず、画面で判定すると一瞬ちらつくため。
func (h *lineHandlers) settings(w http.ResponseWriter, r *http.Request, s *session) {
	if s != nil {
		http.Redirect(w, r, h.webOrigin+notificationSettingsPath, http.StatusFound)
		return
	}
	http.Redirect(w, r, h.webOrigin+"/login?"+url.Values{"callbackURL": {notificationSettingsPath}}.Encode(), http.StatusFound)
}

// oauthStart は GET /api/line/oauth/start。プロフィールの「LINE と連携する」から画面遷移で来る。
func (h *lineHandlers) oauthStart(w http.ResponseWriter, r *http.Request, s *session) {
	if s == nil {
		http.Redirect(w, r, h.webOrigin+"/login?callbackURL=%2Fprofile%23line-connection", http.StatusFound)
		return
	}

	state, nonce, verifier := randomToken(32), randomToken(32), oauth2.GenerateVerifier()
	// 戻り先は固定（C4）。リクエストの値から組み立てない。
	redirectURI := h.webOrigin + lineCallbackPath
	authURL, err := h.line.authCodeURL(state, nonce, verifier, redirectURI)
	if err == nil {
		err = h.store.startOAuthAttempt(r.Context(), state, oauthAttempt{
			UserID: s.UserID, Nonce: nonce, CodeVerifier: verifier, RedirectURI: redirectURI,
		})
	}
	if err != nil {
		slog.ErrorContext(r.Context(), "[line-oauth] Failed to start LINE Login.", "err", err.Error())
		h.profileRedirect(w, r, "unavailable")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// oauthCallback は GET /api/line/oauth/callback。LINE の同意画面から戻ってくる。
// 結果はプロフィール画面へ ?line=<結果> で知らせる（画面が文言を出し分ける）。
func (h *lineHandlers) oauthCallback(w http.ResponseWriter, r *http.Request, s *session) {
	query := parseQuery(r.URL.RawQuery)
	// 利用者が同意画面でキャンセルすると、LINE は error を付けて戻す。
	if _, ok := query["error"]; ok {
		h.profileRedirect(w, r, "cancelled")
		return
	}
	state, code := singleValue(query, "state"), singleValue(query, "code")
	if state == "" || code == "" {
		h.profileRedirect(w, r, "invalid")
		return
	}

	// LINE の画面にいる間にセッションが切れた。state は捨てずに、ログインしたらこの URL へ戻す。
	if s == nil {
		callback := lineCallbackPath + "?" + url.Values{"state": {state}, "code": {code}}.Encode()
		http.Redirect(w, r, h.webOrigin+"/login?callbackURL="+url.QueryEscape(callback), http.StatusFound)
		return
	}

	ctx := r.Context()
	attempt, err := h.store.findOAuthAttempt(ctx, state)
	if err != nil {
		internalError(w, r, err)
		return
	}
	// 無い・別の人の・期限切れの state は使えない（C4）。見つかった state は、使えなくても捨てる。
	if attempt == nil {
		h.profileRedirect(w, r, "expired")
		return
	}
	if err := h.store.discardOAuthAttempt(ctx, state); err != nil {
		internalError(w, r, err)
		return
	}
	if attempt.UserID != s.UserID || attempt.Expired {
		h.profileRedirect(w, r, "expired")
		return
	}

	result, err := h.completeOAuth(ctx, s, attempt, code)
	if err != nil {
		slog.ErrorContext(ctx, "[line-oauth] Failed to complete LINE Login.", "err", err.Error())
		h.profileRedirect(w, r, "failed")
		return
	}
	h.profileRedirect(w, r, result)
}

// completeOAuth はコードをトークンに換え、LINE のアカウントを確かめて連携する。戻り値は画面へ知らせる結果。
func (h *lineHandlers) completeOAuth(ctx context.Context, s *session, attempt *oauthAttempt, code string) (string, error) {
	tokens, err := h.line.exchangeCode(ctx, code, attempt.CodeVerifier, attempt.RedirectURI)
	if err != nil {
		return "", err
	}
	// ID トークンの確認と友だち状態は互いに関係しないので、同時に LINE へ問い合わせる（Node の Promise.all と同じ）。
	// 順番に待つと、1リクエストの上限（requestTimeout）に LINE の待ち時間が積み上がる。
	var identity lineIdentity
	var friend bool
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		identity, err = h.line.verifyIDToken(gctx, tokens.IDToken, attempt.Nonce)
		return err
	})
	g.Go(func() (err error) {
		friend, err = h.line.isFriend(gctx, tokens.AccessToken)
		return err
	})
	if err := g.Wait(); err != nil {
		return "", err
	}
	// LINE の verify も nonce を確かめるが、応答の nonce が始めたときの値と同じかをこちらでも見る。
	if identity.Sub == "" || identity.Nonce != attempt.Nonce {
		return "invalid", nil
	}
	// 友だちでないと、通知を送れない（ブロック中も同じ）。
	if !friend {
		return "friend-required", nil
	}
	linked, err := h.store.linkVerifiedLineUser(ctx, s.UserID, identity.Sub)
	if err != nil {
		return "", err
	}
	if !linked {
		return "already-used", nil
	}

	// 連携できたことをトークにも送る。送れなくても連携は済んでいるので、成功として画面へ戻す。
	pushCtx, cancel := context.WithTimeout(ctx, lineConfirmationTimeout)
	defer cancel()
	if err := h.line.pushText(pushCtx, identity.Sub, lineConnectionCompletedMessage); err != nil {
		slog.ErrorContext(ctx, "[line-oauth] LINE connection completed, but confirmation message failed.", "err", err.Error())
	}
	return "connected", nil
}

func (h *lineHandlers) profileRedirect(w http.ResponseWriter, r *http.Request, result string) {
	http.Redirect(w, r, h.webOrigin+"/profile?line="+result+"#line-connection", http.StatusFound)
}

// lineEvent は Webhook のイベントのうち、使う項目だけ。
type lineEvent struct {
	Type           string `json:"type"`
	WebhookEventID string `json:"webhookEventId"`
	ReplyToken     string `json:"replyToken"`
	Source         struct {
		UserID string `json:"userId"`
	} `json:"source"`
	Message struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"message"`
	Link struct {
		Result string `json:"result"`
		Nonce  string `json:"nonce"`
	} `json:"link"`
}

// webhook は POST /api/line/webhook。LINE のサーバーが、友だち追加・メッセージ・連携の結果を送ってくる。
//
// 署名は、JSON として読む前の本文で確かめる（C1）。同じイベントの再送は webhookEventId で見分けて、
// 2回目は何もしない。イベントごとの失敗はログに残して 200 を返す（Node と同じ。LINE に再送させても、
// 返信のトークンは1回しか使えないので、やり直しにならない）。
func (h *lineHandlers) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, lineWebhookBodyLimit))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "Payload too large")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	if !verifyLineSignature(body, r.Header.Get("x-line-signature"), h.channelSecret) {
		writeError(w, http.StatusUnauthorized, "Invalid signature")
		return
	}
	var payload struct {
		Events []lineEvent `json:"events"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		// 署名は合っているのに JSON として読めない。LINE がそんな本文を送ることは無いはず。
		writeError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	ctx := r.Context()
	for _, event := range payload.Events {
		if err := h.handleEvent(ctx, event); err != nil {
			slog.ErrorContext(ctx, "[line-webhook] Event processing failed.",
				"err", err.Error(), "eventType", event.Type, "webhookEventId", event.WebhookEventID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleEvent はイベント1つを、印を入れてから処理する。失敗したら印を消し、再送されたときにやり直せるようにする。
func (h *lineHandlers) handleEvent(ctx context.Context, event lineEvent) error {
	// webhookEventId は LINE が必ず付ける。無いものは見分けられないので、そのまま処理する。
	if event.WebhookEventID != "" {
		duplicate, err := h.store.markWebhookEvent(ctx, event.WebhookEventID)
		if err != nil {
			return err
		}
		if duplicate {
			slog.InfoContext(ctx, "[line-webhook] Duplicate event skipped.",
				"eventType", event.Type, "webhookEventId", event.WebhookEventID)
			return nil
		}
	}
	err := h.dispatch(ctx, event)
	if err != nil && event.WebhookEventID != "" {
		// 消せなくても、元の失敗のほうを返す（消せなかったことは、再送が飛ばされるだけで害は無い）。
		if unmarkErr := h.store.unmarkWebhookEvent(context.WithoutCancel(ctx), event.WebhookEventID); unmarkErr != nil {
			slog.ErrorContext(ctx, "[line-webhook] Failed to unmark event.", "err", unmarkErr.Error())
		}
	}
	return err
}

func (h *lineHandlers) dispatch(ctx context.Context, event lineEvent) error {
	switch {
	case event.Type == "accountLink":
		return h.completeAccountLink(ctx, event)
	case event.Type == "follow",
		event.Type == "message" && event.Message.Type == "text" && strings.TrimSpace(event.Message.Text) == "連携":
		return h.sendLinkGuide(ctx, event)
	case event.Type == "message" && event.ReplyToken != "":
		return h.line.replyText(ctx, event.ReplyToken, "受験マップとつなぐには「連携」と送ってください。")
	}
	return nil
}

// sendLinkGuide は、友だち追加や「連携」のメッセージに、連携用のリンクを返信する。
func (h *lineHandlers) sendLinkGuide(ctx context.Context, event lineEvent) error {
	lineUserID := event.Source.UserID
	if lineUserID == "" || event.ReplyToken == "" {
		return nil
	}
	connected, err := h.store.isLineUserConnected(ctx, lineUserID)
	if err != nil {
		return err
	}
	if connected {
		return h.line.replyText(ctx, event.ReplyToken,
			"受験マップとはすでに連携済みです。\n通知設定を確認する → "+siteURL+"/line/settings")
	}
	linkToken, err := h.line.issueLinkToken(ctx, lineUserID)
	if err != nil {
		return err
	}
	return h.line.replyText(ctx, event.ReplyToken,
		"受験マップとLINEを連携します。次のリンクを10分以内に開いてログインしてください。\n"+lineAccountLinkURL(linkToken))
}

// completeAccountLink は、LINE の連携画面を通ったあとに届く accountLink イベントで連携を確定する。
func (h *lineHandlers) completeAccountLink(ctx context.Context, event lineEvent) error {
	if event.Link.Result != "ok" || event.Link.Nonce == "" || event.Source.UserID == "" {
		return nil
	}
	result, err := h.store.completeAccountLink(ctx, event.Link.Nonce, event.Source.UserID)
	if err != nil {
		return err
	}
	if event.ReplyToken == "" {
		return nil
	}
	var text string
	switch result {
	case accountLinkExpired:
		text = "連携リンクの期限が切れました。「連携」と送って、もう一度お試しください。"
	case accountLinkLinked:
		text = "受験マップとの連携が完了しました。\n通知設定を続ける → " + siteURL + "/line/settings"
	default:
		text = "このLINEは別の受験マップアカウントに連携済みです。以前のアカウントでLINE連携を解除してから、もう一度お試しください。"
	}
	return h.line.replyText(ctx, event.ReplyToken, text)
}

// singleValue はクエリの key の値。無いときと、2つ以上あって決められないときは空文字。
func singleValue(q map[string][]string, key string) string {
	if len(q[key]) != 1 {
		return ""
	}
	return q[key][0]
}

// randomToken は n バイトの乱数を base64url（= なし）にした文字列。Node の randomBytes(n).toString("base64url")。
func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b) // crypto/rand の Read は失敗しない
	return base64.RawURLEncoding.EncodeToString(b)
}

// ここから下は本物の DB。

type sqlLineStore struct {
	db *sql.DB
}

func (st *sqlLineStore) isConnected(ctx context.Context, userID string) (bool, error) {
	return st.exists(ctx, "SELECT 1 FROM LineConnection WHERE userId = ? LIMIT 1", userID)
}

func (st *sqlLineStore) isLineUserConnected(ctx context.Context, lineUserID string) (bool, error) {
	return st.exists(ctx, "SELECT 1 FROM LineConnection WHERE lineUserId = ? LIMIT 1", lineUserID)
}

func (st *sqlLineStore) exists(ctx context.Context, query string, arg any) (bool, error) {
	var one int
	err := st.db.QueryRowContext(ctx, query, arg).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("line: %w", err)
	}
	return true, nil
}

func (st *sqlLineStore) issueLinkNonce(ctx context.Context, userID, nonce string) error {
	return notification.IssueLinkNonce(ctx, st.db, userID, nonce, time.Now())
}

func (st *sqlLineStore) completeAccountLink(ctx context.Context, nonce, lineUserID string) (accountLinkResult, error) {
	r, err := notification.CompleteAccountLink(ctx, st.db, nonce, lineUserID, time.Now())
	return accountLinkResult(r), err
}

func (st *sqlLineStore) disconnect(ctx context.Context, userID string) error {
	return notification.Disconnect(ctx, st.db, userID, time.Now())
}

func (st *sqlLineStore) startOAuthAttempt(ctx context.Context, state string, a oauthAttempt) error {
	return notification.StartOAuthAttempt(ctx, st.db, state, notification.Attempt{
		UserID: a.UserID, Nonce: a.Nonce, CodeVerifier: a.CodeVerifier, RedirectURI: a.RedirectURI,
	}, time.Now())
}

func (st *sqlLineStore) findOAuthAttempt(ctx context.Context, state string) (*oauthAttempt, error) {
	var a oauthAttempt
	err := st.db.QueryRowContext(ctx,
		`SELECT userId, nonce, codeVerifier, redirectUri, expiresAt <= UTC_TIMESTAMP(3)
		 FROM LineOAuthAttempt WHERE state = ?`, state,
	).Scan(&a.UserID, &a.Nonce, &a.CodeVerifier, &a.RedirectURI, &a.Expired)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find line oauth attempt: %w", err)
	}
	return &a, nil
}

func (st *sqlLineStore) discardOAuthAttempt(ctx context.Context, state string) error {
	return notification.DiscardOAuthAttempt(ctx, st.db, state)
}

func (st *sqlLineStore) linkVerifiedLineUser(ctx context.Context, userID, lineUserID string) (bool, error) {
	return notification.LinkVerifiedLineUser(ctx, st.db, userID, lineUserID, time.Now())
}

func (st *sqlLineStore) markWebhookEvent(ctx context.Context, eventID string) (bool, error) {
	return notification.MarkWebhookEvent(ctx, st.db, eventID, time.Now())
}

func (st *sqlLineStore) unmarkWebhookEvent(ctx context.Context, eventID string) error {
	return notification.UnmarkWebhookEvent(ctx, st.db, eventID)
}
