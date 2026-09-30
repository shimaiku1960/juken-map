package main

import "testing"

func TestRegisteredRoutes(t *testing.T) {
	// Go が受け持つルートと入口の種類。Node の同じルートの config.access と同じであること。
	// ルートを移したらここに1行足す（parity_test.go の parityCases と nginx の振り分けも）。
	// 一覧が変わるとこのテストが落ちるので、入口の種類を取り違えたまま足すことはできない。
	want := []routeEntry{
		{"GET /api/health", accessPublic},
		{"GET /api/dashboard", accessUser},
		{"GET /api/study-logs", accessUser},
		{"GET /api/study-logs/daily", accessUser},
		{"GET /api/study-plans", accessUser},
		{"POST /api/study-logs", accessUser},
		{"PATCH /api/study-logs/{id}", accessUser},
		{"DELETE /api/study-logs/{id}", accessUser},
		{"GET /api/goals", accessUser},
		{"GET /api/goals/first-choice", accessUser},
		{"GET /api/textbooks", accessUser},
		{"GET /api/textbook-masters", accessUser},
		{"GET /api/notification-preferences", accessUser},
		{"PUT /api/notification-preferences", accessUser},
		{"PUT /api/profile", accessUser},
		{"GET /api/universities", accessUser},
		{"GET /api/universities/{id}", accessUser},
		{"POST /api/analytics/registration", accessUser},
		{"POST /api/csp-report", accessAnonymousWrite},
		{"POST /api/cron/daily-study-notifications", accessJob},
		{"GET /api/sim/state", accessJob},
		{"POST /api/sim/users", accessJob},
		{"PATCH /api/sim/users/{seq}", accessJob},
	}

	// ハンドラは呼ばないので、DB は nil のままでよい。
	rt := newRouter(fakeSessions(nil))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true})

	if len(rt.routes) != len(want) {
		t.Fatalf("routes = %v\nwant %v", rt.routes, want)
	}
	for i := range want {
		if rt.routes[i] != want[i] {
			t.Errorf("routes[%d] = %v, want %v", i, rt.routes[i], want[i])
		}
	}
}
