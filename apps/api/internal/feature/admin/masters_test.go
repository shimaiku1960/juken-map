package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx/httpxtest"
)

// マスター編集（masters.go）のテスト。DB は偽物にする（Go の CI には DB が無い）。
// 本物の DB に通して Node と応答を比べるテスト（admin_masters_db_test.go）は、比べる相手の Node の API を
// 消したときに一緒に消した（JUK-84）。

// fakeAdminMasterStore は adminMasterStore の偽物。書き込みは受け取った入力を覚え、failure の結果を返す。
type fakeAdminMasterStore struct {
	failure masterFailure
	count   int

	universities    []universityInput
	faculties       []facultyInput
	textbookMasters []textbookMasterInput
	deleted         []int64
}

func (f *fakeAdminMasterStore) listUniversities(_ context.Context, q string, page int) (apischema.AdminUniversityList, error) {
	return apischema.AdminUniversityList{Universities: []apischema.AdminUniversity{{ID: 1, Name: q}}, Total: 1, Page: page, PageSize: adminUniversitiesPageSize}, nil
}

func (f *fakeAdminMasterStore) universityDetail(_ context.Context, id int64) (*apischema.AdminUniversityDetail, error) {
	if id != 1 {
		return nil, nil
	}
	return &apischema.AdminUniversityDetail{University: apischema.AdminUniversity{ID: 1, Name: "東京大学"}, Faculties: []apischema.AdminFaculty{}}, nil
}

func (f *fakeAdminMasterStore) listTags(context.Context) ([]apischema.AdminTag, error) {
	return []apischema.AdminTag{{ID: 1, Name: "文系"}}, nil
}

func (f *fakeAdminMasterStore) createUniversity(_ context.Context, in universityInput) (masterOutcome[apischema.AdminUniversity], error) {
	f.universities = append(f.universities, in)
	return masterOutcome[apischema.AdminUniversity]{failure: f.failure, count: f.count, value: apischema.AdminUniversity{ID: 9, Name: in.name, Prefecture: in.prefecture, Type: in.typ}}, nil
}

func (f *fakeAdminMasterStore) updateUniversity(_ context.Context, id int64, in universityInput) (masterOutcome[masterChange[apischema.AdminUniversity]], error) {
	f.universities = append(f.universities, in)
	after := apischema.AdminUniversity{ID: id, Name: in.name, Prefecture: in.prefecture, Type: in.typ}
	return masterOutcome[masterChange[apischema.AdminUniversity]]{failure: f.failure, value: masterChange[apischema.AdminUniversity]{before: apischema.AdminUniversity{ID: id, Name: "前"}, after: after}}, nil
}

func (f *fakeAdminMasterStore) deleteUniversity(_ context.Context, id int64) (masterOutcome[apischema.AdminUniversity], error) {
	f.deleted = append(f.deleted, id)
	return masterOutcome[apischema.AdminUniversity]{failure: f.failure, count: f.count, value: apischema.AdminUniversity{ID: id}}, nil
}

func (f *fakeAdminMasterStore) createFaculty(_ context.Context, in facultyInput) (masterOutcome[apischema.AdminFacultySnapshot], error) {
	f.faculties = append(f.faculties, in)
	return masterOutcome[apischema.AdminFacultySnapshot]{failure: f.failure, value: apischema.AdminFacultySnapshot{ID: 5, UniversityID: in.universityID, Name: in.name, ExamDate: in.examDate.Format(time.DateOnly), TagIds: in.tagIDs}}, nil
}

func (f *fakeAdminMasterStore) updateFaculty(_ context.Context, id int64, in facultyInput) (masterOutcome[masterChange[apischema.AdminFacultySnapshot]], error) {
	f.faculties = append(f.faculties, in)
	after := apischema.AdminFacultySnapshot{ID: id, UniversityID: 1, Name: in.name, ExamDate: in.examDate.Format(time.DateOnly), TagIds: in.tagIDs}
	return masterOutcome[masterChange[apischema.AdminFacultySnapshot]]{failure: f.failure, value: masterChange[apischema.AdminFacultySnapshot]{after: after}}, nil
}

func (f *fakeAdminMasterStore) deleteFaculty(_ context.Context, id int64) (masterOutcome[apischema.AdminFacultySnapshot], error) {
	f.deleted = append(f.deleted, id)
	return masterOutcome[apischema.AdminFacultySnapshot]{failure: f.failure, count: f.count}, nil
}

