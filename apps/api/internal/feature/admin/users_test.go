package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/account"
)

// 利用者の管理（users.go）のテスト。DB は偽物にする（Go の CI には DB が無い）。
// 本物の DB に通して Node と応答を比べるテスト（admin_users_db_test.go）は、比べる相手の Node の API を
// 消したときに一緒に消した（JUK-84）。

func TestReadAdminUsersQuery(t *testing.T) {
	// 期待値は Node（Zod の listUsersQuerySchema）に同じ値を渡して得たもの。
	tests := []struct {
		raw       string
		wantKind  apischema.UserKind
		wantQ     string
		wantPage  int
		wantIssue string // "code field message"。空なら通る
	}{
		{"", apischema.UserKindReal, "", 1, ""},
		{"kind=sim&q=%20a%20&page=3", apischema.UserKindSim, " a ", 3, ""},
		{"kind=x", "", "", 0, `invalid_value kind Invalid option: expected one of "real"|"sim"|"seed"|"demo"`},
		{"kind=", "", "", 0, `invalid_value kind Invalid option: expected one of "real"|"sim"|"seed"|"demo"`},
		{"kind=real&kind=sim", "", "", 0, `invalid_value kind Invalid option: expected one of "real"|"sim"|"seed"|"demo"`},
		{"q=a&q=b", "", "", 0, "invalid_type q Invalid input: expected string, received array"},
		{"q=" + strings.Repeat("a", 192), "", "", 0, "too_big q Too big: expected string to have <=191 characters"},
		{"q=" + strings.Repeat("あ", 191), apischema.UserKindReal, strings.Repeat("あ", 191), 1, ""},
		{"page=abc", "", "", 0, "invalid_type page Invalid input: expected number, received NaN"},
		{"page=", "", "", 0, "too_small page Too small: expected number to be >0"},
		{"page=0", "", "", 0, "too_small page Too small: expected number to be >0"},
		{"page=-1", "", "", 0, "too_small page Too small: expected number to be >0"},
		{"page=1.5", "", "", 0, "invalid_type page Invalid input: expected int, received number"},
		{"page=10001", "", "", 0, "too_big page Too big: expected number to be <=10000"},
		{"page=1e3", apischema.UserKindReal, "", 1000, ""},
		{"page=%202%20", apischema.UserKindReal, "", 2, ""},
		{"page=0x10", apischema.UserKindReal, "", 16, ""},
		{"page=Infinity", "", "", 0, "invalid_type page Invalid input: expected number, received Infinity"},
		{"page=3&page", "", "", 0, "invalid_type page Invalid input: expected number, received NaN"},
		{"page=1&page=2", "", "", 0, "invalid_type page Invalid input: expected number, received NaN"},
		{"page=9007199254740993", "", "", 0, "too_big page Too big: expected int to be <=9007199254740991"},
		{"page=1_0", "", "", 0, "invalid_type page Invalid input: expected number, received NaN"},
		// kind・q・page の順に確かめ、最初の1件だけを返す
		{"page=0&kind=x", "", "", 0, `invalid_value kind Invalid option: expected one of "real"|"sim"|"seed"|"demo"`},
	}
	for _, tt := range tests {
		kind, q, page, issue := readAdminUsersQuery(httpx.ParseQuery(tt.raw))
		got := ""
		if issue != nil {
			got = issue.Code + " " + issue.Field + " " + issue.Message
		}
		if got != tt.wantIssue {
			t.Errorf("%q: issue = %q\nwant %q", tt.raw, got, tt.wantIssue)
			continue
		}
		if issue == nil && (kind != tt.wantKind || q != tt.wantQ || page != tt.wantPage) {
			t.Errorf("%q: = %q %q %d, want %q %q %d", tt.raw, kind, q, page, tt.wantKind, tt.wantQ, tt.wantPage)
		}
	}
}

func TestJSNumberFromString(t *testing.T) {
	nan := math.NaN()
	tests := map[string]float64{
		"": 0, "  ": 0, "1": 1, " 2 ": 2, "\n3\t": 3, "+4": 4, "-5": -5, "1.5": 1.5, ".5": 0.5, "5.": 5,
		"1e3": 1000, "1E-1": 0.1, "0x10": 16, "0X1f": 31, "0o17": 15, "0b101": 5, "007": 7,
		"Infinity": math.Inf(1), "-Infinity": math.Inf(-1), "1e400": math.Inf(1),
		"abc": nan, "1_0": nan, "inf": nan, "NaN": nan, "0x1p3": nan, "-0x10": nan, "0x": nan, "1,2": nan, "1 2": nan, "e3": nan,
	}
	for in, want := range tests {
		got := jsNumberFromString(in)
		if math.IsNaN(want) != math.IsNaN(got) || (!math.IsNaN(want) && got != want) {
			t.Errorf("Number(%q) = %v, want %v", in, got, want)
		}
	}
}

