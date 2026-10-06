package main

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 画面の配信のテスト（Node の spa.test.ts・seo.test.ts・security-headers.test.ts から移した）。
// apps/web のビルド成果物（dist）と同じ形を一時ディレクトリに作って読ませる。

const testIndexHTML = `<html><head><title>x</title></head><body><div id="root"></div></body></html>`

func writeTestDist(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.html":            testIndexHTML,
		"ssg/terms.html":        `<html><head><title>x</title></head><body><div id="root"><h1>利用規約</h1></div></body></html>`,
		"ssg/articles/abc.html": `<html><head><title>x</title></head><body><div id="root"><h1>記事のタイトル</h1></div></body></html>`,
		"assets/index-abc.js":   strings.Repeat("console.log(1);", 200),
		"robots.txt":            "User-agent: *\n",
		"opengraph-image.png":   "\x89PNG\r\n\x1a\n",
	}
	meta := map[string]pageMeta{
		"/articles/abc": {
			Title:         "記事のタイトル｜受験マップ",
			Description:   "記事の説明",
			Canonical:     "https://juken-map.com/articles/abc",
			OGTitle:       "記事のタイトル",
			OGType:        "article",
			OGImage:       "https://juken-map.com/opengraph-image.png",
			PublishedTime: "2026-08-01T00:00:00.000Z",
			ModifiedTime:  "2026-08-03T00:00:00.000Z",
		},
	}
	metaJSON, _ := json.Marshal(meta)
	files["ssg/meta.json"] = string(metaJSON)
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func newSPATestRouter(t *testing.T, scripts pageScripts) *httpx.Router {
	t.Helper()
	site, err := loadSPA(writeTestDist(t), scripts)
	if err != nil || site == nil {
		t.Fatalf("loadSPA: %v", err)
	}
	rt := httpx.NewRouter(fakeSessions(nil))
	rt.Public("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	registerSPA(rt, site)
	return rt
}

func spaGet(rt *httpx.Router, target string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	// 本番と同じく、外側の securityHeaders（API 向けの狭い CSP）を通す。HTML はこれを上書きする。
	securityHeaders(rt).ServeHTTP(rec, req)
	return rec
}

func TestLoadSPAWithoutDist(t *testing.T) {
	// WEB_DIST_DIR が無い・index.html が無いときは画面を配らない（開発は Vite が配る）
	for _, root := range []string{"", t.TempDir()} {
		site, err := loadSPA(root, pageScripts{})
		if site != nil || err != nil {
			t.Fatalf("loadSPA(%q) = %v, %v", root, site, err)
		}
	}
}

func TestSPAPrerenderedPages(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})

	// SSG したパスには本文入りの HTML を返し、meta も差し込む
	rec := spaGet(rt, "/terms", nil)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "<h1>利用規約</h1>") ||
		!strings.Contains(body, `<link rel="canonical" href="https://juken-map.com/terms"/>`) {
		t.Fatalf("status = %d, body = %s", rec.Code, body)
	}

	// SSG していないパスには、中身が空の index.html を返す
	rec = spaGet(rt, "/login", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("/login: status = %d, body = %s", rec.Code, rec.Body)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}

	// ssg/ のファイルは直接は配らない（meta の無い同じページが別 URL にできるため）
	for _, target := range []string{"/ssg/terms.html", "/ssg/meta.json"} {
		rec = spaGet(rt, target, nil)
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "利用規約") {
			t.Fatalf("%s: status = %d, body = %s", target, rec.Code, rec.Body)
		}
	}
}

func TestSPATokenLinkPages(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})
	// メールや LINE のリンクで開く画面は Referrer-Policy: no-referrer（認証基準 10 の D3）
	for _, target := range []string{"/verify-email/confirm?token=abc", "/reset-password?token=abc", "/line/link?linkToken=abc"} {
		rec := spaGet(rt, target, nil)
		if rec.Code != http.StatusOK || rec.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatalf("%s: status = %d, Referrer-Policy = %q", target, rec.Code, rec.Header().Get("Referrer-Policy"))
		}
	}
	// ほかの画面は既定（strict-origin-when-cross-origin）のまま
	if got := spaGet(rt, "/login", nil).Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Fatalf("/login: Referrer-Policy = %q", got)
	}
}