func (f *fakeAdminMasterStore) listTextbookMasters(_ context.Context, q string) ([]apischema.AdminTextbookMaster, error) {
	return []apischema.AdminTextbookMaster{{ID: 1, Name: q, Metrics: []apischema.AdminTextbookMasterMetric{}}}, nil
}

func (f *fakeAdminMasterStore) createTextbookMaster(_ context.Context, in textbookMasterInput) (masterOutcome[apischema.AdminTextbookMaster], error) {
	f.textbookMasters = append(f.textbookMasters, in)
	return masterOutcome[apischema.AdminTextbookMaster]{failure: f.failure, value: textbookMasterOf(3, in)}, nil
}

func (f *fakeAdminMasterStore) updateTextbookMaster(_ context.Context, id int64, in textbookMasterInput) (masterOutcome[masterChange[apischema.AdminTextbookMaster]], error) {
	f.textbookMasters = append(f.textbookMasters, in)
	return masterOutcome[masterChange[apischema.AdminTextbookMaster]]{failure: f.failure, value: masterChange[apischema.AdminTextbookMaster]{after: textbookMasterOf(id, in)}}, nil
}

func (f *fakeAdminMasterStore) deleteTextbookMaster(_ context.Context, id int64) (masterOutcome[apischema.AdminTextbookMaster], error) {
	f.deleted = append(f.deleted, id)
	return masterOutcome[apischema.AdminTextbookMaster]{failure: f.failure, count: f.count}, nil
}

func textbookMasterOf(id int64, in textbookMasterInput) apischema.AdminTextbookMaster {
	return apischema.AdminTextbookMaster{ID: id, Name: in.name, Publisher: in.publisher, Edition: in.edition, Isbn: in.isbn, Metrics: in.metrics}
}

func newAdminMasterTestRouter(store *fakeAdminMasterStore) *httpx.Router {
	h := &MasterHandlers{store: store}
	rt := httpx.NewRouter(httpxtest.FakeSessions(httpxtest.Sessions))
	rt.Admin("GET /api/admin/universities", h.ListUniversities)
	rt.Admin("POST /api/admin/universities", h.CreateUniversity)
	rt.Admin("GET /api/admin/universities/{id}", h.UniversityDetail)
	rt.Admin("PATCH /api/admin/universities/{id}", h.UpdateUniversity)
	rt.Admin("DELETE /api/admin/universities/{id}", h.DeleteUniversity)
	rt.Admin("GET /api/admin/tags", h.ListTags)
	rt.Admin("POST /api/admin/faculties", h.CreateFaculty)
	rt.Admin("PATCH /api/admin/faculties/{id}", h.UpdateFaculty)
	rt.Admin("DELETE /api/admin/faculties/{id}", h.DeleteFaculty)
	rt.Admin("GET /api/admin/textbook-masters", h.ListTextbookMasters)
	rt.Admin("POST /api/admin/textbook-masters", h.CreateTextbookMaster)
	rt.Admin("PATCH /api/admin/textbook-masters/{id}", h.UpdateTextbookMaster)
	rt.Admin("DELETE /api/admin/textbook-masters/{id}", h.DeleteTextbookMaster)
	return rt
}

const (
	validUniversity     = `{"name":"大学","prefecture":"東京都","type":"国立"}`
	validFaculty        = `{"name":"学部","examDate":"2027-02-01","tagIds":[1,2],"universityId":1}`
	validTextbookMaster = `{"name":"本","publisher":"社","edition":"第2版","isbn":"978-4-00-000000-0","metrics":[{"unit":"page","totalAmount":100,"isDefault":true}]}`
)

// with は JSON のオブジェクト base の key を value（JSON）に置き換える。value が "" ならキーを消す。
func with(base, key, value string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &m); err != nil {
		panic(err)
	}
	if value == "" {
		delete(m, key)
	} else {
		m[key] = json.RawMessage(value)
	}
	out, _ := json.Marshal(m)
	return string(out)
}

// validation は 400 の ValidationError の本文。
func validation(code, field, message string) string {
	f := "null"
	if field != "" {
		f = `"` + field + `"`
	}
	return `{"error":"` + message + `","code":"` + code + `","field":` + f + `}`
}

