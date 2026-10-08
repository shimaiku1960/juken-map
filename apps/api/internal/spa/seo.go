package spa

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/site"
)

// SPA は誰が来ても同じ index.html を返すため、クローラーと SNS が読む head をサーバー側で作り直す（JUK-111）。
// Node の apps/api/src/seo.ts から移した。差し込む中身と順番は Node と同じにしてある（spa_test.go で確かめる）。

// siteName は画面の名前。画面の src/shared/pageMeta.ts の SITE_NAME と同じ値。
const siteName = "受験マップ"

// ogImage は記事以外の OGP 画像。画面の src/shared/pageMeta.ts の OG_IMAGE と同じ値。
const ogImage = site.URL + "/opengraph-image.png"

const defaultDescription = "学習の開始から時間記録、予定と実績の確認、科目別の振り返りまでをひとつにつなぐ、大学受験生向け学習管理アプリです。"

// pageMeta はページの head の中身。記事の分はビルド（apps/web/scripts/prerender.mjs）が dist/ssg/meta.json に
// 書き出すので、JSON の名前は src/shared/pageMeta.ts の PageMeta と同じにする。
type pageMeta struct {
	Title         string `json:"title"`
	Description   string `json:"description"`
	Canonical     string `json:"canonical,omitempty"`
	OGTitle       string `json:"ogTitle"`
	OGType        string `json:"ogType"`
	OGImage       string `json:"ogImage"`
	Noindex       bool   `json:"noindex"`
	PublishedTime string `json:"publishedTime,omitempty"`
	ModifiedTime  string `json:"modifiedTime,omitempty"`
}

// noindexPrefixes は検索結果に出さないページ。robots.txt では止めない（クロールさせて noindex を読ませる）。
var noindexPrefixes = []string{
	"/login",
	"/signup",
	"/forgot-password",
	"/reset-password",
	"/verify-email",
	"/dashboard",
	"/schedule",
	"/goals",
	"/profile",
	"/explore",
	"/line",
	"/admin",
}

var staticMeta = map[string]struct{ title, description string }{
	"/": {
		title:       "今日の勉強を、合格までの積み重ねに。｜" + siteName,
		description: defaultDescription,
	},
	"/terms": {
		title:       "利用規約｜" + siteName,
		description: "受験マップをご利用いただく際の条件を定めています。",
	},
	"/privacy": {
		title:       "プライバシーポリシー｜" + siteName,
		description: "受験マップにおける利用者情報の取り扱いについて説明します。",
	},
}

var attributeEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
	`"`, "&quot;",
	"'", "&#039;",
)

// escapeAttribute は値を HTML の属性に安全に埋め込める形にする（& < > " ' の5文字）。
func escapeAttribute(v string) string {
	return attributeEscaper.Replace(v)
}

// defaultMeta は記事以外のページの head の中身。
func defaultMeta(pathname string) pageMeta {
	title := siteName
	description := defaultDescription
	canonical := ""
	if preset, ok := staticMeta[pathname]; ok {
		title = preset.title
		description = preset.description
		canonical = site.URL
		if pathname != "/" {
			canonical += pathname
		}
	}
	noindex := false
	for _, prefix := range noindexPrefixes {
		if pathname == prefix || strings.HasPrefix(pathname, prefix+"/") {
			noindex = true
			break
		}
	}
	return pageMeta{
		Title:       title,
		Description: description,
		Canonical:   canonical,
		OGTitle:     title,
		OGType:      "website",
		OGImage:     ogImage,
		Noindex:     noindex,
	}
}

var articlePath = regexp.MustCompile(`^/articles/([^/]+)$`)

// isArticlePath は記事のページ（/articles/:id）か。
func isArticlePath(pathname string) bool {
	return articlePath.MatchString(pathname)
}

