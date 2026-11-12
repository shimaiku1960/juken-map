// Package universities は大学の読み取りのコントローラー（JUK-73・JUK-224）。
//   - GET /api/universities       大学を探す画面の一覧。全員に同じもの
//   - GET /api/universities/{id}  大学詳細。学部・タグと、自分が志望校に登録済みの学部
//
// 読み取りはモデル（internal/model/university・internal/model/goal）にあり、ここは HTTP と応答の形（view.go）だけを持つ。
// 大学・学部・タグの編集は管理画面（internal/feature/admin/universities.go・internal/feature/admin/faculties.go）にあり、変えたら一覧のキャッシュを捨てる（InvalidateExplore）。
package universities

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/goal"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// 大学一覧は全員に同じもので、変わるのは管理画面でマスターを編集したときだけ。毎回 DB を引くと
// 一番重い API（大学 823 件 × LEFT JOIN 3本）になるので、JSON にした状態でメモリに持つ（internal/httpx/json_snapshot.go）。
// 管理画面の編集（internal/feature/admin/universities.go・internal/feature/admin/faculties.go）は InvalidateExplore で捨てる。
const exploreCacheTTL = 10 * time.Minute

type Handlers struct {
	db      *sql.DB
	explore *httpx.JSONSnapshotCache
}

func New(db *sql.DB) *Handlers {
	h := &Handlers{db: db}
	h.explore = httpx.NewJSONSnapshotCache(exploreCacheTTL, func(ctx context.Context) (any, error) {
		universities, err := university.ListForExplore(ctx, db)
		if err != nil {
			return nil, err
		}
		return exploreView(universities), nil
	})
	return h
}

// InvalidateExplore は大学を探す画面の一覧のキャッシュを捨てる。管理画面で大学・学部を変えたら呼ぶ。
func (h *Handlers) InvalidateExplore() {
	h.explore.Invalidate()
}

// List は GET /api/universities。
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	snap, err := h.explore.Get(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities: %w", err))
		return
	}
	httpx.WriteJSONSnapshot(w, r, snap)
}

// Detail は GET /api/universities/{id}。無い大学は 404。
func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	profile, err := university.FindProfile(r.Context(), h.db, id)
	if errors.Is(err, university.ErrNotFound) {
		httpx.WriteError(w, http.StatusNotFound, "Not found")
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities/:id: %w", err))
		return
	}
	// 画面は「この学部は登録済みか」を出し分けるので、同じ応答に含める。
	registered, err := goal.FacultyIDs(r.Context(), h.db, s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("universities/:id goals: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.UniversityDetailResponse{University: profileView(profile), RegisteredFacultyIds: registered})
}
