// Package textbooks は参考書の入口（GET・POST /api/textbooks、PATCH /api/textbooks/{id}、GET /api/textbook-masters）。
// コントローラーとビューにあたる。入力の決まり・読み取り・書き込みはモデルの internal/model/textbook にある（JUK-223）。
package textbooks

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbook"
)

type Handlers struct {
	db *sql.DB
	// masters は GET /api/textbook-masters の JSON。全員に同じで、変わるのは管理画面の編集
	// （internal/feature/admin/textbook_masters.go）だけなので、メモリに持つ（internal/httpx/json_snapshot.go、JUK-50）。
	masters *httpx.JSONSnapshotCache
}

// textbookMastersCacheTTL は、DB を直接書き換えたとき（seed など）の保険。大学一覧と同じ10分。
const textbookMastersCacheTTL = 10 * time.Minute

// New は参考書の入口を作る。
func New(db *sql.DB) *Handlers {
	h := &Handlers{db: db}
	h.masters = httpx.NewJSONSnapshotCache(textbookMastersCacheTTL, func(ctx context.Context) (any, error) {
		masters, err := textbook.ListMasters(ctx, h.db)
		if err != nil {
			return nil, err
		}
		return mastersView(masters), nil
	})
	return h
}

// InvalidateMasters は参考書マスターの応答のキャッシュを捨てる。管理画面でマスターを変えたら呼ぶ。
func (h *Handlers) InvalidateMasters() {
	h.masters.Invalidate()
}

// List は GET /api/textbooks。
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	textbooks, err := textbook.List(r.Context(), h.db, s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("textbooks: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, listView(textbooks))
}

// ListMasters は GET /api/textbook-masters。ログイン必須だが、中身は利用者によらない。
func (h *Handlers) ListMasters(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	snap, err := h.masters.Get(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("textbook-masters: %w", err))
		return
	}
	httpx.WriteJSONSnapshot(w, r, snap)
}