// TestAdminMasterValidation は入力チェック。期待値は Node（Zod のスキーマ）に同じ本文を渡して得たもの。
func TestAdminMasterValidation(t *testing.T) {
	metric := func(unit, total, isDefault string) string {
		return `{"unit":` + unit + `,"totalAmount":` + total + `,"isDefault":` + isDefault + `}`
	}
	seq := func(n int, item func(i int) string) string {
		items := make([]string, n)
		for i := range items {
			items[i] = item(i)
		}
		return "[" + strings.Join(items, ",") + "]"
	}
	units := []string{`"page"`, `"question"`, `"chapter"`, `"number"`, `"part"`, `"section"`}

	tests := []struct {
		name, method, path, body string
		want                     string // 400 の本文
	}{
		// 大学
		{"本文なし", "POST", "/api/admin/universities", "", validation("invalid_type", "", "Invalid input: expected object, received undefined")},
		{"本文が配列", "POST", "/api/admin/universities", `[]`, validation("invalid_type", "", "Invalid input: expected object, received array")},
		{"大学名なし", "POST", "/api/admin/universities", `{}`, validation("invalid_type", "name", "Invalid input: expected string, received undefined")},
		{"大学名が空白だけ（削ってから min）", "POST", "/api/admin/universities", with(validUniversity, "name", `"  "`), validation("too_small", "name", "大学名を入力してください")},
		{"大学名が101文字", "POST", "/api/admin/universities", with(validUniversity, "name", `"`+strings.Repeat("a", 101)+`"`), validation("too_big", "name", "大学名は100文字以内で入力してください")},
		{"大学名が数", "POST", "/api/admin/universities", with(validUniversity, "name", `1`), validation("invalid_type", "name", "Invalid input: expected string, received number")},
		{"都道府県でない", "POST", "/api/admin/universities", with(validUniversity, "prefecture", `"東京"`), validation("invalid_prefecture", "prefecture", "都道府県を選んでください")},
		{"都道府県が数", "POST", "/api/admin/universities", with(validUniversity, "prefecture", `1`), validation("invalid_type", "prefecture", "Invalid input: expected string, received number")},
		{"種別が違う", "POST", "/api/admin/universities", with(validUniversity, "type", `"x"`), validation("invalid_value", "type", "種別を選んでください")},
		{"種別が数", "POST", "/api/admin/universities", with(validUniversity, "type", `1`), validation("invalid_value", "type", "種別を選んでください")},
		{"種別なし", "POST", "/api/admin/universities", with(validUniversity, "type", ""), validation("invalid_value", "type", "種別を選んでください")},
		{"書き換え：ID の形が違えば本文より先に 400", "PATCH", "/api/admin/universities/0", `{}`, validation("invalid_format", "id", "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/")},
		{"書き換え：本文も確かめる", "PATCH", "/api/admin/universities/1", with(validUniversity, "type", `"x"`), validation("invalid_value", "type", "種別を選んでください")},
		{"詳細：ID の形", "GET", "/api/admin/universities/01", "", validation("invalid_format", "id", "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/")},
		{"削除：ID の形", "DELETE", "/api/admin/universities/abc", "", validation("invalid_format", "id", "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/")},
		{"一覧：q が101文字", "GET", "/api/admin/universities?q=" + strings.Repeat("a", 101), "", validation("too_big", "q", "Too big: expected string to have <=100 characters")},
		{"一覧：q が2つ（page より先）", "GET", "/api/admin/universities?q=a&q=b&page=x", "", validation("invalid_type", "q", "Invalid input: expected string, received array")},
		{"一覧：page が1001", "GET", "/api/admin/universities?page=1001", "", validation("too_big", "page", "Too big: expected number to be <=1000")},
		{"一覧：page が0", "GET", "/api/admin/universities?page=0", "", validation("too_small", "page", "Too small: expected number to be >0")},

		// 学部
		{"学部名が空", "POST", "/api/admin/faculties", with(validFaculty, "name", `""`), validation("too_small", "name", "学部名を入力してください")},
		{"受験日の形が違う", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"2027-2-1"`), validation("invalid_format", "examDate", "受験日を選んでください")},
		{"受験日が空", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `""`), validation("invalid_format", "examDate", "受験日を選んでください")},
		{"受験日が数", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `1`), validation("invalid_type", "examDate", "Invalid input: expected string, received number")},
		{"受験日の日が32", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"2027-02-32"`), validation("invalid_exam_date", "examDate", "受験日を選んでください")},
		{"受験日の月が13", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"2027-13-01"`), validation("invalid_exam_date", "examDate", "受験日を選んでください")},
		{"受験日の月が0", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"2027-00-10"`), validation("invalid_exam_date", "examDate", "受験日を選んでください")},
		{"受験日の日が0", "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"2027-01-00"`), validation("invalid_exam_date", "examDate", "受験日を選んでください")},
		{"タグが文字列", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `"1"`), validation("invalid_type", "tagIds", "Invalid input: expected array, received string")},
		{"タグなし", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", ""), validation("invalid_type", "tagIds", "Invalid input: expected array, received undefined")},
		{"タグが重なる", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `[1,1]`), validation("duplicate_tags", "tagIds", "同じタグが重なっています")},
		{"タグが0", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `[0]`), validation("too_small", "tagIds.0", "Too small: expected number to be >0")},
		{"タグが文字列の要素", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `["1"]`), validation("invalid_type", "tagIds.0", "Invalid input: expected number, received string")},
		{"タグが小数", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `[1.5]`), validation("invalid_type", "tagIds.0", "Invalid input: expected int, received number")},
		{"タグが21個", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", seq(21, func(i int) string { return strconv.Itoa(i + 1) })), validation("too_big", "tagIds", "タグは20個までです")},
		{"タグが21個で先頭が0（要素が先）", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", seq(22, strconv.Itoa)), validation("too_small", "tagIds.0", "Too small: expected number to be >0")},
		{"タグが21個で全部同じ（個数が重なりより先）", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", seq(21, func(int) string { return "1" })), validation("too_big", "tagIds", "タグは20個までです")},
		{"タグが重なり、3つ目が0（要素が先）", "POST", "/api/admin/faculties", with(validFaculty, "tagIds", `[1,1,0]`), validation("too_small", "tagIds.2", "Too small: expected number to be >0")},
		{"大学が文字列", "POST", "/api/admin/faculties", with(validFaculty, "universityId", `"1"`), validation("invalid_type", "universityId", "Invalid input: expected number, received string")},
		{"大学が0", "POST", "/api/admin/faculties", with(validFaculty, "universityId", `0`), validation("too_small", "universityId", "Too small: expected number to be >0")},
		{"大学なし", "POST", "/api/admin/faculties", with(validFaculty, "universityId", ""), validation("invalid_type", "universityId", "Invalid input: expected number, received undefined")},
		{"名前と大学の両方が不正なら名前（大学は最後）", "POST", "/api/admin/faculties", with(with(validFaculty, "name", `""`), "universityId", `0`), validation("too_small", "name", "学部名を入力してください")},
		{"学部の書き換え：ID の形", "PATCH", "/api/admin/faculties/1111111111111111", validFaculty, validation("invalid_format", "id", "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/")},

		// 参考書
		{"参考書名なし", "POST", "/api/admin/textbook-masters", `{}`, validation("invalid_type", "name", "Invalid input: expected string, received undefined")},
		{"参考書名が空白だけ", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "name", `"  "`), validation("too_small", "name", "参考書名を入力してください")},
		{"参考書名が151文字", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "name", `"`+strings.Repeat("a", 151)+`"`), validation("too_big", "name", "参考書名は150文字以内で入力してください")},
		{"出版社が数", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "publisher", `1`), validation("invalid_type", "publisher", "Invalid input: expected string, received number")},
		{"出版社が101文字", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "publisher", `"`+strings.Repeat("a", 101)+`"`), validation("too_big", "publisher", "100文字以内で入力してください")},
		{"版が51文字", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "edition", `"`+strings.Repeat("a", 51)+`"`), validation("too_big", "edition", "50文字以内で入力してください")},
		{"ISBN が3桁", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "isbn", `"123"`), validation("invalid_isbn", "isbn", "ISBN は10桁か13桁で入力してください")},
		{"ISBN が空", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "isbn", `""`), validation("invalid_isbn", "isbn", "ISBN は10桁か13桁で入力してください")},
		{"ISBN が数", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "isbn", `9784000000000`), validation("invalid_type", "isbn", "Invalid input: expected string, received number")},
		{"総量の候補が空", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", `[]`), validation("too_small", "metrics", "総量を1つ以上入力してください")},
		{"総量の候補が文字列", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", `"x"`), validation("invalid_type", "metrics", "Invalid input: expected array, received string")},
		{"候補が null", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", `[null]`), validation("invalid_type", "metrics.0", "Invalid input: expected object, received null")},
		{"単位が違う", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"x"`, "1", "true")+"]"), validation("invalid_range_unit", "metrics.0.unit", "単位を選んでください")},
		{"単位が数", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`1`, "1", "true")+"]"), validation("invalid_type", "metrics.0.unit", "Invalid input: expected string, received number")},
		{"総量が文字列", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, `"1"`, "true")+"]"), validation("invalid_type", "metrics.0.totalAmount", "総量を入力してください")},
		{"総量なし", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", `[{"unit":"page","isDefault":true}]`), validation("invalid_type", "metrics.0.totalAmount", "総量を入力してください")},
		{"総量が小数", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1.5", "true")+"]"), validation("invalid_type", "metrics.0.totalAmount", "総量は整数で入力してください")},
		{"総量が0", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "0", "true")+"]"), validation("too_small", "metrics.0.totalAmount", "総量は1以上で入力してください")},
		{"総量が100001", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "100001", "true")+"]"), validation("too_big", "metrics.0.totalAmount", "総量は100000以下で入力してください")},
		{"既定が文字列", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1", `"true"`)+"]"), validation("invalid_type", "metrics.0.isDefault", "Invalid input: expected boolean, received string")},
		{"単位が重なる", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"page"`, "1", "false")+"]"), validation("duplicate_units", "metrics", "同じ単位が重なっています")},
		{"既定が無い", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1", "false")+"]"), validation("default_unit_required", "metrics", "既定の単位を1つ選んでください")},
		{"既定が2つ", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"question"`, "1", "true")+"]"), validation("default_unit_required", "metrics", "既定の単位を1つ選んでください")},
		{"既定が2つで単位も重なる（重なりが先）", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"page"`, "1", "true")+"]"), validation("duplicate_units", "metrics", "同じ単位が重なっています")},
		{"候補が7つ（個数が重なりより先）", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", seq(7, func(i int) string { return metric(units[i%6], "1", "false") })), validation("too_big", "metrics", "Too big: expected array to have <=6 items")},
		{"候補が7つで先頭の単位が違う（要素が先）", "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "metrics", seq(7, func(i int) string {
			if i == 0 {
				return metric(`"x"`, "1", "false")
			}
			return metric(units[i-1], "1", "false")
		})), validation("invalid_range_unit", "metrics.0.unit", "単位を選んでください")},
		{"一覧：q が2つ", "GET", "/api/admin/textbook-masters?q=a&q=b", "", validation("invalid_type", "q", "Invalid input: expected string, received array")},
		{"一覧：q が101文字", "GET", "/api/admin/textbook-masters?q=" + strings.Repeat("a", 101), "", validation("too_big", "q", "Too big: expected string to have <=100 characters")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAdminMasterStore{}
			rec := adminRequest(newAdminMasterTestRouter(store), tt.method, tt.path, tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400（本文 %s）", rec.Code, rec.Body.String())
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), tt.want)
			if len(store.universities)+len(store.faculties)+len(store.textbookMasters)+len(store.deleted) != 0 {
				t.Error("弾いた入力が DB に届いた")
			}
		})
	}
}

