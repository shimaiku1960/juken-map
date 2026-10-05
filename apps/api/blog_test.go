package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ブログの中継のテスト。microCMS は httptest の偽物にする（本物の API キーは要らない）。

func newBlogTestRouter(t *testing.T, upstream http.HandlerFunc) *router {
	t.Helper()
	srv := httptest.NewServer(upstream)
	t.Cleanup(srv.Close)
	rt := newRouter(fakeSessions(nil))
	registerBlogRoutes(rt, blogConfig{apiBase: srv.URL + "/api/v1", apiKey: "key"})
	return rt
}

func getBlog(rt *router, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestBlogDetail(t *testing.T) {
	var gotPath, gotKey string
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotKey = r.URL.Path, r.Header.Get("X-MICROCMS-API-KEY")
		switch r.URL.Path {
		case "/api/v1/blogs/abc":
			_, _ = w.Write([]byte(`{"id":"abc","title":"記事"}`))
		case "/api/v1/blogs/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})

	// 記事が取れたら中身を変えずに返す。キーはサーバーから microCMS へだけ送る
	rec := getBlog(rt, "/api/blog/abc")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"id":"abc","title":"記事"}` {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if gotPath != "/api/v1/blogs/abc" || gotKey != "key" {
		t.Fatalf("microCMS へ path = %q, key = %q", gotPath, gotKey)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}

	// 記事が存在しないときだけ 404
	rec = getBlog(rt, "/api/blog/missing")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing: status = %d", rec.Code)
	}
	assertJSONEqual(t, rec.Body.String(), `{"error":"Not found"}`)

	// microCMS の障害は 404 にせず 502（記事が無いことにしない）
	rec = getBlog(rt, "/api/blog/broken")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("broken: status = %d", rec.Code)
	}
	assertJSONEqual(t, rec.Body.String(), `{"error":"Bad Gateway"}`)
}

func TestBlogDetailEscapesID(t *testing.T) {
	var gotRawPath string
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		gotRawPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{}`))
	})
	// ID に ? や # を混ぜても、microCMS の別の URL（クエリ）にならない
	getBlog(rt, "/api/blog/a%3Fdraft=1")
	if gotRawPath != "/api/v1/blogs/a%3Fdraft=1" {
		t.Fatalf("microCMS へ path = %q", gotRawPath)
	}
}

func TestBlogList(t *testing.T) {
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/blogs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"contents":[],"totalCount":0}`))
	})
	rec := getBlog(rt, "/api/blog")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"contents":[],"totalCount":0}` {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
}

func TestBlogUpstreamFailures(t *testing.T) {
	// 一覧で microCMS が落ちていたら 500 ではなく 502
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	if rec := getBlog(rt, "/api/blog"); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}

	// 一覧の 404 は設定の誤りなので、記事が無いことにせず 502
	rt = newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if rec := getBlog(rt, "/api/blog"); rec.Code != http.StatusBadGateway {
		t.Fatalf("list 404: status = %d, want 502", rec.Code)
	}

	// 設定が無ければ microCMS を呼ばずに 502
	rt = newRouter(fakeSessions(nil))
	registerBlogRoutes(rt, blogConfig{})
	if rec := getBlog(rt, "/api/blog"); rec.Code != http.StatusBadGateway {
		t.Fatalf("未設定: status = %d, want 502", rec.Code)
	}
}

func TestBlogTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("3秒待つ")
	}
	// 応答が遅ければ上限（3秒）で打ち切って 502。404 にはしない
	release := make(chan struct{})
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)
	start := time.Now()
	rec := getBlog(rt, "/api/blog/abc")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > microcmsTimeout+time.Second {
		t.Fatalf("待った時間 = %s", elapsed)
	}
}

func TestBlogTooLarge(t *testing.T) {
	// 上限を超えた応答は、途中で切った JSON を返さずに 502
	rt := newBlogTestRouter(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", maxBlogBody+1)))
	})
	if rec := getBlog(rt, "/api/blog"); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
}
