package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/line"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

const testMicrocmsSecret = "microcms-test-secret"

// fakeDeployer は動かした回数を数える。err を入れると失敗を返す。
type fakeDeployer struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (d *fakeDeployer) dispatch(context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return d.err
}

func (d *fakeDeployer) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func microcmsSign(body string) string {
	mac := hmac.New(sha256.New, []byte(testMicrocmsSecret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func newMicrocmsTestRouter(secret string, d deployer) *httpx.Router {
	rt := httpx.NewRouter(httpxtest.FakeSessions(nil))
	registerRoutes(rt, nil, jobConfig{}, line.Config{}, microcmsWebhookConfig{secret: secret, deployer: d})
	return rt
}

func postMicrocms(rt *httpx.Router, body, signature string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/webhooks/microcms", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if signature != "" {
		req.Header.Set("X-MICROCMS-Signature", signature)
	}
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	return rec
}

// 公開中の記事を更新した通知（microCMS の形）。
const publishedEdit = `{"service":"juken-map","api":"blogs","id":"a1","type":"edit","contents":{
	"old":{"id":"a1","status":["PUBLISH"],"draftKey":null,"publishValue":{"title":"古い"},"draftValue":null},
	"new":{"id":"a1","status":["PUBLISH"],"draftKey":null,"publishValue":{"title":"新しい"},"draftValue":null}}}`

func TestMicrocmsWebhookSignature(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		signature string
		body      string
	}{
		{"署名なし", testMicrocmsSecret, "", publishedEdit},
		{"16進数でない署名", testMicrocmsSecret, "not-a-signature", publishedEdit},
		{"別の秘密で作った署名", testMicrocmsSecret, func() string {
			mac := hmac.New(sha256.New, []byte("other-secret"))
			mac.Write([]byte(publishedEdit))
			return hex.EncodeToString(mac.Sum(nil))
		}(), publishedEdit},
		// 署名は元の本文で作り、本文を1文字だけ変えて送る
		{"本文を1文字変えたもの", testMicrocmsSecret, microcmsSign(publishedEdit), strings.Replace(publishedEdit, "a1", "a2", 1)},
		{"秘密が未設定なら正しい形の署名でも断る", "", microcmsSign(publishedEdit), publishedEdit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &fakeDeployer{}
			rec := postMicrocms(newMicrocmsTestRouter(tt.secret, d), tt.body, tt.signature)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), `{"error":"Invalid signature"}`)
			if d.count() != 0 {
				t.Errorf("署名が合わないのにデプロイを動かした（%d 回）", d.count())
			}
		})
	}
}

func TestMicrocmsWebhookTooLarge(t *testing.T) {
	d := &fakeDeployer{}
	body := `{"pad":"` + strings.Repeat("a", microcmsWebhookBodyLimit) + `"}`
	rec := postMicrocms(newMicrocmsTestRouter(testMicrocmsSecret, d), body, microcmsSign(body))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if d.count() != 0 {
		t.Errorf("デプロイを動かした（%d 回）", d.count())
	}
}

func TestMicrocmsWebhookInvalidJSON(t *testing.T) {
	d := &fakeDeployer{}
	body := `{"api":`
	rec := postMicrocms(newMicrocmsTestRouter(testMicrocmsSecret, d), body, microcmsSign(body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if d.count() != 0 {
		t.Errorf("デプロイを動かした（%d 回）", d.count())
	}
}

func TestMicrocmsWebhookDecidesDeploy(t *testing.T) {
	const (
		draft          = `{"status":["DRAFT"],"publishValue":null,"draftValue":{"title":"下書き"}}`
		published      = `{"status":["PUBLISH"],"publishValue":{"title":"公開"},"draftValue":null}`
		publishedDraft = `{"status":["PUBLISH_AND_DRAFT"],"publishValue":{"title":"公開"},"draftValue":{"title":"直し中"}}`
		closed         = `{"status":["CLOSED"],"publishValue":null,"draftValue":null}`
		// 公開を終えても publishValue が残る形で来ても、公開の状態が変われば作り直す。
		closedKeepsValue = `{"status":["CLOSED"],"publishValue":{"title":"公開"},"draftValue":null}`
	)
	payload := func(typ, before, after string) string {
		return `{"service":"juken-map","api":"blogs","id":"a1","type":"` + typ + `","contents":{"old":` + before + `,"new":` + after + `}}`
	}
	tests := []struct {
		name string
		body string
		want string // skipped か dispatched
	}{
		{"下書きで新規作成", payload("new", "null", draft), "skipped"},
		{"公開で新規作成", payload("new", "null", published), "dispatched"},
		{"下書きを公開", payload("edit", draft, published), "dispatched"},
		{"公開中の記事を更新", publishedEdit, "dispatched"},
		{"公開中の記事の下書きを保存", payload("edit", published, publishedDraft), "skipped"},
		{"直した下書きを公開", payload("edit", publishedDraft, `{"status":["PUBLISH"],"publishValue":{"title":"直した"},"draftValue":null}`), "dispatched"},
		{"公開を終了", payload("edit", published, closed), "dispatched"},
		{"公開を終了（中身が残る形）", payload("edit", published, closedKeepsValue), "dispatched"},
		{"公開中の記事を削除", payload("delete", published, "null"), "dispatched"},
		{"下書きのまま削除", payload("delete", draft, "null"), "skipped"},
		{"一括の操作（contents が null）", `{"service":"juken-map","api":"blogs","id":null,"type":"edit","contents":null}`, "dispatched"},
		{"形が分からない（status が無い）", payload("edit", `{"publishValue":{"title":"a"}}`, `{"publishValue":{"title":"a"}}`), "dispatched"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &fakeDeployer{}
			rec := postMicrocms(newMicrocmsTestRouter(testMicrocmsSecret, d), tt.body, microcmsSign(tt.body))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（%s）", rec.Code, rec.Body.String())
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), `{"deploy":"`+tt.want+`"}`)
			wantCalls := 0
			if tt.want == "dispatched" {
				wantCalls = 1
			}
			if d.count() != wantCalls {
				t.Errorf("デプロイを %d 回動かした、want %d", d.count(), wantCalls)
			}
		})
	}
}

