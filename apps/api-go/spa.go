package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 画面（apps/web のビルド成果物 dist/）の配信（JUK-111。Node の apps/api/src/spa.ts から移した）。
//
// dist は起動時に全部メモリへ読み込み、リクエストのたびにファイルを開かない。2.5MB ほどなので収まり、
// パスに ../ を混ぜてほかのファイルを読ませる余地も無くなる（引くのは読み込んだ表だけ）。
// 圧縮できる種類は gzip 版も起動時に作っておく。Node は br（品質4）で圧縮していたが、Go の標準ライブラリに
// br は無いので gzip の最高圧縮にした（大きさは br の品質4とほぼ同じ）。

// tokenLinkPages はメールや LINE のリンクで開く、URL にトークンが載る画面（?token=…・?linkToken=…）。
var tokenLinkPages = map[string]bool{"/verify-email/confirm": true, "/reset-password": true, "/line/link": true}

// gzipMinSize より小さい応答は圧縮しない（Node の @fastify/compress の既定の下限と同じ）。
const gzipMinSize = 1024

// compressibleTypes は gzip 版を作る拡張子。画像・動画・フォントはもう圧縮されているので外す。
var compressibleTypes = map[string]bool{
	".html": true, ".js": true, ".css": true, ".svg": true, ".json": true,
	".txt": true, ".xml": true, ".webmanifest": true, ".map": true, ".ico": true,
}

// assetTypes は Go の mime が知らない（OS の一覧に無いことがある）種類。Node の @fastify/static（send）と同じ値にする。
var assetTypes = map[string]string{
	".webmanifest": "application/manifest+json",
}

type staticAsset struct {
	body         []byte
	gzip         []byte // 圧縮しない種類・小さいものは nil
	etag         string
	gzipETag     string
	modTime      time.Time
	contentType  string
	cacheControl string
}

// spaSite は読み込んだ dist。
type spaSite struct {
	scripts   pageScripts
	csp       string
	indexHTML string
	pages     map[string]string       // SSG した HTML（/terms → ssg/terms.html の中身）
	meta      map[string]pageMeta     // SSG したページの meta（ssg/meta.json）
	assets    map[string]*staticAsset // そのまま配るファイル（/assets/x.js など。ssg/ は含めない）
	sitemap   []byte
}

// loadSPA は root（WEB_DIST_DIR）を読み込む。root が空か index.html が無ければ (nil, nil)（画面を配らない。
// 開発では Vite が画面を配るのでここは通らない）。
func loadSPA(root string, scripts pageScripts) (*spaSite, error) {
	if root == "" {
		return nil, nil
	}
	index, err := os.ReadFile(filepath.Join(root, "index.html"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	site := &spaSite{
		scripts:   scripts,
		csp:       scripts.pageCSP(),
		indexHTML: string(index),
		pages:     map[string]string{},
		meta:      map[string]pageMeta{},
		assets:    map[string]*staticAsset{},
	}

	err = filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		// ssg/ の HTML は下の pages に入れ、meta を差し込んで返す。ファイルのまま /ssg/terms.html でも読めると、
		// meta の無い同じページが別 URL にできてしまうので、そのままは配らない。
		if strings.HasPrefix(rel, "ssg/") {
			switch {
			case rel == "ssg/meta.json":
				if err := json.Unmarshal(body, &site.meta); err != nil {
					return fmt.Errorf("%s: %w", rel, err)
				}
			case strings.HasSuffix(rel, ".html"):
				site.pages["/"+strings.TrimSuffix(strings.TrimPrefix(rel, "ssg/"), ".html")] = string(body)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		site.assets["/"+rel] = newStaticAsset(rel, body, info.ModTime())
		return nil
	})
	if err != nil {
		return nil, err
	}

	// sitemap.xml は SSG した記事から、起動時に一度だけ作る。robots.txt と OGP 画像は apps/web/public に置いた実ファイル。
	var articles []sitemapArticle
	for pathname, m := range site.meta {
		if isArticlePath(pathname) {
			articles = append(articles, sitemapArticle{pathname: pathname, lastModified: m.ModifiedTime})
		}
	}
	// Node は meta.json に書かれた順に並べていた。Go の map は順番を持たないので、パスで並べて毎回同じにする。
	sort.Slice(articles, func(i, j int) bool { return articles[i].pathname < articles[j].pathname })
	site.sitemap = buildSitemap(articles)
	return site, nil
}

func newStaticAsset(rel string, body []byte, modTime time.Time) *staticAsset {
	ext := path.Ext(rel)
	contentType := assetTypes[ext]
	if contentType == "" {
		contentType = mime.TypeByExtension(ext)
	}
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	a := &staticAsset{
		body:        body,
		etag:        contentETag(body),
		modTime:     modTime,
		contentType: contentType,
		// Node の @fastify/static の既定（public, max-age=0）に合わせ、下の2つだけ変える。
		cacheControl: "public, max-age=0",
	}
	switch {
	// index.html はデプロイのたびに中身が変わるため、必ず再検証させる。
	case rel == "index.html":
		a.cacheControl = "no-cache"
	// Vite が出す assets/* はファイル名にハッシュが入るので永久キャッシュしてよい。
	case strings.HasPrefix(rel, "assets/"):
		a.cacheControl = "public, max-age=31536000, immutable"
	}
	if compressibleTypes[ext] && len(body) >= gzipMinSize {
		a.gzip = gzipBytes(body, gzip.BestCompression)
		a.gzipETag = strings.TrimSuffix(a.etag, `"`) + `-gz"`
	}
	return a
}

func contentETag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:8]) + `"`
}