// Scripts は、どのページにも差し込む実行時の設定（環境変数）。画面のバンドルに焼き込むと
// 環境ごとに再ビルドが要るので、サーバーが差し込む。
type Scripts struct {
	GAMeasurementID  string // GA4。空なら計測のタグを出さない
	FaroCollectorURL string // 画面のエラーの送り先（Grafana Faro）。空なら画面は送らない
}

// analyticsInlineScript は GA4 のインラインスクリプト。CSP はこの中身のハッシュ（inlineScriptHashes）
// だけを許す。画面遷移の計測は GA4 の拡張計測（履歴の変化を自動で拾う）に任せる。
//
// URL にトークンが載る画面（tokenLinkPages）では、最初の page_view を送らない。page_view は
// ページの URL（dl）ごと Google へ送るが、画面がトークンを URL から消す（useTokenFromLink.ts）のと
// gtag.js が URL を読むのは早い者勝ちになるため（JUK-124）。消したあとの URL は拡張計測が拾う。
func analyticsInlineScript(id string) string {
	return `
      window.dataLayer = window.dataLayer || [];
      function gtag(){dataLayer.push(arguments);}
      window.gtag = gtag;
      gtag('js', new Date());
      gtag('config', '` + id + `', /[?&](token|linkToken)=/.test(location.search) ? { send_page_view: false } : {});
    `
}

func (p Scripts) analyticsTag() string {
	if p.GAMeasurementID == "" {
		return ""
	}
	id := escapeAttribute(p.GAMeasurementID)
	return `<script async src="https://www.googletagmanager.com/gtag/js?id=` + id + `"></script>
    <script>` + analyticsInlineScript(id) + `</script>`
}

// inlineScriptHashes は CSP でインラインスクリプトを許すための sha256。'unsafe-inline' で全部を許す代わりに、
// GA4 の1本だけを中身のハッシュで許す。
func (p Scripts) inlineScriptHashes() []string {
	if p.GAMeasurementID == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(analyticsInlineScript(escapeAttribute(p.GAMeasurementID))))
	return []string{"'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"}
}

// faroTag は Faro の送り先を画面へ渡す meta。画面側（apps/web/src/lib/faro.ts）がこれを読んで送信を始める。
func (p Scripts) faroTag() string {
	if p.FaroCollectorURL == "" {
		return ""
	}
	return `<meta name="faro-collector-url" content="` + escapeAttribute(p.FaroCollectorURL) + `"/>`
}

var titleTag = regexp.MustCompile(`<title>[^<]*</title>`)

