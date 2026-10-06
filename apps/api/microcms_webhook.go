package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// microCMS で記事を公開・更新・削除したら、本番の記事（ビルド時に SSG したもの、JUK-110）を作り直す（JUK-112）。
// microCMS の「カスタム通知」の Webhook を受け、署名を確かめてから GitHub の API で deploy.yml を動かす。
//
// microCMS 公式の GitHub Actions 連携を使わないのは、push できる強さのトークンを外のサービスに預けることになるため。
// ここでは「Actions の実行」だけの fine-grained トークンを、自分の AWS（Secrets Manager）にだけ置く。

// microcmsWebhookBodyLimit は本文の上限。本番の nginx の client_max_body_size の既定（1m）と同じにする。
// 本文には記事の更新前と更新後が丸ごと入るが、今の記事なら十分に収まる。
const microcmsWebhookBodyLimit = 1 << 20

// deployCoalesceWindow は、デプロイを動かしてから次に動かすまでの間。この間に来た通知は、
// 間が明けたときの1回にまとめる（記事をまとめて直すと、記事の数だけ通知が来るため）。
const deployCoalesceWindow = time.Minute

// microcmsWebhookConfig は microCMS の Webhook の設定。
type microcmsWebhookConfig struct {
	secret   string // MICROCMS_WEBHOOK_SECRET。空なら Webhook は必ず 401
	deployer deployer
}

// deployer は本番のデプロイ（deploy.yml）を動かす。テストでは偽物に差し替える。
type deployer interface {
	dispatch(ctx context.Context) error
}

// microcmsWebhookHandler は POST /api/webhooks/microcms。
type microcmsWebhookHandler struct {
	secret  string
	trigger *deployTrigger
}

