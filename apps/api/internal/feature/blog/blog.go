// Package blog はブログ（microCMS の記事）の入口。記事の中継（blog.go）と、記事を公開したときに
// 画面を作り直す Webhook（microcms_webhook.go）。
package blog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// ブログの記事の中継（GET /api/blog・/api/blog/{id}。JUK-111 で Node の routes/blog.ts から移した）。
// microCMS の API キーをサーバー側で使うため、SPA から直接は叩けない。ここで中継し、キーはサーバーに閉じたままにする。
// 認証は不要（公開コンテンツ）。記事のページの HTML はビルドで作り置くので（JUK-110）、ここは画面からだけ使われる。

// microcmsTimeout は microCMS を待つ上限。画面が記事を取るときに長く待たせないよう短めにする（Node と同じ3秒）。
const microcmsTimeout = 3 * time.Second

// maxBlogBody は microCMS の応答として読む大きさの上限。記事の一覧は本文込みで10件なので、数MB あれば足りる。
const maxBlogBody = 8 << 20

type Config struct {
	ServiceDomain string // MICROCMS_SERVICE_DOMAIN（xxx.microcms.io の xxx）
	APIKey        string // MICROCMS_API_KEY
	// APIBase は microCMS の API の根元。空なら https://<serviceDomain>.microcms.io/api/v1。テストで差し替える。
	APIBase string
	Client  *http.Client
}

type blogHandlers struct {
	base   string
	apiKey string
	client *http.Client
}

func newBlogHandlers(c Config) *blogHandlers {
	base := c.APIBase
	if base == "" && c.ServiceDomain != "" {
		base = "https://" + c.ServiceDomain + ".microcms.io/api/v1"
	}
	client := c.Client
	if client == nil {
		client = &http.Client{}
	}
	return &blogHandlers{base: base, apiKey: c.APIKey, client: client}
}

// RegisterRoutes はブログの記事の中継を登録する。
func RegisterRoutes(rt *httpx.Router, c Config) {
	blog := newBlogHandlers(c)
	rt.Public("GET /api/blog", blog.list)
	rt.Public("GET /api/blog/{id}", blog.detail)
}

// errBlogNotFound は microCMS が 404 を返したこと（記事が存在しない）。
var errBlogNotFound = errors.New("microcms: not found")

func (h *blogHandlers) list(w http.ResponseWriter, r *http.Request) {
	body, err := h.fetch(r.Context(), "/blogs")
	if err != nil {
		// 一覧の 404 は「記事が無い」ではなく設定の誤り（エンドポイント名など）なので、ほかの失敗と同じく 502。
		upstreamFailed(w, r, err)
		return
	}
	writeRawJSON(w, r, body)
}

func (h *blogHandlers) detail(w http.ResponseWriter, r *http.Request) {
	body, err := h.fetch(r.Context(), "/blogs/"+url.PathEscape(r.PathValue("id")))
	// 404 だけを「記事が無い」として扱う。タイムアウトや microCMS の障害まで 404 にすると、
	// こちらの調査でも利用者の画面でも原因を取り違える。
	if errors.Is(err, errBlogNotFound) {
		httpx.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "Not found"})
		return
	}
	if err != nil {
		upstreamFailed(w, r, err)
		return
	}
	writeRawJSON(w, r, body)
}

// fetch は microCMS の API を GET し、2xx なら本文をそのまま返す。
func (h *blogHandlers) fetch(ctx context.Context, path string) ([]byte, error) {
	if h.base == "" || h.apiKey == "" {
		return nil, errors.New("microcms: MICROCMS_SERVICE_DOMAIN と MICROCMS_API_KEY が設定されていません")
	}
	ctx, cancel := context.WithTimeout(ctx, microcmsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-MICROCMS-API-KEY", h.apiKey)
	res, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, errBlogNotFound
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, fmt.Errorf("microcms: status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBlogBody+1))
	if err != nil {
		return nil, err
	}
	// 途中で切った JSON を返すと画面が壊れるので、上限を超えたら失敗にする。
	if len(body) > maxBlogBody {
		return nil, fmt.Errorf("microcms: 応答が %d バイトを超えた", maxBlogBody)
	}
	return body, nil
}

// upstreamFailed は microCMS 側の失敗を 502 で返す。500 にすると「こちらのバグ」と区別が付かず、
// 可観測性で見るエラー率にも、自分の不具合と外部の障害が混ざってしまう。
func upstreamFailed(w http.ResponseWriter, r *http.Request, err error) {
	slog.ErrorContext(r.Context(), "[blog] microCMS request failed.", "err", err.Error())
	httpx.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "Bad Gateway"})
}

// writeRawJSON は microCMS の応答を中身を変えずに返す（Node は SDK が読んだ値を JSON に戻していたので、中身は同じ）。
// 一覧は本文込みで大きいので、受け付けるなら gzip にする。
func writeRawJSON(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	httpx.WriteMaybeGzip(w, r, http.StatusOK, body)
}