// injectMeta は HTML の <title> を差し替え、</head> の直前に meta を差し込む。
func (p Scripts) injectMeta(html string, meta pageMeta) string {
	optional := func(ok bool, tag string) string {
		if ok {
			return tag
		}
		return ""
	}
	candidates := []string{
		`<meta name="description" content="` + escapeAttribute(meta.Description) + `"/>`,
		optional(meta.Canonical != "", `<link rel="canonical" href="`+escapeAttribute(meta.Canonical)+`"/>`),
		optional(meta.Noindex, `<meta name="robots" content="noindex, nofollow"/>`),
		`<meta property="og:site_name" content="` + siteName + `"/>`,
		`<meta property="og:locale" content="ja_JP"/>`,
		`<meta property="og:title" content="` + escapeAttribute(meta.OGTitle) + `"/>`,
		`<meta property="og:description" content="` + escapeAttribute(meta.Description) + `"/>`,
		`<meta property="og:type" content="` + meta.OGType + `"/>`,
		`<meta property="og:image" content="` + escapeAttribute(meta.OGImage) + `"/>`,
		optional(meta.Canonical != "", `<meta property="og:url" content="`+escapeAttribute(meta.Canonical)+`"/>`),
		optional(meta.PublishedTime != "", `<meta property="article:published_time" content="`+escapeAttribute(meta.PublishedTime)+`"/>`),
		optional(meta.ModifiedTime != "", `<meta property="article:modified_time" content="`+escapeAttribute(meta.ModifiedTime)+`"/>`),
		`<meta name="twitter:card" content="summary_large_image"/>`,
		`<meta name="twitter:title" content="` + escapeAttribute(meta.OGTitle) + `"/>`,
		`<meta name="twitter:description" content="` + escapeAttribute(meta.Description) + `"/>`,
		`<meta name="twitter:image" content="` + escapeAttribute(meta.OGImage) + `"/>`,
		p.analyticsTag(),
		p.faroTag(),
	}
	tags := make([]string, 0, len(candidates))
	for _, tag := range candidates {
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	// 最初の1か所だけを置き換える。
	if loc := titleTag.FindStringIndex(html); loc != nil {
		html = html[:loc[0]] + "<title>" + escapeAttribute(meta.Title) + "</title>" + html[loc[1]:]
	}
	return strings.Replace(html, "</head>", "  "+strings.Join(tags, "\n    ")+"\n  </head>", 1)
}

// pageCSP は画面（HTML）に付ける Content-Security-Policy。API の応答は internal/app の securityHeaders が
// もっと狭い default-src 'none' を付けるので、HTML を返すときだけこれで上書きする。
//
// 2026-09-19 から2日ほど Report-Only で流し、本番の主要画面を実ブラウザでひと通り踏んでも違反が0件だったため、
// 2026-09-21 に止めるモードへ切り替えた。許可の漏れがあると画面が壊れるので、
// 一覧を増やすときは先に Report-Only で確かめること。違反は internal/feature/cspreport が受けて Loki に `csp violation` で残る。
func (p Scripts) pageCSP() string {
	// GA4 の計測の送り先。gtag.js は地域ごとのサブドメインへ送る。
	googleAnalytics := []string{
		"https://www.googletagmanager.com",
		"https://*.google-analytics.com",
		"https://*.analytics.google.com",
	}
	connect := []string{"'self'"}
	if origin := urlOrigin(p.FaroCollectorURL); origin != "" {
		connect = append(connect, origin)
	}
	connect = append(connect, googleAnalytics...)

	directives := []struct {
		name   string
		values []string
	}{
		{"default-src", []string{"'self'"}},
		{"script-src", append([]string{"'self'", "https://www.googletagmanager.com"}, p.inlineScriptHashes()...)},
		// Sonner（トースト）が JS で <style> を差し込むため、スタイルだけはインラインを許す。
		// CSS の差し込みでできることはスクリプトよりずっと限られるので、よくある妥協。
		{"style-src", []string{"'self'", "'unsafe-inline'", "https://fonts.googleapis.com"}},
		{"font-src", []string{"'self'", "https://fonts.gstatic.com"}},
		// microCMS の記事画像、GA4 の計測ピクセル。
		{"img-src", append([]string{"'self'", "data:", "blob:", "https://images.microcms-assets.io"}, googleAnalytics...)},
		{"connect-src", connect},
		{"object-src", []string{"'none'"}},
		{"base-uri", []string{"'self'"}},
		{"form-action", []string{"'self'"}},
		// 他のサイトの <iframe> に入れさせない（クリックジャッキング対策。X-Frame-Options の後継）。
		{"frame-ancestors", []string{"'none'"}},
		// 報告は report-uri だけにする。後継の report-to は Chrome が報告をまとめて後から送るため、
		// 手元の確認では数分待っても1件も届かなかった。
		{"report-uri", []string{cspReportPath}},
	}
	parts := make([]string, len(directives))
	for i, d := range directives {
		parts[i] = d.name + " " + strings.Join(d.values, " ")
	}
	return strings.Join(parts, "; ")
}

// cspReportPath は CSP の違反の報告を受ける先（internal/feature/cspreport）。
const cspReportPath = "/api/csp-report"

// urlOrigin は URL のオリジン（scheme://host[:port]）。読めなければ空。
func urlOrigin(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// sitemapArticle は sitemap に載せる記事。ビルドで SSG した記事の meta（dist/ssg/meta.json）から作る。
type sitemapArticle struct {
	pathname     string
	lastModified string
}

// buildSitemap は sitemap.xml を作る（固定ページ＋SSG した記事）。microCMS には問い合わせないので、表示のたびに待たない。
func buildSitemap(articles []sitemapArticle) []byte {
	type entry struct {
		url, lastModified, changeFrequency string
		priority                           float64
	}
	entries := []entry{
		{url: site.URL, changeFrequency: "weekly", priority: 1},
		{url: site.URL + "/blog", changeFrequency: "weekly", priority: 0.6},
		{url: site.URL + "/terms", changeFrequency: "yearly", priority: 0.3},
		{url: site.URL + "/privacy", changeFrequency: "yearly", priority: 0.3},
	}
	for _, a := range articles {
		entries = append(entries, entry{
			url:             site.URL + a.pathname,
			lastModified:    sitemapLastModified(a.lastModified),
			changeFrequency: "monthly",
			priority:        0.5,
		})
	}

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  <url>\n    <loc>" + escapeAttribute(e.url) + "</loc>")
		if e.lastModified != "" {
			b.WriteString("\n    <lastmod>" + e.lastModified + "</lastmod>")
		}
		b.WriteString("\n    <changefreq>" + e.changeFrequency + "</changefreq>")
		b.WriteString("\n    <priority>" + strconv.FormatFloat(e.priority, 'f', -1, 64) + "</priority>\n  </url>")
	}
	b.WriteString("\n</urlset>")
	return []byte(b.String())
}

// sitemapLastModified は記事の更新日時を JavaScript の toISOString() と同じ形（UTC・ミリ秒3桁・Z）にする。
// 読めなければ空（lastmod を出さない。読めない日付1件で起動に失敗しないため）。
func sitemapLastModified(v string) string {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return ""
	}
	return dates.ISOMillis(t)
}