// fakeAdminUserStore は adminUserStore の偽物。
type fakeAdminUserStore struct {
	users   map[string]*adminTarget
	banned  []string
	deleted []string
}

func (f *fakeAdminUserStore) overview(context.Context, time.Time) (apischema.AdminOverview, error) {
	return apischema.AdminOverview{}, nil
}

func (f *fakeAdminUserStore) listUsers(_ context.Context, kind apischema.UserKind, q string, page int) (apischema.AdminUserList, error) {
	return apischema.AdminUserList{Users: []apischema.AdminUser{}, Page: page, PageSize: adminUsersPageSize}, nil
}

func (f *fakeAdminUserStore) findTarget(_ context.Context, id string) (*adminTarget, error) {
	return f.users[id], nil
}

func (f *fakeAdminUserStore) ban(_ context.Context, id string, now time.Time) (account.Suspension, error) {
	f.banned = append(f.banned, id)
	bannedAt := dates.ISOMillis(now)
	if u := f.users[id]; u != nil && u.BannedAt != nil {
		bannedAt = *u.BannedAt
	}
	return account.Suspension{BannedAt: bannedAt, SessionsRemoved: 2}, nil
}

func (f *fakeAdminUserStore) unban(context.Context, string, time.Time) error { return nil }

func (f *fakeAdminUserStore) deleteUser(_ context.Context, id string) (removedCounts, error) {
	f.deleted = append(f.deleted, id)
	return removedCounts{StudyLogs: 3}, nil
}

func strPtr(s string) *string { return &s }

func newAdminUserTestRouter() (*httpx.Router, *fakeAdminUserStore) {
	store := &fakeAdminUserStore{users: map[string]*adminTarget{
		"u1":     {ID: "u1", Email: strPtr("alice@example.com"), Role: "user"},
		"u2":     {ID: "u2", Email: strPtr("admin@example.com"), Role: "admin"}, // ログイン中の管理者自身
		"other":  {ID: "other", Email: strPtr("other-admin@example.com"), Role: "admin"},
		"demo":   {ID: "demo", Email: strPtr(httpx.DemoEmail), Role: "user"},
		"old":    {ID: "old", Email: strPtr("old@example.com"), Role: "user", BannedAt: strPtr("2026-09-01T00:00:00.000Z")},
		"noMail": {ID: "noMail", Role: "user"},
		"blank":  {ID: "blank", Email: strPtr(""), Role: "user"},
	}}
	h := &UserHandlers{store: store, now: time.Now}
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	rt.Admin("GET /api/admin/users", h.ListUsers)
	rt.Admin("POST /api/admin/users/{id}/ban", h.Ban)
	rt.Admin("POST /api/admin/users/{id}/unban", h.Unban)
	rt.Admin("DELETE /api/admin/users/{id}", h.DeleteUser)
	return rt, store
}

func adminRequest(rt *httpx.Router, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
	rec := httptest.NewRecorder()
	rt.ServeHTTP(rec, req)
	return rec
}

