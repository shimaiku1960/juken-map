package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// 入力の決まりそのものはモデル（internal/model/university の input_test.go）で、
// DB で断った結果の status・文言と、変更の記録は app の TestAdminMastersDB で確かめる。
// ここでは、DB に触る前に断るもの（path の id の形と、path と本文を確かめる順番）だけを見る。
// DB は nil で渡すので、DB まで届けば落ちる。

func newAdminMasterTestRouter() *httpx.Router {
	h := &MasterHandlers{}
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	rt.Admin("GET /api/admin/universities", h.ListUniversities)
	rt.Admin("POST /api/admin/universities", h.CreateUniversity)
	rt.Admin("GET /api/admin/universities/{id}", h.UniversityDetail)
	rt.Admin("PATCH /api/admin/universities/{id}", h.UpdateUniversity)
	rt.Admin("DELETE /api/admin/universities/{id}", h.DeleteUniversity)
	rt.Admin("POST /api/admin/faculties", h.CreateFaculty)
	rt.Admin("PATCH /api/admin/faculties/{id}", h.UpdateFaculty)
	rt.Admin("DELETE /api/admin/faculties/{id}", h.DeleteFaculty)
	return rt
}

// validationBody は 400 の ValidationError の本文。
func validationBody(code, field, message string) string {
	f := "null"
	if field != "" {
		f = `"` + field + `"`
	}
	return `{"error":"` + message + `","code":"` + code + `","field":` + f + `}`
}

func TestAdminMasterRejectsBeforeDB(t *testing.T) {
	const (
		badID   = "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/"
		faculty = `{"name":"学部","examDate":"2027-02-01","tagIds":[]}`
	)
	tests := []struct {
		name, method, path, body string
		want                     string // 400 の本文
	}{
		{"大学の作成：本文なし", "POST", "/api/admin/universities", "", validationBody("invalid_type", "", "Invalid input: expected object, received undefined")},
		{"大学の書き換え：ID の形が違えば本文より先に 400", "PATCH", "/api/admin/universities/0", `{}`, validationBody("invalid_format", "id", badID)},
		{"大学の書き換え：本文も確かめる", "PATCH", "/api/admin/universities/1", `{"name":"大学","prefecture":"東京都","type":"x"}`, validationBody("invalid_value", "type", "種別を選んでください")},
		{"大学の詳細：ID の形", "GET", "/api/admin/universities/01", "", validationBody("invalid_format", "id", badID)},
		{"大学の削除：ID の形", "DELETE", "/api/admin/universities/abc", "", validationBody("invalid_format", "id", badID)},
		{"大学の一覧：q が2つ", "GET", "/api/admin/universities?q=a&q=b&page=x", "", validationBody("invalid_type", "q", "Invalid input: expected string, received array")},
		{"大学の一覧：page が0", "GET", "/api/admin/universities?page=0", "", validationBody("too_small", "page", "Too small: expected number to be >0")},
		{"学部の作成：大学なし", "POST", "/api/admin/faculties", faculty, validationBody("invalid_type", "universityId", "Invalid input: expected number, received undefined")},
		{"学部の書き換え：ID の形", "PATCH", "/api/admin/faculties/1111111111111111", faculty, validationBody("invalid_format", "id", badID)},
		{"学部の削除：ID の形", "DELETE", "/api/admin/faculties/0", "", validationBody("invalid_format", "id", badID)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminRequest(newAdminMasterTestRouter(), tt.method, tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400（本文 %s）", rec.Code, rec.Body.String())
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), tt.want)
		})
	}

	t.Run("削除に受け付けない本文は 415（DB より先）", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/admin/universities/7", strings.NewReader("x"))
		req.Header.Set("Content-Type", "text/html")
		req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
		rec := httptest.NewRecorder()
		newAdminMasterTestRouter().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