func TestMicrocmsWebhookDispatchFailed(t *testing.T) {
	d := &fakeDeployer{err: errors.New("GitHub API 401")}
	rt := newMicrocmsTestRouter(testMicrocmsSecret, d)
	rec := postMicrocms(rt, publishedEdit, microcmsSign(publishedEdit))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	httpxtest.AssertJSONEqual(t, rec.Body.String(), `{"error":"Failed to dispatch deploy"}`)

	// 失敗したら間引かず、次の通知ですぐ動かし直す。
	d.mu.Lock()
	d.err = nil
	d.mu.Unlock()
	rec = postMicrocms(rt, publishedEdit, microcmsSign(publishedEdit))
	httpxtest.AssertJSONEqual(t, rec.Body.String(), `{"deploy":"dispatched"}`)
	if d.count() != 2 {
		t.Errorf("デプロイを %d 回動かした、want 2", d.count())
	}
}

func TestDeployTriggerCoalesces(t *testing.T) {
	d := &fakeDeployer{}
	now := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	var scheduled []func()
	var waits []time.Duration
	tr := newDeployTrigger(d)
	tr.now = func() time.Time { return now }
	tr.afterFunc = func(wait time.Duration, f func()) {
		waits = append(waits, wait)
		scheduled = append(scheduled, f)
	}
	request := func(want string) {
		t.Helper()
		got, err := tr.request(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("request = %q, want %q", got, want)
		}
	}

	request("dispatched")
	now = now.Add(10 * time.Second)
	request("queued")
	now = now.Add(10 * time.Second)
	request("queued")
	if d.count() != 1 || len(scheduled) != 1 {
		t.Fatalf("間の中で動かした: calls=%d scheduled=%d", d.count(), len(scheduled))
	}
	// 予約は、最初に動かしてから1分のところ。
	if waits[0] != 50*time.Second {
		t.Errorf("wait = %v, want 50s", waits[0])
	}

	// 間が明けて予約が動く。
	now = now.Add(40 * time.Second)
	scheduled[0]()
	if d.count() != 2 {
		t.Fatalf("予約が動かなかった: calls=%d", d.count())
	}
	// 予約で動かした時刻から、また1分は間引く。
	now = now.Add(30 * time.Second)
	request("queued")
	if len(scheduled) != 2 || waits[1] != 30*time.Second {
		t.Fatalf("scheduled=%d waits=%v", len(scheduled), waits)
	}
	scheduled[1]()

	// 1分以上空けば、すぐ動かす。
	now = now.Add(2 * time.Minute)
	request("dispatched")
	if d.count() != 4 {
		t.Errorf("calls = %d, want 4", d.count())
	}
}

func TestGithubWorkflowDispatcher(t *testing.T) {
	var gotPath, gotAuth, gotAccept, gotVersion, gotBody string
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.Method + " " + r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotVersion = r.Header.Get("X-GitHub-Api-Version")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(status)
		if status != http.StatusNoContent {
			_, _ = w.Write([]byte(`{"message":"Bad credentials"}`))
		}
	}))
	defer srv.Close()

	g := &githubWorkflowDispatcher{
		client: srv.Client(), apiBase: srv.URL, repo: "shimaiku1960/juken-map",
		workflow: "deploy.yml", ref: "main", token: "github-token",
	}
	if err := g.dispatch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "POST /repos/shimaiku1960/juken-map/actions/workflows/deploy.yml/dispatches" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer github-token" || gotAccept != "application/vnd.github+json" || gotVersion != "2022-11-28" {
		t.Errorf("headers: auth=%q accept=%q version=%q", gotAuth, gotAccept, gotVersion)
	}
	httpxtest.AssertJSONEqual(t, gotBody, `{"ref":"main"}`)

	t.Run("GitHub が断ったら失敗を返す", func(t *testing.T) {
		status = http.StatusUnauthorized
		err := g.dispatch(context.Background())
		if err == nil || !strings.Contains(err.Error(), "GitHub API 401") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("トークンが未設定なら呼ばない", func(t *testing.T) {
		gotPath = ""
		empty := *g
		empty.token = ""
		if err := empty.dispatch(context.Background()); err == nil {
			t.Fatal("トークンが空なのに失敗しなかった")
		}
		if gotPath != "" {
			t.Errorf("GitHub を呼んだ: %q", gotPath)
		}
	})
}
