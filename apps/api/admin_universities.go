package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/university"
)

// 管理者ページのマスター編集のうち、大学（/api/admin/universities）。共通の部品と全体の決まりは admin_masters.go。

// ---- 入口 ----

// prefectures は47都道府県。Node の src/shared/prefectures.ts の PREFECTURES と同じ並び。
var prefectures = []string{
	"北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県",
	"茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県",
	"新潟県", "山梨県", "長野県",
	"富山県", "石川県", "福井県",
	"岐阜県", "静岡県", "愛知県", "三重県",
	"滋賀県", "京都府", "大阪府", "兵庫県", "奈良県", "和歌山県",
	"鳥取県", "島根県", "岡山県", "広島県", "山口県",
	"徳島県", "香川県", "愛媛県", "高知県",
	"福岡県", "佐賀県", "長崎県", "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
}

// listUniversities は GET /api/admin/universities。
func (h *adminMasterHandlers) listUniversities(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	query := parseQuery(r.URL.RawQuery)
	q, issue := readMasterSearchQuery(query)
	if issue == nil {
		var page int
		page, issue = readPageQuery(query, adminUniversitiesMaxPage)
		if issue == nil {
			list, err := h.store.listUniversities(r.Context(), q, page)
			if err != nil {
				httpx.InternalError(w, r, fmt.Errorf("admin universities: %w", err))
				return
			}
			httpx.WriteJSON(w, http.StatusOK, list)
			return
		}
	}
	issue.Write(w)
}

// universityDetail は GET /api/admin/universities/{id}。
func (h *adminMasterHandlers) universityDetail(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	detail, err := h.store.universityDetail(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin university detail: %w", err))
		return
	}
	if detail == nil {
		httpx.WriteError(w, http.StatusNotFound, universityMessages.notFound)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, detail)
}

// createUniversity は POST /api/admin/universities。
func (h *adminMasterHandlers) createUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	input, in := readUniversityInput(body.Value())
	if in.Reject(w) {
		return
	}
	outcome, err := h.store.createUniversity(r.Context(), input)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin create university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "University", outcome.value.ID, "after", outcome.value)
	httpx.WriteJSON(w, http.StatusCreated, outcome.value)
}

// updateUniversity は PATCH /api/admin/universities/{id}。
func (h *adminMasterHandlers) updateUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	// Node と同じく path を先に、本文を後に確かめる。
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readUniversityInput(body.Value())
	if in.Reject(w) {
		return
	}
	outcome, err := h.store.updateUniversity(r.Context(), id, input)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin update university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "University", id, "before", outcome.value.before, "after", outcome.value.after)
	httpx.WriteJSON(w, http.StatusOK, outcome.value.after)
}

// deleteUniversity は DELETE /api/admin/universities/{id}。
func (h *adminMasterHandlers) deleteUniversity(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	// 本文は使わないが、Node（Fastify）はハンドラより先に本文を読むので、受け付けない形なら同じく 415・413 にする。
	if _, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteUniversity(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin delete university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "University", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 入力 ----

type universityInput struct {
	name, prefecture, typ string
}

// record は持ち主に渡す形にする。
func (in universityInput) record() university.UniversityInput {
	return university.UniversityInput{Name: in.name, Prefecture: in.prefecture, Type: in.typ}
}

// readUniversityInput は universityInputSchema。
func readUniversityInput(body any) (universityInput, *httpx.ObjectInput) {
	in := httpx.ReadObject(body)
	v := universityInput{
		name:       in.String("name", masterNameRule("大学名")),
		prefecture: in.String("prefecture", httpx.StringRule{Checks: []httpx.StringCheck{httpx.OneOf(prefectures, "invalid_prefecture", "都道府県を選んでください")}}),
		typ:        in.Enum("type", func(s string) bool { return apischema.UniversityInputType(s).Valid() }, "種別を選んでください"),
	}
	return v, in
}

// ---- DB ----

const adminUniversityColumns = `u.id, u.name, u.prefecture, u.type,
  (SELECT COUNT(*) FROM Faculty f WHERE f.universityId = u.id) AS facultyCount,
  (SELECT COUNT(*) FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId WHERE f.universityId = u.id) AS goalCount`

func scanAdminUniversity(scan func(...any) error) (apischema.AdminUniversity, error) {
	var u apischema.AdminUniversity
	err := scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.FacultyCount, &u.GoalCount)
	return u, err
}

func (st *sqlAdminMasterStore) listUniversities(ctx context.Context, q string, page int) (apischema.AdminUniversityList, error) {
	list := apischema.AdminUniversityList{Universities: []apischema.AdminUniversity{}, Page: page, PageSize: adminUniversitiesPageSize}
	where, params := "", []any{}
	if q != "" {
		where, params = "WHERE u.name LIKE ?", append(params, "%"+escapeLike(q)+"%")
	}
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM University u "+where, params...).Scan(&list.Total); err != nil {
		return list, err
	}
	// #nosec G202 -- adminUniversityColumns は固定の列の並び、where は固定の条件と ? だけ。値は params で渡す
	rows, err := st.db.QueryContext(ctx,
		"SELECT "+adminUniversityColumns+" FROM University u "+where+" ORDER BY u.name ASC, u.id ASC LIMIT ? OFFSET ?",
		append(params, adminUniversitiesPageSize, (page-1)*adminUniversitiesPageSize)...)
	if err != nil {
		return list, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanAdminUniversity(rows.Scan)
		if err != nil {
			return list, err
		}
		list.Universities = append(list.Universities, u)
	}
	return list, rows.Err()
}

