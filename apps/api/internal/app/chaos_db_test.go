//go:build dbtest

// 障害注入（JUK-173）を、本番と同じ registerRoutes と本物の MySQL で通しで確かめる。
// 機械の入口で始める → 対象のルートが失敗する → 管理画面で止める → 元に戻る、の流れと、入口の断り方を見る。
// くじ引き・遅延・DB と外部 API への差し込みは internal/fault のテストが確かめる。
package app

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dbtest"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

func TestChaosDB(t *testing.T) {
	db := dbtest.Open(t)
	fx := dbtest.Fixture{T: t, DB: db}
	fx.Lock(dbtest.ChaosLock)
	const secret = "test-chaos-secret"
	t.Cleanup(func() {
		fx.Exec("DELETE FROM `ChaosExperiment` WHERE stoppedBy = 'admin:chaos-db-test' OR route = 'GET /api/health'")
	})

	injector := fault.New()
	app := newDBAdminAppWith(t, db, jobConfig{chaos: injector, chaosSecret: secret})
	app.rt.Faults = injector
	// 本番では middleware.go が付けるリクエストの情報を、ここで付ける（障害注入の対象かどうかはここに入る）。
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		req = req.WithContext(telemetry.WithRequestInfo(req.Context(), &telemetry.RequestInfo{}))
		rec := httptest.NewRecorder()
		app.rt.ServeHTTP(rec, req)
		return rec
	}
	job := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return serve(req)
	}
	health := func() int { return serve(httptest.NewRequest(http.MethodGet, "/api/health", nil)).Code }
	const start = `{"kind":"http_error","route":"GET /api/health","rate":1,"statusCode":503,"durationSeconds":300}`

	if code := job(http.MethodPost, "/api/chaos/experiments", start, "wrong").Code; code != http.StatusUnauthorized {
		t.Fatalf("秘密が違っても始められた: %d", code)
	}
	for _, body := range []string{
		`{"kind":"http_error","route":"GET /api/health","rate":1,"statusCode":503}`,                           // 終わる時刻が無い
		`{"kind":"http_error","route":"GET /api/admin/chaos","rate":1,"statusCode":503,"durationSeconds":60}`, // 止める道
		`{"kind":"http_error","route":"*","rate":1,"statusCode":404,"durationSeconds":60}`,
	} {
		if rec := job(http.MethodPost, "/api/chaos/experiments", body, secret); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d（%s）", body, rec.Code, rec.Body.String())
		}
	}

	var started apischema.ChaosExperiment
	app.expect(job(http.MethodPost, "/api/chaos/experiments", start, secret), http.StatusCreated, &started)
	if started.Status != apischema.ChaosStatusRunning || started.StatusCode != 503 {
		t.Fatalf("started = %+v", started)
	}
	app.expect(job(http.MethodPost, "/api/chaos/experiments", start, secret), http.StatusConflict, nil)

	// 始めた入口が同じプロセスの Injector をその場で読み直すので、次のリクエストから失敗する。
	if code := health(); code != http.StatusServiceUnavailable {
		t.Fatalf("障害が起きていない: %d", code)
	}

	// 管理画面には、実行中の実験を出さない（調べる練習で答えにならないように）。
	adminState := func() apischema.AdminChaosState {
		var state apischema.AdminChaosState
		app.expect(app.send(http.MethodGet, "/api/admin/chaos", "", "chaos-db-test"), http.StatusOK, &state)
		return state
	}
	findStarted := func(state apischema.AdminChaosState) *apischema.ChaosExperiment {
		for i, e := range state.Experiments {
			if e.ID == started.ID {
				return &state.Experiments[i]
			}
		}
		return nil
	}
	if state := adminState(); !state.Enabled || findStarted(state) != nil {
		t.Fatalf("実行中の実験が管理画面に出ている: %+v", state)
	}

	// 障害を起こせるルートは機械の入口だけが返す。止める道（管理画面・障害注入の入口）は含まない。
	var list apischema.ChaosExperimentList
	app.expect(job(http.MethodGet, "/api/chaos/experiments", "", secret), http.StatusOK, &list)
	if !slices.Contains(list.Routes, "GET /api/health") || slices.Contains(list.Routes, "GET /api/admin/chaos") || slices.Contains(list.Routes, "POST /api/chaos/experiments") {
		t.Errorf("routes = %v", list.Routes)
	}

	// 緊急停止は、実行中の実験があってもなくても 204 で、止めたかどうかを返さない。
	app.expect(app.send(http.MethodPost, "/api/admin/chaos/stop", "", "chaos-db-test"), http.StatusNoContent, nil)
	if code := health(); code != http.StatusOK {
		t.Fatalf("止めても戻らない: %d", code)
	}
	app.expect(app.send(http.MethodPost, "/api/admin/chaos/stop", "", "chaos-db-test"), http.StatusNoContent, nil)
	stopped := findStarted(adminState())
	if stopped == nil || stopped.Status != apischema.ChaosStatusStopped || stopped.StoppedBy == nil || *stopped.StoppedBy != "admin:chaos-db-test" {
		t.Fatalf("止めた実験が記録に無い: %+v", stopped)
	}

	// 機械の入口からも、まとめて止められる。
	app.expect(job(http.MethodPost, "/api/chaos/experiments", start, secret), http.StatusCreated, nil)
	var result apischema.ChaosStopResult
	app.expect(job(http.MethodPost, "/api/chaos/experiments/stop", "", secret), http.StatusOK, &result)
	if result.Stopped != 1 || health() != http.StatusOK {
		t.Fatalf("stopped = %d", result.Stopped)
	}
}