// microcmsPayload は microCMS の Webhook の本文のうち、作り直すかを決めるのに使う項目。
// type は new・edit・delete。一括の操作では id と contents が null で来る。
type microcmsPayload struct {
	API      string `json:"api"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Contents *struct {
		Old *microcmsContent `json:"old"`
		New *microcmsContent `json:"new"`
	} `json:"contents"`
}

// microcmsContent は記事の1つの版。status は公開の状態（PUBLISH・DRAFT・PUBLISH_AND_DRAFT・CLOSED）、
// publishValue は公開中の中身で、公開していなければ null。下書きの保存では draftValue だけが変わる。
// どちらもキーが無ければ nil、null なら "null" のバイト列になる（形が分からないものを見分けるため RawMessage で受ける）。
type microcmsContent struct {
	Status       []string        `json:"status"`
	PublishValue json.RawMessage `json:"publishValue"`
}

func (h *microcmsWebhookHandler) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, microcmsWebhookBodyLimit))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "Payload too large")
		return
	}
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}
	// 署名は JSON として読む前の本文で確かめる（読み直した JSON は元のバイト列と一致しない）。
	if !verifyMicrocmsSignature(body, r.Header.Get("x-microcms-signature"), h.secret) {
		httpx.WriteError(w, http.StatusUnauthorized, "Invalid signature")
		return
	}
	var payload microcmsPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "Invalid payload")
		return
	}

	ctx := r.Context()
	if !payload.changesPublished() {
		slog.InfoContext(ctx, "[microcms-webhook] Published content unchanged. Skip deploy.",
			"api", payload.API, "contentId", payload.ID, "type", payload.Type)
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"deploy": "skipped"})
		return
	}
	result, err := h.trigger.request(ctx)
	if err != nil {
		// microCMS の管理画面の Webhook のログに失敗として残る。そのときは deploy.yml を手で動かす。
		slog.ErrorContext(ctx, "[microcms-webhook] Failed to dispatch deploy.", "err", err.Error(),
			"api", payload.API, "contentId", payload.ID, "type", payload.Type)
		httpx.WriteError(w, http.StatusBadGateway, "Failed to dispatch deploy")
		return
	}
	slog.InfoContext(ctx, "[microcms-webhook] Deploy requested.", "deploy", result,
		"api", payload.API, "contentId", payload.ID, "type", payload.Type)
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"deploy": result})
}

// changesPublished は、公開中の中身が変わったか（作り直すと本番の見た目が変わるか）を返す。
// 公開・公開中の記事の更新・公開の終了・削除で true、公開していない記事の下書きの保存や削除で false。
// 迷うものは作り直す側に倒す（余分なビルドは無害だが、取りこぼすと記事が出ない）。
//   - 中身の形が分からない（一括の操作で contents が null・項目が無い）→ true
//   - 公開中の記事の下書きの保存（PUBLISH → PUBLISH_AND_DRAFT、publishValue は同じ）→ false
func (p microcmsPayload) changesPublished() bool {
	if p.Contents == nil {
		return true
	}
	before, after := p.Contents.Old, p.Contents.New
	if !before.known() || !after.known() {
		return true
	}
	return before.public() != after.public() || !bytes.Equal(before.publishValue(), after.publishValue())
}

// known は、作り直すかを決められる形か。版が無い（新規の old・削除の new）か、2つの項目がそろっていること。
func (c *microcmsContent) known() bool {
	return c == nil || (c.Status != nil && c.PublishValue != nil)
}

// public は公開中か。PUBLISH_AND_DRAFT（公開中で、下書きもある）も公開中に数える。
func (c *microcmsContent) public() bool {
	if c == nil {
		return false
	}
	for _, s := range c.Status {
		if strings.HasPrefix(s, "PUBLISH") {
			return true
		}
	}
	return false
}

// publishValue は公開中の中身を返す。版が無いか、公開していなければ nil。
func (c *microcmsContent) publishValue() []byte {
	if c == nil || bytes.Equal(c.PublishValue, []byte("null")) {
		return nil
	}
	return c.PublishValue
}

// verifyMicrocmsSignature は x-microcms-signature を確かめる。microCMS は、本文の HMAC-SHA256 を
// 16進数の文字列にして送ってくる（LINE は base64 なので、形が違う）。
// hmac.Equal は一定時間で比べるので、何文字目まで合っているかを時間から当てられない。
func verifyMicrocmsSignature(body []byte, signature string, secret string) bool {
	if secret == "" || signature == "" {
		return false
	}
	got, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}

// deployTrigger は、続けて来たデプロイの依頼を間引く。
//
//   - 前に動かしてから deployCoalesceWindow 以上たっていれば、すぐ動かす（"dispatched"）
//   - 間の中なら、間が明けたときに1回だけ動かす予約を入れる（"queued"）。予約が既にあれば何もしない
//
// 予約で後から動かすのは、間の中の更新を取りこぼさないため。deploy.yml は同じ時に1本しか走らず、
// 待っているものは最新の1本に置き換わるので、作り直しが走っている最中の依頼も、最後の状態で作り直される。
//
// 予約はこのプロセスのメモリにだけある。動かしてから間が明けるまで（1分）にデプロイで Go が入れ替わることは
// 無い（ビルドに数分かかる）が、ほかの理由で落ちれば予約は消える。そのときは deploy.yml を手で動かす。
type deployTrigger struct {
	deployer deployer
	window   time.Duration
	now      func() time.Time
	// afterFunc は time.AfterFunc。テストでは、時間を待たずに予約を動かせるものに差し替える。
	afterFunc func(time.Duration, func())

	mu      sync.Mutex
	last    time.Time // 最後に動かした時刻
	pending bool      // 間が明けたときに動かす予約があるか
}

func newDeployTrigger(d deployer) *deployTrigger {
	return &deployTrigger{
		deployer:  d,
		window:    deployCoalesceWindow,
		now:       time.Now,
		afterFunc: func(d time.Duration, f func()) { time.AfterFunc(d, f) },
	}
}

func (t *deployTrigger) request(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.pending {
		t.mu.Unlock()
		return "queued", nil
	}
	now := t.now()
	if wait := t.window - now.Sub(t.last); wait > 0 {
		t.pending = true
		t.mu.Unlock()
		t.afterFunc(wait, t.dispatchQueued)
		return "queued", nil
	}
	t.last = now
	t.mu.Unlock()

	if err := t.deployer.dispatch(ctx); err != nil {
		// 動かせなかったので、次の依頼は待たずに動かせるようにする。
		t.mu.Lock()
		t.last = time.Time{}
		t.mu.Unlock()
		return "", err
	}
	return "dispatched", nil
}

// dispatchQueued は予約していた1回を動かす。依頼したリクエストはもう返し終えているので、
// 失敗はログに残すだけ（Grafana の Loki で拾う）。
func (t *deployTrigger) dispatchQueued() {
	t.mu.Lock()
	t.pending = false
	t.last = t.now()
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), externalTimeout)
	defer cancel()
	if err := t.deployer.dispatch(ctx); err != nil {
		slog.Error("[microcms-webhook] Failed to dispatch queued deploy.", "err", err.Error())
	}
}

// githubWorkflowDispatcher は GitHub の API で、main の deploy.yml を workflow_dispatch で動かす。
// トークンは fine-grained で、このリポジトリの「Actions: Read and write」だけを持たせる。
type githubWorkflowDispatcher struct {
	client *http.Client
	// apiBase は GitHub の API の根元（既定 https://api.github.com）。テストでは偽のサーバーへ向ける。
	apiBase  string
	repo     string // owner/name
	workflow string // ワークフローのファイル名
	ref      string
	token    string // GITHUB_DEPLOY_TOKEN
}

func (g *githubWorkflowDispatcher) dispatch(ctx context.Context) error {
	if g.token == "" {
		return errors.New("GITHUB_DEPLOY_TOKEN is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, externalTimeout)
	defer cancel()
	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/dispatches", g.apiBase, g.repo, g.workflow)
	body := fmt.Sprintf(`{"ref":%q}`, g.ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	res, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// 受け付けると 204（本文なし）。
	if res.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(res.Body, 300))
		return fmt.Errorf("GitHub API %d: %s", res.StatusCode, detail)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	return nil
}
