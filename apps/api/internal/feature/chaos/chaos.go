// Package chaos は障害注入の実験（JUK-173）のコントローラー。機械の入口（/api/chaos/、CHAOS_ENABLED=on のときだけ）で始め、
// 機械の入口と管理画面（/api/admin/chaos）で止める。障害を起こすのは internal/fault、実験の入力の決まり・読み書きは
// モデルの internal/model/chaos（JUK-226）、応答の形は view.go。
package chaos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	chaosmodel "github.com/shimaiku1960/juken-map/apps/api/internal/model/chaos"
)

// recentLimit は記録を返す件数。
const recentLimit = 20

type Handlers struct {
	db *sql.DB
	// injector は CHAOS_ENABLED=on のときだけある。始めた・止めたものを、このプロセスではその場で効かせる。
	injector *fault.Injector
	// routes は障害注入の対象にしてよいルート（httpx.Router.FaultRoutes）。全ルートを登録し終えてから読む。
	routes func() []string
	now    func() time.Time // DATETIME(3) に書くので、ミリ秒で切り捨てた時刻

}

func New(db *sql.DB, injector *fault.Injector, routes func() []string) *Handlers {
	return &Handlers{db: db, injector: injector, routes: routes, now: dates.NowMillis}
}

// Start は POST /api/chaos/experiments。
func (h *Handlers) Start(w http.ResponseWriter, r *http.Request) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, err := chaosmodel.StartFromBody(r.Context(), h.db, body.JSON, h.routes(), h.now())
	var invalid chaosmodel.InputError
	if errors.As(err, &invalid) {
		httpx.WriteError(w, http.StatusBadRequest, invalid.Error())
		return
	}
	if errors.Is(err, chaosmodel.ErrRunning) {
		httpx.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("chaos start: %w", err))
		return
	}
	h.refresh(r.Context())
	record, err := h.find(r.Context(), id)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("chaos start: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, record)
}

// List は GET /api/chaos/experiments。
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.recent(r.Context())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("chaos list: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.ChaosExperimentList{Experiments: list, Routes: h.routeList()})
}

// StopAll は POST /api/chaos/experiments/stop。
func (h *Handlers) StopAll(w http.ResponseWriter, r *http.Request) {
	n, err := chaosmodel.StopAll(r.Context(), h.db, chaosmodel.StoppedByJob, h.now())
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("chaos stop all: %w", err))
		return
	}
	h.refresh(r.Context())
	httpx.WriteJSON(w, http.StatusOK, apischema.ChaosStopResult{Stopped: n})
}

// AdminState は GET /api/admin/chaos。実行中の実験は返さない。予告なしの障害で原因を調べる練習をするので、
// 管理画面が答えにならないようにする（JUK-178）。
func (h *Handlers) AdminState(w http.ResponseWriter, r *http.Request, _ *httpx.Session) {
	now := h.now()
	records, err := chaosmodel.Finished(r.Context(), h.db, now, recentLimit)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin chaos state: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.AdminChaosState{Enabled: h.injector != nil, Experiments: experimentsView(records, now)})
}

// AdminStop は POST /api/admin/chaos/stop。練習をやめたいときの緊急停止。実行中の実験があったかどうかは返さない
// （押して確かめることで答えが分からないようにする）。
func (h *Handlers) AdminStop(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, err := chaosmodel.StopAll(r.Context(), h.db, chaosmodel.StoppedByAdmin+s.UserID, h.now()); err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin chaos stop: %w", err))
		return
	}
	h.refresh(r.Context())
	w.WriteHeader(http.StatusNoContent)
}

// refresh は、このプロセスの Injector にすぐ読み直させる。失敗しても次の定期の読み込みで追いつくので、応答は変えない。
func (h *Handlers) refresh(ctx context.Context) {
	_ = h.injector.Refresh(context.WithoutCancel(ctx), h.db)
}

func (h *Handlers) recent(ctx context.Context) ([]apischema.ChaosExperiment, error) {
	records, err := chaosmodel.Recent(ctx, h.db, recentLimit)
	if err != nil {
		return nil, err
	}
	return experimentsView(records, h.now()), nil
}

func (h *Handlers) routeList() []string {
	if routes := h.routes(); routes != nil {
		return routes
	}
	return []string{}
}

func (h *Handlers) find(ctx context.Context, id int64) (apischema.ChaosExperiment, error) {
	records, err := chaosmodel.Recent(ctx, h.db, recentLimit)
	if err != nil {
		return apischema.ChaosExperiment{}, err
	}
	for _, rec := range records {
		if rec.ID == id {
			return experimentView(rec, h.now()), nil
		}
	}
	return apischema.ChaosExperiment{}, fmt.Errorf("experiment %d not found", id)
}
