package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
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

// countingCache は、load を呼んだ回数を数えるキャッシュ。時計は now で止めておく。
func countingCache(now *time.Time, load func(n int) (any, error)) (*jsonSnapshotCache, *int) {
	calls := 0
	c := newJSONSnapshotCache(time.Minute, func(context.Context) (any, error) {
		calls++
		return load(calls)
	})
	c.now = func() time.Time { return *now }
	return c, &calls
}

func TestJSONSnapshotCache(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	c, calls := countingCache(&now, func(n int) (any, error) { return []int{n}, nil })
	get := func() string {
		t.Helper()
		s, err := c.get(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return string(s.json)
	}

	if got := get(); got != "[1]" {
		t.Fatalf("1回目 = %s", got)
	}
	if got := get(); got != "[1]" || *calls != 1 {
		t.Errorf("期限内なのに読み直した: %s, load %d 回", got, *calls)
	}
	c.invalidate()
	if got := get(); got != "[2]" {
		t.Errorf("invalidate の後も古い中身: %s", got)
	}
	now = now.Add(time.Minute)
	if got := get(); got != "[3]" {
		t.Errorf("期限が切れても古い中身: %s", got)
	}
}

// 読み込みの途中で invalidate されたら、その結果は返すが置かない（編集の前の DB から作ったかもしれない）。
func TestJSONSnapshotCacheInvalidatedWhileLoading(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	var c *jsonSnapshotCache
	c, calls := countingCache(&now, func(n int) (any, error) {
		if n == 1 {
			c.invalidate() // 読んでいる間に管理画面で編集された
		}
		return []int{n}, nil
	})

	if s, err := c.get(context.Background()); err != nil || string(s.json) != "[1]" {
		t.Fatalf("get() = %v, %v", s, err)
	}
	if c.snapshot.Load() != nil {
		t.Error("途中で捨てられた読み込みの結果が置かれた")
	}
	if s, _ := c.get(context.Background()); string(s.json) != "[2]" || *calls != 2 {
		t.Errorf("次の get で読み直していない: %s", s.json)
	}
}

func TestJSONSnapshotCacheLoadError(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	boom := errors.New("boom")
	c, _ := countingCache(&now, func(int) (any, error) { return nil, boom })
	if _, err := c.get(context.Background()); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
	if c.snapshot.Load() != nil {
		t.Error("失敗したのにキャッシュがある")
	}
}

func TestWriteJSONSnapshot(t *testing.T) {
	snap := &jsonSnapshot{json: []byte(`[]`), gzip: []byte("gz"), etag: `"x"`}
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
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("If-None-Match", tt.ifNoneMatch)
			req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			res := httptest.NewRecorder()
			writeJSONSnapshot(res, req, snap)

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

// GET /api/textbook-masters もキャッシュから返す（db は nil なので、DB を引けば panic する）。
func TestTextbookMastersFromCache(t *testing.T) {
	st := newTextbookStore(nil)
	snap, err := newJSONSnapshot([]apischema.TextbookMaster{}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	st.masters.snapshot.Store(snap)
	rt := newRouter(fakeSessions(testSessions))
	rt.user("GET /api/textbook-masters", (&textbookHandlers{store: st}).listMasters)

	req := httptest.NewRequest("GET", "/api/textbook-masters", nil)
	req.AddCookie(&http.Cookie{Name: "test", Value: "alice"})
	req.Header.Set("If-None-Match", snap.etag)
	res := httptest.NewRecorder()
	rt.ServeHTTP(res, req)
	if res.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", res.Code)
	}
}