// findAdminUniversity は大学を1件引く。無ければ nil。
func findAdminUniversity(ctx context.Context, db database.Runner, id int64) (*apischema.AdminUniversity, error) {
	u, err := scanAdminUniversity(db.QueryRowContext(ctx, "SELECT "+adminUniversityColumns+" FROM University u WHERE u.id = ?", id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (st *sqlAdminMasterStore) universityDetail(ctx context.Context, id int64) (*apischema.AdminUniversityDetail, error) {
	university, err := findAdminUniversity(ctx, st.db, id)
	if err != nil || university == nil {
		return nil, err
	}
	rows, err := st.db.QueryContext(ctx,
		`SELECT f.id, f.name, f.examDate,
		        (SELECT COUNT(*) FROM FinalGoal g WHERE g.facultyId = f.id) AS goalCount,
		        t.id AS tagId, t.name AS tagName
		 FROM Faculty f
		 LEFT JOIN _FacultyToTag ft ON ft.A = f.id
		 LEFT JOIN Tag t ON t.id = ft.B
		 WHERE f.universityId = ?
		 ORDER BY f.id ASC, t.id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	detail := &apischema.AdminUniversityDetail{University: *university, Faculties: []apischema.AdminFaculty{}}
	for rows.Next() {
		var (
			f        apischema.AdminFaculty
			examDate string
			tagID    *int64
			tagName  *string
		)
		if err := rows.Scan(&f.ID, &f.Name, &examDate, &f.GoalCount, &tagID, &tagName); err != nil {
			return nil, err
		}
		// 行は（学部 × タグ）の数だけ並ぶ。同じ学部の行は隣り合うので、直前と比べて束ねる。
		if n := len(detail.Faculties); n == 0 || detail.Faculties[n-1].ID != f.ID {
			f.ExamDate = examDate[:10] // DATETIME の日付の部分（Node の toISOString().slice(0, 10)）
			f.Tags = []apischema.AdminTag{}
			detail.Faculties = append(detail.Faculties, f)
		}
		if tagID != nil && tagName != nil {
			last := &detail.Faculties[len(detail.Faculties)-1]
			last.Tags = append(last.Tags, apischema.AdminTag{ID: *tagID, Name: *tagName})
		}
	}
	return detail, rows.Err()
}

func (st *sqlAdminMasterStore) createUniversity(ctx context.Context, in universityInput) (masterOutcome[apischema.AdminUniversity], error) {
	u, err := university.CreateUniversity(ctx, st.db, in.record(), nowMillis())
	return masterOutcomeOf(apischema.AdminUniversity(u), err, st.universitiesChanged)
}

func (st *sqlAdminMasterStore) updateUniversity(ctx context.Context, id int64, in universityInput) (masterOutcome[masterChange[apischema.AdminUniversity]], error) {
	c, err := university.UpdateUniversity(ctx, st.db, id, in.record())
	return masterOutcomeOf(masterChange[apischema.AdminUniversity]{before: apischema.AdminUniversity(c.Before), after: apischema.AdminUniversity(c.After)}, err, st.universitiesChanged)
}

func (st *sqlAdminMasterStore) deleteUniversity(ctx context.Context, id int64) (masterOutcome[apischema.AdminUniversity], error) {
	u, err := university.DeleteUniversity(ctx, st.db, id)
	return masterOutcomeOf(apischema.AdminUniversity(u), err, st.universitiesChanged)
}