func TestAdminUserActions(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{"停止：いない相手は 404", "POST", "/api/admin/users/nobody/ban", "", 404, `{"error":"ユーザーが見つかりません"}`},
		{"停止：自分自身は 409", "POST", "/api/admin/users/u2/ban", "", 409, `{"error":"自分自身は停止・削除できません"}`},
		{"停止：他の管理者は 409", "POST", "/api/admin/users/other/ban", "", 409, `{"error":"他の管理者は停止・削除できません（先に権限を外してください）"}`},
		{"停止：デモは 409", "POST", "/api/admin/users/demo/ban", "", 409, `{"error":"デモアカウントは停止・削除できません"}`},
		{"停止：済みなら最初に止めた日時のまま", "POST", "/api/admin/users/old/ban", "", 200,
			`{"id":"old","email":"old@example.com","bannedAt":"2026-09-01T00:00:00.000Z","sessionsRemoved":2}`},
		{"停止：ID が長すぎれば 400", "POST", "/api/admin/users/" + strings.Repeat("a", 192) + "/ban", "", 400,
			`{"error":"Too big: expected string to have <=191 characters","code":"too_big","field":"id"}`},
		{"停止：受け付けない本文は 415", "POST", "/api/admin/users/u1/ban", "", 0, ""},

		{"解除：いない相手は 404", "POST", "/api/admin/users/nobody/unban", "", 404, `{"error":"ユーザーが見つかりません"}`},
		{"解除：守りは見ない（管理者でも解除はできる）", "POST", "/api/admin/users/other/unban", "", 200, `{"id":"other","email":"other-admin@example.com"}`},

		{"削除：本文なしは 400", "DELETE", "/api/admin/users/u1", "", 400,
			`{"error":"Invalid input: expected object, received undefined","code":"invalid_type","field":null}`},
		{"削除：メールが空は 400", "DELETE", "/api/admin/users/u1", `{"email":""}`, 400,
			`{"error":"Too small: expected string to have >=1 characters","code":"too_small","field":"email"}`},
		{"削除：メール違いは 400", "DELETE", "/api/admin/users/u1", `{"email":"bob@example.com"}`, 400, `{"error":"メールアドレスが一致しません"}`},
		// user.email は NULL を許す。空白だけを打つと空文字どうしで一致してしまうので、メールの無い相手は消さない（JUK-89）。
		{"削除：メールの無い相手は、空白だけを打っても 409", "DELETE", "/api/admin/users/noMail", `{"email":" "}`, 409, `{"error":"メールアドレスの無い利用者は、本人の確認ができないため削除できません"}`},
		{"削除：メールの無い相手は、何を打っても 409", "DELETE", "/api/admin/users/noMail", `{"email":"x@example.com"}`, 409, `{"error":"メールアドレスの無い利用者は、本人の確認ができないため削除できません"}`},
		{"削除：メールが空文字の相手も 409", "DELETE", "/api/admin/users/blank", `{"email":" "}`, 409, `{"error":"メールアドレスの無い利用者は、本人の確認ができないため削除できません"}`},
		{"削除：守られた相手は、メールが合っていても 409", "DELETE", "/api/admin/users/demo", `{"email":"` + httpx.DemoEmail + `"}`, 409, `{"error":"デモアカウントは停止・削除できません"}`},
		{"削除：大文字小文字と前後の空白は無視して比べる", "DELETE", "/api/admin/users/u1", `{"email":"  ALICE@example.com "}`, 200,
			`{"id":"u1","email":"alice@example.com","removed":{"studyLogs":3,"studyPlans":0,"textbooks":0,"finalGoals":0}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newAdminUserTestRouter()
			var rec *httptest.ResponseRecorder
			if tt.wantStatus == 0 {
				// Content-Type が text/html の本文は 415
				req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("x"))
				req.Header.Set("Content-Type", "text/html")
				req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
				rec = httptest.NewRecorder()
				rt.ServeHTTP(rec, req)
				if rec.Code != http.StatusUnsupportedMediaType {
					t.Fatalf("status = %d, want 415", rec.Code)
				}
				return
			}
			rec = adminRequest(rt, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), tt.wantBody)
		})
	}

	t.Run("断った操作は DB に届かない", func(t *testing.T) {
		rt, store := newAdminUserTestRouter()
		for _, path := range []string{"/api/admin/users/u2/ban", "/api/admin/users/other/ban", "/api/admin/users/demo/ban"} {
			adminRequest(rt, "POST", path, "")
		}
		adminRequest(rt, "DELETE", "/api/admin/users/u1", `{"email":"bob@example.com"}`)
		if len(store.banned) != 0 || len(store.deleted) != 0 {
			t.Fatalf("banned = %v, deleted = %v", store.banned, store.deleted)
		}
	})
}

// TestAdminUserActionAuditLog は、停止・解除・削除が「誰が・誰に・何をしたか」を以前と同じ形で
// 構造化ログに残すことを確かめる（セキュリティ基準 H4。Grafana の Loki で過去の記録と並べて追う）。
func TestAdminUserActionAuditLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	rt, _ := newAdminUserTestRouter()
	adminRequest(rt, "POST", "/api/admin/users/u1/ban", "")
	adminRequest(rt, "POST", "/api/admin/users/u1/unban", "")
	adminRequest(rt, "DELETE", "/api/admin/users/u1", `{"email":"alice@example.com"}`)
	adminRequest(rt, "POST", "/api/admin/users/demo/ban", "") // 断った操作は残さない

	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("JSON でない行: %q", line)
		}
		if m["msg"] == "admin user action" {
			lines = append(lines, m)
		}
	}
	if len(lines) != 3 {
		t.Fatalf("監査ログ = %v, want 3行", lines)
	}
	for i, action := range []string{"ban", "unban", "delete"} {
		m := lines[i]
		if m["level"] != "WARN" || m["adminId"] != "u2" || m["action"] != action || m["targetId"] != "u1" || m["targetEmail"] != "alice@example.com" {
			t.Errorf("%s の監査ログ = %v", action, m)
		}
	}
	if lines[0]["sessionsRemoved"] != float64(2) {
		t.Errorf("停止の監査ログに消したセッションの数が無い: %v", lines[0])
	}
	if removed, _ := lines[2]["removed"].(map[string]any); removed["studyLogs"] != float64(3) {
		t.Errorf("削除の監査ログに消えた行数が無い: %v", lines[2])
	}
}