func TestSPAArticles(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})

	// 作り置いた本文入りの HTML に、ビルドが書き出した記事の meta を入れて返す
	rec := spaGet(rt, "/articles/abc", nil)
	body := rec.Body.String()
	for _, want := range []string{
		"<h1>記事のタイトル</h1>",
		"<title>記事のタイトル｜受験マップ</title>",
		`<link rel="canonical" href="https://juken-map.com/articles/abc"/>`,
		`<meta property="og:type" content="article"/>`,
		`<meta property="article:modified_time" content="2026-08-03T00:00:00.000Z"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("本文に %s が無い", want)
		}
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	// SSG に無い記事は 404（soft 404 にしない）。本文は空の index.html
	rec = spaGet(rt, "/articles/nope", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("/articles/nope: status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestSPASitemap(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})
	rec := spaGet(rt, "/sitemap.xml", nil)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/xml; charset=utf-8" {
		t.Fatalf("status = %d, Content-Type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	want := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://juken-map.com</loc>
    <changefreq>weekly</changefreq>
    <priority>1</priority>
  </url>
  <url>
    <loc>https://juken-map.com/blog</loc>
    <changefreq>weekly</changefreq>
    <priority>0.6</priority>
  </url>
  <url>
    <loc>https://juken-map.com/terms</loc>
    <changefreq>yearly</changefreq>
    <priority>0.3</priority>
  </url>
  <url>
    <loc>https://juken-map.com/privacy</loc>
    <changefreq>yearly</changefreq>
    <priority>0.3</priority>
  </url>
  <url>
    <loc>https://juken-map.com/articles/abc</loc>
    <lastmod>2026-08-03T00:00:00.000Z</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.5</priority>
  </url>
</urlset>`
	if got := rec.Body.String(); got != want {
		t.Fatalf("sitemap =\n%s\nwant\n%s", got, want)
	}
}

func TestSPAUnknownPaths(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})

	// App.tsx が持たないパスは、同じ HTML を 404 で返す（ボットのスキャンを「正常」に数えない）
	rec := spaGet(rt, "/wp-login.php", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `<div id="root"></div>`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	// 末尾の / は react-router と同じく同じ画面として扱う
	if rec := spaGet(rt, "/login/", nil); rec.Code != http.StatusOK {
		t.Fatalf("/login/: status = %d", rec.Code)
	}

	// 存在しない API は index.html ではなく JSON の 404
	rec = spaGet(rt, "/api/nope", nil)
	if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("/api/nope: status = %d, Content-Type = %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	// API の CSP は狭いまま（画面の CSP を付けない）
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; frame-ancestors 'none'" {
		t.Fatalf("/api/nope: CSP = %q", got)
	}
	// ほかのルートは今までどおり先に当たる
	if rec := spaGet(rt, "/api/health", nil); rec.Code != http.StatusOK {
		t.Fatalf("/api/health: status = %d", rec.Code)
	}
	// GET 以外で当たらないものは、画面ではなく JSON の 404
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	post := httptest.NewRecorder()
	rt.ServeHTTP(post, req)
	if post.Code != http.StatusNotFound || !strings.HasPrefix(post.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("POST /login: status = %d, Content-Type = %q", post.Code, post.Header().Get("Content-Type"))
	}
}