// gzipBytes は本文を gzip にする。起動時に作り置く分は最高圧縮、リクエストのたびに作る HTML は既定の強さにする。
func gzipBytes(body []byte, level int) []byte {
	var buf bytes.Buffer
	w, _ := gzip.NewWriterLevel(&buf, level)
	_, _ = w.Write(body)
	_ = w.Close()
	return buf.Bytes()
}

// registerSPA は画面の配信を登録する。"GET /" はほかのどのルートにも当たらない GET を受ける
// （ServeMux はより長いパスのルートを先に選ぶ）。GET 以外で当たらないものは、今までどおり router.go の notFound（JSON の 404）。
func registerSPA(rt *router, site *spaSite) {
	rt.public("GET /sitemap.xml", site.serveSitemap)
	rt.public("GET /", site.serve)
}

func (s *spaSite) serveSitemap(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	writeMaybeGzip(w, r, http.StatusOK, s.sitemap)
}

// serve は、API にも sitemap にも当たらなかった GET。実ファイルがあればそれを、無ければ画面の HTML を返す。
func (s *spaSite) serve(w http.ResponseWriter, r *http.Request) {
	info := requestInfoFrom(r.Context())
	pathname := r.URL.Path

	// API の 404 まで index.html を返すと、JSON を期待しているクライアントが壊れる。存在しない API は API のまま 404。
	if strings.HasPrefix(pathname, "/api/") {
		if info != nil {
			info.route = "(unmatched)"
		}
		notFound(w, r)
		return
	}
	// メトリクスの route は Node と同じく、API 以外をまとめて "(web)" にする（ファイルごとに分けると種類が増えすぎる）。
	if info != nil {
		info.route = "(web)"
	}

	if a, ok := s.assets[pathname]; ok {
		s.serveAsset(w, r, a)
		return
	}

	status := http.StatusOK
	// SPA は何を渡されても index.html を返せてしまうので、App.tsx が持たないパスは画面（NotFoundPage）と同じ 404 で返す。
	// 200 のままだと、ボットのスキャンまで「正常」に数えられてエラー率が当てにならず、検索エンジンにも soft 404 と見られる。
	// 本文は変えない（SPA が読み込まれて NotFoundPage を描く）。
	if !isKnownSPARoute(pathname) {
		status = http.StatusNotFound
	}

	h := w.Header()
	// メールや LINE のリンク（?token=…・?linkToken=…）で開く画面は、ほかのサイトへ移ったときに Referer で
	// URL を送らない（認証基準 10 の D3。画面も読み込んだらすぐ URL からトークンを消す：useTokenFromLink.ts）。
	if tokenLinkPages[pathname] {
		h.Set("Referrer-Policy", "no-referrer")
	}
	h.Set("Content-Security-Policy", s.csp)
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-cache")

	// 記事はビルドで SSG する（JUK-110）。microCMS で記事を更新したら、デプロイで作り直す。
	// SSG に無い記事（存在しない・下書き・公開してまだ作り直していない）は 404 にする。
	// 本文は SPA が /api/blog から取って描くので、公開直後の記事も画面には出る。
	page, prerendered := s.pages[pathname]
	if isArticlePath(pathname) && !prerendered {
		writeMaybeGzip(w, r, http.StatusNotFound, []byte(s.scripts.injectMeta(s.indexHTML, defaultMeta(pathname))))
		return
	}
	if !prerendered {
		page = s.indexHTML
	}
	meta, ok := s.meta[pathname]
	if !ok {
		meta = defaultMeta(pathname)
	}
	// SSG したページは本文入りの HTML を、それ以外は中身が空の index.html を返す。
	// SPA なのでクローラーと SNS は JS 実行前の HTML しか読まない。その分を head に差し込む。
	writeMaybeGzip(w, r, status, []byte(s.scripts.injectMeta(page, meta)))
}

func (s *spaSite) serveAsset(w http.ResponseWriter, r *http.Request, a *staticAsset) {
	h := w.Header()
	h.Set("Content-Type", a.contentType)
	h.Set("Cache-Control", a.cacheControl)
	if strings.HasPrefix(a.contentType, "text/html") {
		h.Set("Content-Security-Policy", s.csp)
	}
	body, etag := a.body, a.etag
	if a.gzip != nil {
		// 同じ URL でも Accept-Encoding で本文の形が変わる、と途中のキャッシュに伝える。
		h.Set("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			// Content-Encoding を付けて返すと、nginx の gzip は圧縮し直さない。
			h.Set("Content-Encoding", "gzip")
			body, etag = a.gzip, a.gzipETag
		}
	}
	h.Set("ETag", etag)
	// ServeContent が If-None-Match・If-Modified-Since（304）・Range（動画の途中から）・HEAD を扱う。
	http.ServeContent(w, r, "", a.modTime, bytes.NewReader(body))
}

// writeMaybeGzip は本文を返す。受け付けるなら gzip にする（小さいものはそのまま）。Content-Type は呼び出し側が付ける。
func writeMaybeGzip(w http.ResponseWriter, r *http.Request, status int, body []byte) {
	h := w.Header()
	if len(body) >= gzipMinSize {
		h.Add("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			h.Set("Content-Encoding", "gzip")
			body = gzipBytes(body, gzip.DefaultCompression)
		}
	}
	h.Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