// spaRoutes は SPA が描けるパスの一覧。src/shared/routes.ts の SPA_ROUTES と同じ並び
// （spa_test.go の TestSPARoutesMatchShared が突き合わせる。App.tsx との対応は apps/web/src/App.routes.test.ts）。
var spaRoutes = []string{
	// 公開
	"/",
	"/blog",
	"/articles/:id",
	"/terms",
	"/privacy",
	"/lp",

	// 認証フロー
	"/login",
	"/signup",
	"/forgot-password",
	"/reset-password",
	"/verify-email",
	"/verify-email/confirm",

	// ログイン必須
	"/dashboard",
	"/goals",
	"/explore",
	"/explore/:universityId",
	"/profile",
	"/schedule",
	"/admin",
	"/admin/masters",

	// LINE のトークから開く
	"/line/link",
}

// ":id" のような部分は「/ を含まない1区切り」として扱う。ルートの文字列に正規表現の特殊文字は入らない。
var spaRouteMatchers = func() []*regexp.Regexp {
	param := regexp.MustCompile(`:[^/]+`)
	matchers := make([]*regexp.Regexp, len(spaRoutes))
	for i, route := range spaRoutes {
		matchers[i] = regexp.MustCompile("^" + param.ReplaceAllString(route, "[^/]+") + "$")
	}
	return matchers
}()

var trailingSlashes = regexp.MustCompile(`/+$`)

// isKnownSPARoute は、そのパスを SPA が描けるか（＝ index.html を 200 で返してよいか）。
func isKnownSPARoute(pathname string) bool {
	// react-router は "/login/" を "/login" と同じ画面として扱うので、判定も揃える。
	normalized := pathname
	if len(pathname) > 1 {
		normalized = trailingSlashes.ReplaceAllString(pathname, "")
		if normalized == "" {
			normalized = "/"
		}
	}
	for _, m := range spaRouteMatchers {
		if m.MatchString(normalized) {
			return true
		}
	}
	return false
}