// TestAdminMasterInputValues は、通った入力を Node と同じ値にして DB へ渡すこと（削る・整える・null にする）。
func TestAdminMasterInputValues(t *testing.T) {
	t.Run("大学：名前の前後の空白を削る。削った後で100文字なら通る", func(t *testing.T) {
		store := &fakeAdminMasterStore{}
		name := strings.Repeat("a", 100)
		rec := adminRequest(newAdminMasterTestRouter(store), "POST", "/api/admin/universities", with(validUniversity, "name", `" `+name+` "`))
		if rec.Code != http.StatusCreated || store.universities[0].name != name {
			t.Fatalf("status = %d, 渡した名前 = %q", rec.Code, store.universities[0].name)
		}
	})

	t.Run("学部：受験日は JavaScript の new Date と同じく繰り上げる", func(t *testing.T) {
		for in, want := range map[string]string{
			"2027-02-01": "2027-02-01", "2027-02-29": "2027-03-01", "2027-02-31": "2027-03-03", "2027-04-31": "2027-05-01",
		} {
			store := &fakeAdminMasterStore{}
			rec := adminRequest(newAdminMasterTestRouter(store), "POST", "/api/admin/faculties", with(validFaculty, "examDate", `"`+in+`"`))
			if rec.Code != http.StatusCreated {
				t.Fatalf("%s: status = %d（%s）", in, rec.Code, rec.Body.String())
			}
			if got := store.faculties[0].examDate; !got.Equal(mustDate(t, want)) || got.Location() != time.UTC {
				t.Errorf("%s → %v, want %s の UTC 0時", in, got, want)
			}
		}
	})

	t.Run("学部：タグなしも通る。書き換えは universityId を読まない", func(t *testing.T) {
		store := &fakeAdminMasterStore{}
		rec := adminRequest(newAdminMasterTestRouter(store), "PATCH", "/api/admin/faculties/3", `{"name":"学部","examDate":"2027-02-01","tagIds":[]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d（%s）", rec.Code, rec.Body.String())
		}
		httpxtest.AssertJSONEqual(t, rec.Body.String(), `{"id":3,"universityId":1,"name":"学部","examDate":"2027-02-01","tagIds":[]}`)
	})

	t.Run("参考書：ISBN を整え、空の出版社・版は null にする", func(t *testing.T) {
		for _, c := range []struct{ isbn, want string }{
			{"978-4-00-000000-0", "9784000000000"}, {"4-00-000000-x", "400000000X"}, {"4 00 000000 X", "400000000X"},
			{"978\u30004000000000", "9784000000000"}, {"\ufeff9784000000000\u00a0", "9784000000000"},
		} {
			store := &fakeAdminMasterStore{}
			body := with(with(with(validTextbookMaster, "isbn", `"`+c.isbn+`"`), "publisher", `" "`), "edition", "")
			rec := adminRequest(newAdminMasterTestRouter(store), "POST", "/api/admin/textbook-masters", body)
			if rec.Code != http.StatusCreated {
				t.Fatalf("%q: status = %d（%s）", c.isbn, rec.Code, rec.Body.String())
			}
			got := store.textbookMasters[0]
			if got.isbn != c.want || got.publisher != nil || got.edition != nil {
				t.Errorf("%q: isbn = %q, publisher = %v, edition = %v", c.isbn, got.isbn, got.publisher, got.edition)
			}
		}
	})

	t.Run("参考書：出版社・版は削ってから保存する", func(t *testing.T) {
		store := &fakeAdminMasterStore{}
		rec := adminRequest(newAdminMasterTestRouter(store), "POST", "/api/admin/textbook-masters", with(validTextbookMaster, "publisher", `"  社  "`))
		if rec.Code != http.StatusCreated || *store.textbookMasters[0].publisher != "社" {
			t.Fatalf("status = %d, publisher = %v", rec.Code, store.textbookMasters[0].publisher)
		}
	})

	t.Run("一覧：q は削ってから渡し、空なら絞らない", func(t *testing.T) {
		for raw, want := range map[string]string{"?q=%20%E6%9D%B1%20": "東", "?q=%20%20": "", "": ""} {
			rec := adminRequest(newAdminMasterTestRouter(&fakeAdminMasterStore{}), "GET", "/api/admin/universities"+raw, "")
			var list apischema.AdminUniversityList
			if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || list.Universities[0].Name != want || list.Page != 1 {
				t.Errorf("%q: status = %d, q = %q, page = %d", raw, rec.Code, list.Universities[0].Name, list.Page)
			}
		}
	})
}

// TestAdminMasterFailures は、DB 側で断った結果を Node と同じ status・文言で返すこと。
func TestAdminMasterFailures(t *testing.T) {
	tests := []struct {
		name, method, path, body string
		failure                  masterFailure
		count                    int
		wantStatus               int
		wantBody                 string
	}{
		{"大学の重複", "POST", "/api/admin/universities", validUniversity, masterDuplicate, 0, 409, `{"error":"同じ名前の大学がすでにあります"}`},
		{"大学が無い（書き換え）", "PATCH", "/api/admin/universities/7", validUniversity, masterNotFound, 0, 404, `{"error":"大学が見つかりません"}`},
		{"大学が使われている", "DELETE", "/api/admin/universities/7", "", masterInUse, 3, 409, `{"error":"この大学の学部が志望校に3件使われているため削除できません"}`},
		{"大学が無い（詳細）", "GET", "/api/admin/universities/7", "", masterOK, 0, 404, `{"error":"大学が見つかりません"}`},
		{"学部の大学が無い", "POST", "/api/admin/faculties", validFaculty, masterNotFound, 0, 404, `{"error":"学部（または大学）が見つかりません"}`},
		{"学部の重複", "PATCH", "/api/admin/faculties/7", validFaculty, masterDuplicate, 0, 409, `{"error":"この大学に同じ名前の学部がすでにあります"}`},
		{"存在しないタグ", "POST", "/api/admin/faculties", validFaculty, masterInvalidTags, 0, 400, `{"error":"存在しないタグが含まれています"}`},
		{"学部が使われている", "DELETE", "/api/admin/faculties/7", "", masterInUse, 2, 409, `{"error":"この学部が志望校に2件使われているため削除できません"}`},
		{"ISBN の重複", "PATCH", "/api/admin/textbook-masters/7", validTextbookMaster, masterDuplicate, 0, 409, `{"error":"同じ ISBN の参考書がすでにあります"}`},
		{"参考書が無い", "DELETE", "/api/admin/textbook-masters/7", "", masterNotFound, 0, 404, `{"error":"参考書が見つかりません"}`},
		{"参考書が使われている", "DELETE", "/api/admin/textbook-masters/7", "", masterInUse, 5, 409, `{"error":"この参考書は利用者の5冊に使われているため削除できません"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := adminRequest(newAdminMasterTestRouter(&fakeAdminMasterStore{failure: tt.failure, count: tt.count}), tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d（本文 %s）", rec.Code, tt.wantStatus, rec.Body.String())
			}
			httpxtest.AssertJSONEqual(t, rec.Body.String(), tt.wantBody)
		})
	}

	t.Run("削除の成功は 204 で本文なし", func(t *testing.T) {
		for _, path := range []string{"/api/admin/universities/7", "/api/admin/faculties/7", "/api/admin/textbook-masters/7"} {
			rec := adminRequest(newAdminMasterTestRouter(&fakeAdminMasterStore{}), "DELETE", path, "")
			if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
				t.Errorf("%s: status = %d, 本文 = %q", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("削除に受け付けない本文は 415（Node の Fastify と同じく、ハンドラより先）", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/admin/universities/7", strings.NewReader("x"))
		req.Header.Set("Content-Type", "text/html")
		req.AddCookie(&http.Cookie{Name: "test", Value: "admin"})
		rec := httptest.NewRecorder()
		store := &fakeAdminMasterStore{}
		newAdminMasterTestRouter(store).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType || len(store.deleted) != 0 {
			t.Fatalf("status = %d, deleted = %v", rec.Code, store.deleted)
		}
	})
}

// TestAdminMasterChangeLog は、変更が「誰が・何を・前→後」を Node と同じ形で構造化ログに残すことを確かめる。
func TestAdminMasterChangeLog(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	rt := newAdminMasterTestRouter(&fakeAdminMasterStore{})
	adminRequest(rt, "POST", "/api/admin/universities", validUniversity)
	adminRequest(rt, "PATCH", "/api/admin/universities/4", validUniversity)
	adminRequest(rt, "DELETE", "/api/admin/textbook-masters/6", "")
	// 断った操作は残さない
	adminRequest(newAdminMasterTestRouter(&fakeAdminMasterStore{failure: masterDuplicate}), "POST", "/api/admin/universities", validUniversity)

	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		m := httpxtest.DecodeJSON(t, line)
		if m["msg"] == "admin master change" {
			lines = append(lines, m)
		}
	}
	if len(lines) != 3 {
		t.Fatalf("変更のログ = %v, want 3行", lines)
	}
	want := []struct {
		action, table string
		id            float64
		keys          []string
	}{
		{"create", "University", 9, []string{"after"}},
		{"update", "University", 4, []string{"before", "after"}},
		{"delete", "TextbookMaster", 6, []string{"before"}},
	}
	for i, w := range want {
		m := lines[i]
		if m["level"] != "INFO" || m["adminId"] != "u2" || m["action"] != w.action || m["table"] != w.table || m["id"] != w.id {
			t.Errorf("%d 行目 = %v", i, m)
		}
		for _, key := range w.keys {
			if _, ok := m[key].(map[string]any); !ok {
				t.Errorf("%d 行目に %s が無い: %v", i, key, m)
			}
		}
	}
	if before := lines[1]["before"].(map[string]any); before["name"] != "前" {
		t.Errorf("書き換えの before = %v", before)
	}
}

func mustDate(t *testing.T, ymd string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, ymd)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
