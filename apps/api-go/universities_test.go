package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMatchesETag(t *testing.T) {
	// Node の routes/universities.test.ts と同じ場面
	const etag = `"abc"`
	tests := map[string]bool{
		`"abc"`:            true,
		`"other", W/"abc"`: true, // 途中で圧縮し直されると弱い形で戻ってくる
		`"other"`:          false,
		``:                 false,
	}
	for in, want := range tests {
		if got := matchesETag(in, etag); got != want {
			t.Errorf("matchesETag(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAcceptsGzip(t *testing.T) {
	// Node の pickEncoding のテストから br を除いたもの。Go は br を持たないので、br だけのときは圧縮しない。
	tests := map[string]bool{
		"gzip, deflate, br, zstd": true,
		"gzip;q=1.0, br;q=0.5":    true,
		"br;q=0, gzip":            true,
		"GZIP":                    true,
		"*":                       true,
		"gzip;q=0":                false,
		"gzip;q=0.000":            false,
		"gzip;q=":                 false, // Node の Number("") は 0
		"br":                      false,
		"deflate":                 false,
		"identity":                false,
		"":                        false,
	}
	for in, want := range tests {
		if got := acceptsGzip(in); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestMarshalLikeJS(t *testing.T) {
	// JSON.stringify と同じバイト列にする（ETag を Node と揃えるため）
	got, err := marshalLikeJS([]exploreUniversityDTO{{ID: 1, Name: "A&B<大学>", Faculties: []exploreFacultyDTO{}}})
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"id":1,"name":"A&B<大学>","prefecture":"","type":"","faculties":[]}]`
	if string(got) != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestExploreCache(t *testing.T) {
	// 期限内は DB を引かない（db は nil のままなので、引けば panic する）
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	st := &universityStore{now: func() time.Time { return now }}
	cached := &exploreSnapshot{json: []byte(`[]`), etag: `"x"`, expiresAt: now.Add(time.Second)}
	st.snapshot.Store(cached)

	got, err := st.explore(context.Background())
	if err != nil || got != cached {
		t.Fatalf("explore() = %v, %v; want the cached snapshot", got, err)
	}
}

func TestUniversityListConditional(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	st := &universityStore{now: func() time.Time { return now }}
	st.snapshot.Store(&exploreSnapshot{
		json: []byte(`[]`), gzip: []byte("gz"), etag: `"x"`, expiresAt: now.Add(time.Minute),
	})
	rt := newRouter(fakeSessions(testSessions))
	rt.user("GET /api/universities", (&universityHandlers{store: st}).list)

	tests := []struct {
		name, ifNoneMatch, acceptEncoding string
		wantStatus                        int
		wantEncoding, wantBody            string
	}{
		{"ETag が合えば 304 で本文なし", `W/"x"`, "gzip", 304, "", ""},
		{"gzip を受け付けるなら圧縮済みを返す", "", "gzip, br", 200, "gzip", "gz"},
		{"受け付けなければそのまま", `"old"`, "br", 200, "", "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/universities", nil)
			req.AddCookie(&http.Cookie{Name: "test", Value: "alice"})
			req.Header.Set("If-None-Match", tt.ifNoneMatch)
			req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			res := httptest.NewRecorder()
			rt.ServeHTTP(res, req)

			if res.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", res.Code, tt.wantStatus)
			}
			h := res.Header()
			if h.Get("ETag") != `"x"` || h.Get("Cache-Control") != "private, no-cache" || h.Get("Vary") != "Accept-Encoding" {
				t.Errorf("ヘッダー = %v", h)
			}
			if h.Get("Content-Encoding") != tt.wantEncoding || res.Body.String() != tt.wantBody {
				t.Errorf("Content-Encoding = %q, 本文 = %q", h.Get("Content-Encoding"), res.Body.String())
			}
		})
	}
}