func TestSPAStaticFiles(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{})

	// ファイル名にハッシュが入る assets/* は永久キャッシュ。受け付けるなら作り置いた gzip を返す
	rec := spaGet(rt, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "gzip, br"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	h := rec.Header()
	if h.Get("Cache-Control") != "public, max-age=31536000, immutable" || h.Get("Content-Encoding") != "gzip" ||
		h.Get("Vary") != "Accept-Encoding" || !strings.HasPrefix(h.Get("Content-Type"), "text/javascript") {
		t.Fatalf("headers = %v", h)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if string(plain) != strings.Repeat("console.log(1);", 200) {
		t.Fatalf("gzip を戻した中身が違う")
	}

	// 同じ ETag で聞き直したら 304（本文を送らない）
	etag := h.Get("ETag")
	rec = spaGet(rt, "/assets/index-abc.js", map[string]string{"Accept-Encoding": "gzip", "If-None-Match": etag})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("If-None-Match: status = %d", rec.Code)
	}
	// gzip を受け付けなければ元のまま（ETag も別）
	rec = spaGet(rt, "/assets/index-abc.js", nil)
	if rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("ETag") == etag {
		t.Fatalf("gzip 無し: headers = %v", rec.Header())
	}

	// 小さいファイル・画像は圧縮しない。キャッシュは既定（public, max-age=0）
	for _, target := range []string{"/robots.txt", "/opengraph-image.png"} {
		rec = spaGet(rt, target, map[string]string{"Accept-Encoding": "gzip"})
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "" || rec.Header().Get("Cache-Control") != "public, max-age=0" {
			t.Fatalf("%s: status = %d, headers = %v", target, rec.Code, rec.Header())
		}
	}
	// index.html をファイルとして開いても、必ず再検証させる
	if got := spaGet(rt, "/index.html", nil).Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("/index.html: Cache-Control = %q", got)
	}
	// 途中のパスを ../ で戻しても、読み込んだ dist の外は読めない
	if rec := spaGet(rt, "/assets/../../etc/passwd", nil); rec.Code == http.StatusOK && !strings.Contains(rec.Body.String(), `<div id="root">`) {
		t.Fatalf("dist の外を返した: %s", rec.Body)
	}
}

func TestSPAPageHTMLIsGzipped(t *testing.T) {
	rt := newSPATestRouter(t, pageScripts{gaMeasurementID: "G-TEST123"})
	// meta を差し込んだ HTML は 1KB を超えるので、受け付けるなら gzip にする
	rec := spaGet(rt, "/", map[string]string{"Accept-Encoding": "gzip"})
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("status = %d, headers = %v", rec.Code, rec.Header())
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !strings.Contains(string(plain), "<title>今日の勉強を、合格までの積み重ねに。｜受験マップ</title>") {
		t.Fatalf("本文 = %s", plain)
	}
}

func TestSPAPageCSP(t *testing.T) {
	scripts := pageScripts{gaMeasurementID: "G-TEST123", faroCollectorURL: "https://faro.example.net/collect/key"}
	rt := newSPATestRouter(t, scripts)
	rec := spaGet(rt, "/login", nil)
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Fatalf("画面の CSP になっていない: %q", csp)
	}
	// Faro の送り先はオリジンだけを許す
	if !regexp.MustCompile(`connect-src 'self' https://faro\.example\.net `).MatchString(csp) {
		t.Fatalf("connect-src: %q", csp)
	}

	// GA4 のインラインスクリプトを、実際に差し込んだ中身のハッシュで許す
	inline := regexp.MustCompile(`(?s)<script>(.*?)</script>`).FindStringSubmatch(rec.Body.String())
	if inline == nil {
		t.Fatalf("GA4 のインラインスクリプトが無い: %s", rec.Body)
	}
	sum := sha256.Sum256([]byte(inline[1]))
	scriptSrc := regexp.MustCompile(`script-src ([^;]*)`).FindStringSubmatch(csp)[1]
	if !strings.Contains(scriptSrc, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'") {
		t.Fatalf("script-src にハッシュが無い: %q", scriptSrc)
	}
	// スクリプトはインラインを丸ごと許さない（スタイルは Sonner のために許している）
	if strings.Contains(scriptSrc, "'unsafe-inline'") {
		t.Fatalf("script-src = %q", scriptSrc)
	}
	// 設定が無ければ Faro も GA4 も入れない
	bare := pageScripts{}.pageCSP()
	if strings.Contains(bare, "faro") || strings.Contains(bare, "sha256") {
		t.Fatalf("CSP = %q", bare)
	}
}

