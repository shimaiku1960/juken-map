// Package chaos は障害注入の実験（JUK-173）の入口。機械の入口（/api/chaos/、CHAOS_ENABLED=on のときだけ）で始め、
// 機械の入口と管理画面（/api/admin/chaos）で止める。障害を起こすのは internal/fault、書き込みは internal/write/chaos。
package chaos

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	writechaos "github.com/shimaiku1960/juken-map/apps/api/internal/write/chaos"
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
	spec, err := parseSpec(body.JSON)
	if err == nil {
		err = spec.Validate(h.routes())
	}
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := h.now()
	id, err := writechaos.Start(r.Context(), h.db, writechaos.Experiment{
		Kind:       string(spec.Kind),
		Route:      spec.Route,
		Rate:       spec.Rate,
		DelayMs:    spec.DelayMs,
		StatusCode: spec.StatusCode,
		StartsAt:   now,
		EndsAt:     now.Add(spec.Duration),
	}, now)
	if errors.Is(err, writechaos.ErrRunning) {
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
	n, err := writechaos.StopAll(r.Context(), h.db, writechaos.StoppedByJob, h.now())
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
	records, err := fault.Finished(r.Context(), h.db, now, recentLimit)
	if err != nil {
		httpx.InternalError(w, r, fmt.Errorf("admin chaos state: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, apischema.AdminChaosState{Enabled: h.injector != nil, Experiments: toSchemas(records, now)})
}

// AdminStop は POST /api/admin/chaos/stop。練習をやめたいときの緊急停止。実行中の実験があったかどうかは返さない
// （押して確かめることで答えが分からないようにする）。
func (h *Handlers) AdminStop(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	if _, err := writechaos.StopAll(r.Context(), h.db, writechaos.StoppedByAdmin+s.UserID, h.now()); err != nil {
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
	records, err := fault.Recent(ctx, h.db, recentLimit)
	if err != nil {
		return nil, err
	}
	return toSchemas(records, h.now()), nil
}

func (h *Handlers) routeList() []string {
	if routes := h.routes(); routes != nil {
		return routes
	}
	return []string{}
}

func (h *Handlers) find(ctx context.Context, id int64) (apischema.ChaosExperiment, error) {
	records, err := fault.Recent(ctx, h.db, recentLimit)
	if err != nil {
		return apischema.ChaosExperiment{}, err
	}
	for _, rec := range records {
		if rec.ID == id {
			return toSchema(rec, h.now()), nil
		}
	}
	return apischema.ChaosExperiment{}, fmt.Errorf("experiment %d not found", id)
}

func toSchemas(records []fault.Record, now time.Time) []apischema.ChaosExperiment {
	list := make([]apischema.ChaosExperiment, len(records))
	for i, rec := range records {
		list[i] = toSchema(rec, now)
	}
	return list
}

func toSchema(rec fault.Record, now time.Time) apischema.ChaosExperiment {
	e := apischema.ChaosExperiment{
		ID:         rec.ID,
		Kind:       apischema.ChaosRecordKind(rec.Kind),
		Route:      rec.Route,
		Rate:       rec.Rate,
		DelayMs:    rec.DelayMs,
		StatusCode: rec.StatusCode,
		StartsAt:   iso(rec.StartsAt),
		EndsAt:     iso(rec.EndsAt),
		Status:     apischema.ChaosStatusEnded,
	}
	switch {
	case rec.StoppedAt != nil:
		stoppedAt := iso(*rec.StoppedAt)
		stoppedBy := rec.StoppedBy
		e.StoppedAt, e.StoppedBy = &stoppedAt, &stoppedBy
		e.Status = apischema.ChaosStatusStopped
	case now.Before(rec.EndsAt):
		e.Status = apischema.ChaosStatusRunning
	}
	return e
}

func iso(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// parseSpec は POST /api/chaos/experiments の本文を読む。余計なキーは無視する。値の範囲は fault.Spec.Validate が確かめる。
func parseSpec(body any) (fault.Spec, error) {
	m, ok := body.(map[string]any)
	if !ok {
		return fault.Spec{}, errors.New("本文は JSON のオブジェクトにしてください")
	}
	kind, _ := m["kind"].(string)
	route, _ := m["route"].(string)
	rate, ok := number(m["rate"])
	if !ok {
		return fault.Spec{}, errors.New("rate は数にしてください")
	}
	seconds, ok := integer(m["durationSeconds"])
	if !ok {
		return fault.Spec{}, errors.New("durationSeconds は整数にしてください")
	}
	spec := fault.Spec{Kind: fault.Kind(kind), Route: route, Rate: rate, Duration: time.Duration(seconds) * time.Second}
	for _, f := range []struct {
		key string
		dst *int
	}{{"delayMs", &spec.DelayMs}, {"statusCode", &spec.StatusCode}} {
		v, present := m[f.key]
		if !present {
			continue
		}
		n, ok := integer(v)
		if !ok {
			return fault.Spec{}, fmt.Errorf("%s は整数にしてください", f.key)
		}
		*f.dst = int(n)
	}
	return spec, nil
}

func number(v any) (float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := n.Float64()
	return f, err == nil
}

// integer は小数を含まない数（1.0 は整数）。範囲は呼び出し側が確かめるので、ここでは int に収まるかだけを見る。
func integer(v any) (int64, bool) {
	f, ok := number(v)
	if !ok || f != math.Trunc(f) || math.Abs(f) > httpx.MaxSafeInteger {
		return 0, false
	}
	return int64(f), true
}
