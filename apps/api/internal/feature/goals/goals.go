// Package goals は志望校の入口（GET・POST /api/goals、GET /api/goals/first-choice、PUT・PATCH・DELETE /api/goals/{id}）。
// コントローラーとビューにあたる。入力の決まり・読み取り・書き込みはモデルの internal/model/goal にある（JUK-222）。
package goals

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/goal"
)

type Handlers struct {
	db *sql.DB
}

func New(db *sql.DB) *Handlers { return &Handlers{db: db} }

// List は GET /api/goals。
func (h *Handlers) List(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	goals, err := goal.List(r.Context(), h.db, s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("goals: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, listView(goals))
}

// FirstChoice は GET /api/goals/first-choice。第一志望が無ければ null を返す。
func (h *Handlers) FirstChoice(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	g, err := goal.FindFirstChoice(r.Context(), h.db, s.UserID)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("goals/first-choice: %w", err))
		return
	}
	if g == nil {
		httpx.WriteJSON(w, http.StatusOK, nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, firstChoiceView(*g))
}
