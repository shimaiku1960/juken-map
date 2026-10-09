package textbooks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

func TestTextbookMasterJSON(t *testing.T) {
	// 手元のマスターはどれも総量の候補と出版社・版を持つので、その逆の場合は応答一致テストで確かめられない。
	// 画面が前提にしている形（候補が無くても "metrics": []、出版社・版が無ければ null）をここで確かめる。
	raw, err := json.Marshal(apischema.TextbookMaster{Metrics: []apischema.TextbookMasterMetric{}})
	if err != nil {
		t.Fatal(err)
	}
	got := httpxtest.DecodeJSON(t, string(raw))
	if list, ok := got["metrics"].([]any); !ok || len(list) != 0 {
		t.Errorf("metrics = %v, want []", got["metrics"])
	}
	for _, key := range []string{"publisher", "edition"} {
		if v, has := got[key]; !has || v != nil {
			t.Errorf("%s = %v（キーあり %v）, want null", key, v, has)
		}
	}
}

func TestTextbookMastersFromCache(t *testing.T) {
	st := newStore(nil)
	loads := 0
	st.masters = httpx.NewJSONSnapshotCache(time.Minute, func(context.Context) (any, error) {
		loads++
		return []apischema.TextbookMaster{}, nil
	})
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	rt.User("GET /api/textbook-masters", (&Handlers{store: st}).ListMasters)

	get := func(etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", "/api/textbook-masters", nil)
		req.AddCookie(&http.Cookie{Name: "test", Value: "alice"})
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		res := httptest.NewRecorder()
		rt.ServeHTTP(res, req)
		return res
	}
	first := get("")
	etag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || etag == "" {
		t.Fatalf("status = %d, ETag = %q", first.Code, etag)
	}
	if res := get(etag); res.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", res.Code)
	}
	if loads != 1 {
		t.Errorf("loads = %d, want 1（2回目はキャッシュから返す）", loads)
	}
}