func TestInjectMetaFaro(t *testing.T) {
	html := "<html><head><title>x</title></head><body></body></html>"
	meta := pageMeta{Title: "受験マップ", Description: "説明", OGTitle: "受験マップ", OGType: "website", OGImage: "https://juken-map.com/og.png"}

	// FARO_COLLECTOR_URL があれば meta で画面へ渡す
	got := pageScripts{faroCollectorURL: "https://faro.example/collect/abc"}.injectMeta(html, meta)
	if !strings.Contains(got, `<meta name="faro-collector-url" content="https://faro.example/collect/abc"/>`) {
		t.Fatalf("got %s", got)
	}
	// 無ければ出さない
	if got := (pageScripts{}).injectMeta(html, meta); strings.Contains(got, "faro-collector-url") {
		t.Fatalf("got %s", got)
	}
	// 値は属性として安全に埋め込む
	if got := (pageScripts{faroCollectorURL: `https://faro.example/"><script>`}).injectMeta(html, meta); strings.Contains(got, `"><script>`) {
		t.Fatalf("got %s", got)
	}
}

func TestInjectMetaMatchesNode(t *testing.T) {
	// Node の injectMeta（apps/api/src/seo.ts）が返していた形をそのまま固定する。差し込む順番・字下げも同じ。
	// GA4 の config だけは、トークンが載る画面で最初の page_view を送らないよう変えた（JUK-124）。
	got := pageScripts{gaMeasurementID: "G-1"}.injectMeta(
		"<html><head><title>x</title></head><body></body></html>", defaultMeta("/terms"))
	want := `<html><head><title>利用規約｜受験マップ</title>  <meta name="description" content="受験マップをご利用いただく際の条件を定めています。"/>
    <link rel="canonical" href="https://juken-map.com/terms"/>
    <meta property="og:site_name" content="受験マップ"/>
    <meta property="og:locale" content="ja_JP"/>
    <meta property="og:title" content="利用規約｜受験マップ"/>
    <meta property="og:description" content="受験マップをご利用いただく際の条件を定めています。"/>
    <meta property="og:type" content="website"/>
    <meta property="og:image" content="https://juken-map.com/opengraph-image.png"/>
    <meta property="og:url" content="https://juken-map.com/terms"/>
    <meta name="twitter:card" content="summary_large_image"/>
    <meta name="twitter:title" content="利用規約｜受験マップ"/>
    <meta name="twitter:description" content="受験マップをご利用いただく際の条件を定めています。"/>
    <meta name="twitter:image" content="https://juken-map.com/opengraph-image.png"/>
    <script async src="https://www.googletagmanager.com/gtag/js?id=G-1"></script>
    <script>
      window.dataLayer = window.dataLayer || [];
      function gtag(){dataLayer.push(arguments);}
      window.gtag = gtag;
      gtag('js', new Date());
      gtag('config', 'G-1', /[?&](token|linkToken)=/.test(location.search) ? { send_page_view: false } : {});
    </script>
  </head><body></body></html>`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDefaultMetaNoindex(t *testing.T) {
	for path, want := range map[string]bool{
		"/": false, "/terms": false, "/blog": false, "/articles/abc": false,
		"/login": true, "/admin/masters": true, "/line/link": true, "/explore/1": true,
		// 前方一致は区切りまで（/lineup は /line ではない）
		"/lineup": false,
	} {
		if got := defaultMeta(path).Noindex; got != want {
			t.Errorf("defaultMeta(%q).Noindex = %v, want %v", path, got, want)
		}
	}
}

func TestSPARoutesMatchShared(t *testing.T) {
	// SPA が描けるパスの一覧は src/shared/routes.ts が正（App.tsx との対応は apps/web/src/App.routes.test.ts）。
	// Go の写しがずれると、新しい画面が 404 で返る。
	src, err := os.ReadFile("../../src/shared/routes.ts")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)export const SPA_ROUTES = \[(.*?)\] as const`).FindSubmatch(src)
	if block == nil {
		t.Fatal("routes.ts に SPA_ROUTES が見つからない")
	}
	var shared []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(block[1], -1) {
		shared = append(shared, string(m[1]))
	}
	if strings.Join(shared, " ") != strings.Join(spaRoutes, " ") {
		t.Fatalf("spaRoutes = %v\nroutes.ts = %v", spaRoutes, shared)
	}
}
