package admin

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/university"
)

// 管理者ページのマスター編集のうち、学部とタグ（/api/admin/faculties・/api/admin/tags）。
// 共通の部品と全体の決まりは masters.go。

// ---- 入口 ----

// ListTags は GET /api/admin/tags。
func (h *MasterHandlers) ListTags(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	tags, err := h.store.listTags(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin tags: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, tags)
}

// CreateFaculty は POST /api/admin/faculties。
func (h *MasterHandlers) CreateFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	input, in := readFacultyInput(body.Value(), true)
	if in.Reject(w) {
		return
	}
	outcome, err := h.store.createFaculty(r.Context(), input)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin create faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "Faculty", outcome.value.ID, "after", outcome.value)
	httpx.WriteJSON(w, http.StatusCreated, outcome.value)
}

// UpdateFaculty は PATCH /api/admin/faculties/{id}。
func (h *MasterHandlers) UpdateFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readFacultyInput(body.Value(), false)
	if in.Reject(w) {
		return
	}
	outcome, err := h.store.updateFaculty(r.Context(), id, input)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin update faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "Faculty", id, "before", outcome.value.before, "after", outcome.value.after)
	httpx.WriteJSON(w, http.StatusOK, outcome.value.after)
}

// DeleteFaculty は DELETE /api/admin/faculties/{id}。
func (h *MasterHandlers) DeleteFaculty(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteFaculty(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin delete faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "Faculty", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 入力 ----

type facultyInput struct {
	name     string
	examDate time.Time
	tagIDs   []int64
	// universityID は作成のときだけ（書き換えでは大学を移せない）。
	universityID int64
}

// record は持ち主に渡す形にする。
func (in facultyInput) record() university.FacultyInput {
	return university.FacultyInput{Name: in.name, ExamDate: in.examDate, TagIDs: in.tagIDs, UniversityID: in.universityID}
}

// readFacultyInput は facultyInputSchema（withUniversity なら createFacultySchema）。
// createFacultySchema は facultyInputSchema を extend したものなので、universityId は最後に確かめる。
func readFacultyInput(body any, withUniversity bool) (facultyInput, *httpx.ObjectInput) {
	in := httpx.ReadObject(body)
	var v facultyInput
	v.name = in.String("name", masterNameRule("学部名"))
	examDate := in.String("examDate", httpx.StringRule{Checks: []httpx.StringCheck{
		{OK: httpx.YMDPattern.MatchString, Code: "invalid_format", Message: "受験日を選んでください"},
		{OK: func(s string) bool { _, ok := examDateOf(s); return ok }, Code: "invalid_exam_date", Message: "受験日を選んでください"},
	}})
	v.tagIDs = readTagIDs(in, "tagIds")
	if withUniversity {
		v.universityID = int64(in.Number("universityId", httpx.NumberRule{Int: true, Positive: true}))
	}
	if in.Issue == nil {
		v.examDate, _ = examDateOf(examDate)
	}
	return v, in
}

// examDateOf は受験日（YYYY-MM-DD）を、Node が保存する new Date("YYYY-MM-DD")（UTC の0時）と同じ日時にする。
// JavaScript の new Date は月が 01〜12・日が 01〜31 なら受け付け、その月に無い日は繰り上げる
// （2027-02-30 は 3月2日）。time.Date も同じく繰り上げる。形（httpx.YMDPattern）は呼ぶ前に確かめておくこと。
func examDateOf(s string) (time.Time, bool) {
	year, _ := strconv.Atoi(s[0:4])
	month, _ := strconv.Atoi(s[5:7])
	day, _ := strconv.Atoi(s[8:10])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

// readTagIDs は z.array(z.number().int().positive()).max(20, …).refine(重ならない)。
// Zod は要素 → max → refine の順に確かめる（要素が不正なら、21個でも要素の issue が先）。
func readTagIDs(in *httpx.ObjectInput, key string) []int64 {
	items := in.Array(key, "")
	if in.Issue != nil {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for i, item := range items {
		f, issue := httpx.CheckNumber(in.Field(key)+"."+strconv.Itoa(i), item, httpx.NumberRule{Int: true, Positive: true})
		if issue != nil {
			in.Issue = issue
			return nil
		}
		ids = append(ids, int64(f))
	}
	if len(ids) > adminFacultyTagsMax {
		in.AddIssue("too_big", key, "タグは20個までです")
		return nil
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			in.AddIssue("duplicate_tags", key, "同じタグが重なっています")
			return nil
		}
		seen[id] = true
	}
	return ids
}

// ---- DB ----

func (st *sqlAdminMasterStore) listTags(ctx context.Context) ([]apischema.AdminTag, error) {
	rows, err := st.db.QueryContext(ctx, "SELECT id, name FROM Tag ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []apischema.AdminTag{}
	for rows.Next() {
		var t apischema.AdminTag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (st *sqlAdminMasterStore) createFaculty(ctx context.Context, in facultyInput) (masterOutcome[apischema.AdminFacultySnapshot], error) {
	f, err := university.CreateFaculty(ctx, st.db, in.record(), dates.NowMillis())
	return masterOutcomeOf(apischema.AdminFacultySnapshot(f), err, st.universitiesChanged)
}

func (st *sqlAdminMasterStore) updateFaculty(ctx context.Context, id int64, in facultyInput) (masterOutcome[masterChange[apischema.AdminFacultySnapshot]], error) {
	c, err := university.UpdateFaculty(ctx, st.db, id, in.record())
	return masterOutcomeOf(masterChange[apischema.AdminFacultySnapshot]{before: apischema.AdminFacultySnapshot(c.Before), after: apischema.AdminFacultySnapshot(c.After)}, err, st.universitiesChanged)
}

func (st *sqlAdminMasterStore) deleteFaculty(ctx context.Context, id int64) (masterOutcome[apischema.AdminFacultySnapshot], error) {
	f, err := university.DeleteFaculty(ctx, st.db, id)
	return masterOutcomeOf(apischema.AdminFacultySnapshot(f), err, st.universitiesChanged)
}
