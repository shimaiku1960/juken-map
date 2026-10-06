package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 大学の読み取り（JUK-73）。Node の routes/universities.ts と services/university-service.ts にあたる。
//   - GET /api/universities       大学を探す画面の一覧。全員に同じもの
//   - GET /api/universities/{id}  大学詳細。学部・タグと、自分が志望校に登録済みの学部
// 大学・学部・タグの編集は管理画面（internal/feature/admin/universities.go・internal/feature/admin/faculties.go）にあり、変えたら下のキャッシュを捨てる。

// 一覧の応答の形。画面が使うのは大学の列と「学部ごとのタグ名」だけなので、Node と同じくそれだけ返す。
// JSON のバイト列を Node の JSON.stringify と同じにする（下の ETag を Node と揃えるため）ので、
// キーの並びも Node の組み立てと同じにしてある。
type exploreUniversityDTO struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Prefecture string              `json:"prefecture"`
	Type       string              `json:"type"`
	Faculties  []exploreFacultyDTO `json:"faculties"`
}

type exploreFacultyDTO struct {
	Tags []exploreTagDTO `json:"tags"`
}

type exploreTagDTO struct {
	Name string `json:"name"`
}

// 大学一覧は全員に同じもので、変わるのは管理画面でマスターを編集したときだけ。毎回 DB を引くと
// 一番重い API（大学 823 件 × LEFT JOIN 3本）になるので、JSON にした状態でメモリに持つ（internal/httpx/json_snapshot.go）。
// 管理画面の編集（internal/feature/admin/universities.go・internal/feature/admin/faculties.go）は explore.Invalidate で捨てる。
const exploreCacheTTL = 10 * time.Minute

type universityStore struct {
	db      *sql.DB
	explore *httpx.JSONSnapshotCache
}

func newUniversityStore(db *sql.DB) *universityStore {
	st := &universityStore{db: db}
	st.explore = httpx.NewJSONSnapshotCache(exploreCacheTTL, func(ctx context.Context) (any, error) {
		return st.listForExplore(ctx)
	})
	return st
}

// listForExplore は大学 → 学部 → タグを LEFT JOIN 1本で取り、入れ子へ詰め直す。
// 行は（大学 × 学部 × タグ）の数だけ並ぶ。
func (st *universityStore) listForExplore(ctx context.Context) ([]exploreUniversityDTO, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, f.id AS facultyId, t.name AS tagName
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 ORDER BY u.name ASC, u.id ASC, f.id ASC, t.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	universities := make([]exploreUniversityDTO, 0)
	var lastFacultyID *int64
	for rows.Next() {
		var (
			u         exploreUniversityDTO
			facultyID *int64
			tagName   *string
		)
		if err := rows.Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &facultyID, &tagName); err != nil {
			return nil, err
		}
		// ORDER BY で同じ大学・同じ学部の行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(universities) == 0 || universities[len(universities)-1].ID != u.ID {
			u.Faculties = make([]exploreFacultyDTO, 0)
			universities = append(universities, u)
			lastFacultyID = nil
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if facultyID == nil {
			continue
		}
		last := &universities[len(universities)-1]
		if lastFacultyID == nil || *lastFacultyID != *facultyID {
			last.Faculties = append(last.Faculties, exploreFacultyDTO{Tags: make([]exploreTagDTO, 0)})
			lastFacultyID = facultyID
		}
		if tagName != nil {
			f := &last.Faculties[len(last.Faculties)-1]
			f.Tags = append(f.Tags, exploreTagDTO{Name: *tagName})
		}
	}
	return universities, rows.Err()
}

// 大学詳細の応答の型は openapi/openapi.yaml から生成した UniversityDetailResponse。
// 学部（FacultyWithTags）は志望校の学部と違い、大学を入れ子にしない。

// findDetail は大学詳細ページ用。学部と、絞り込みに使うタグまで一度に引く。無ければ nil。
func (st *universityStore) findDetail(ctx context.Context, id int64) (*apischema.UniversityDetail, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, u.createdAt,
		        f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
		        f.createdAt AS f_createdAt, f.universityId AS f_universityId,
		        t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 WHERE u.id = ?
		 ORDER BY f.id ASC, t.id ASC`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detail *apischema.UniversityDetail
	for rows.Next() {
		var (
			u                            apischema.UniversityDetail
			fID, fUniversityID, tID      *int64
			fName, fExamDate, fCreatedAt *string
			tName, tCreatedAt            *string
		)
		if err := rows.Scan(
			&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.CreatedAt,
			&fID, &fName, &fExamDate, &fCreatedAt, &fUniversityID,
			&tID, &tName, &tCreatedAt,
		); err != nil {
			return nil, err
		}
		if detail == nil {
			u.CreatedAt = database.ISOFromDatetime(u.CreatedAt)
			u.Faculties = make([]apischema.FacultyWithTags, 0)
			detail = &u
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if fID == nil {
			continue
		}
		if n := len(detail.Faculties); n == 0 || detail.Faculties[n-1].ID != *fID {
			detail.Faculties = append(detail.Faculties, apischema.FacultyWithTags{
				ID:           *fID,
				Name:         *fName,
				ExamDate:     database.ISOFromDatetime(*fExamDate),
				CreatedAt:    database.ISOFromDatetime(*fCreatedAt),
				UniversityID: *fUniversityID,
				Tags:         make([]apischema.Tag, 0),
			})
		}
		if tID != nil {
			f := &detail.Faculties[len(detail.Faculties)-1]
			f.Tags = append(f.Tags, apischema.Tag{ID: *tID, Name: *tName, CreatedAt: database.ISOFromDatetime(*tCreatedAt)})
		}
	}
	return detail, rows.Err()
}

// listGoalFacultyIDs は志望校として登録済みの学部 ID。大学詳細で「登録済み」を出し分けるのに使う。
func (st *universityStore) listGoalFacultyIDs(ctx context.Context, userID string) ([]int64, error) {
	rows, err := st.db.QueryContext(ctx,
		"SELECT facultyId FROM FinalGoal WHERE userId = ? ORDER BY facultyId ASC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type universityHandlers struct {
	store *universityStore
}

// list は GET /api/universities。
func (h *universityHandlers) list(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	snap, err := h.store.explore.Get(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities: %w", err))
		return
	}
	httpx.WriteJSONSnapshot(w, r, snap)
}

// detail は GET /api/universities/{id}。無い大学は 404（Node と同じ文言）。
func (h *universityHandlers) detail(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	university, err := h.store.findDetail(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities/:id: %w", err))
		return
	}
	if university == nil {
		httpx.WriteError(w, http.StatusNotFound, "Not found")
		return
	}
	// 画面は「この学部は登録済みか」を出し分けるので、同じ応答に含める。
	registered, err := h.store.listGoalFacultyIDs(r.Context(), s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities/:id goals: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.UniversityDetailResponse{University: *university, RegisteredFacultyIds: registered})
}
