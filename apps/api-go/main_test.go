package main

import "testing"

func TestRegisteredRoutes(t *testing.T) {
	// Go が受け持つルートと入口の種類。
	// ルートを足したらここに1行足す（nginx の振り分けも）。
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
		{"POST /api/study-plans", accessUser},
		{"PATCH /api/study-plans/{id}", accessUser},
		{"DELETE /api/study-plans/{id}", accessUser},
		{"POST /api/study-plans/{id}/complete", accessUser},
		{"GET /api/goals", accessUser},
		{"GET /api/goals/first-choice", accessUser},
		{"POST /api/goals", accessUser},
		{"PUT /api/goals/{id}", accessUser},
		{"PATCH /api/goals/{id}", accessUser},
		{"DELETE /api/goals/{id}", accessUser},
		{"GET /api/textbooks", accessUser},
		{"GET /api/textbook-masters", accessUser},
		{"POST /api/textbooks", accessUser},
		{"PATCH /api/textbooks/{id}", accessUser},
		{"GET /api/notification-preferences", accessUser},
		{"PUT /api/notification-preferences", accessUser},
		{"PUT /api/profile", accessUser},
		{"GET /api/universities", accessUser},
		{"GET /api/universities/{id}", accessUser},
		{"POST /api/analytics/registration", accessUser},
		{"POST /api/csp-report", accessAnonymousWrite},
		{"GET /api/line/connection", accessUser},
		{"DELETE /api/line/connection", accessUser},
		{"POST /api/line/account-link", accessUser},
		{"GET /api/line/oauth/start", accessOAuth},
		{"GET /api/line/oauth/callback", accessOAuth},
		{"POST /api/line/webhook", accessWebhook},
		{"POST /api/webhooks/microcms", accessWebhook},
		{"GET /api/admin/overview", accessAdmin},
		{"GET /api/admin/users", accessAdmin},
		{"POST /api/admin/users/{id}/ban", accessAdmin},
		{"POST /api/admin/users/{id}/unban", accessAdmin},
		{"DELETE /api/admin/users/{id}", accessAdmin},
		{"GET /api/admin/universities", accessAdmin},
		{"POST /api/admin/universities", accessAdmin},
		{"GET /api/admin/universities/{id}", accessAdmin},
		{"PATCH /api/admin/universities/{id}", accessAdmin},
		{"DELETE /api/admin/universities/{id}", accessAdmin},
		{"GET /api/admin/tags", accessAdmin},
		{"POST /api/admin/faculties", accessAdmin},
		{"PATCH /api/admin/faculties/{id}", accessAdmin},
		{"DELETE /api/admin/faculties/{id}", accessAdmin},
		{"GET /api/admin/textbook-masters", accessAdmin},
		{"POST /api/admin/textbook-masters", accessAdmin},
		{"PATCH /api/admin/textbook-masters/{id}", accessAdmin},
		{"DELETE /api/admin/textbook-masters/{id}", accessAdmin},
		{"POST /api/cron/daily-study-notifications", accessJob},
		{"GET /api/sim/state", accessJob},
		{"POST /api/sim/users", accessJob},
		{"PATCH /api/sim/users/{seq}", accessJob},
	}

	// ハンドラは呼ばないので、DB は nil のままでよい。
	rt := newRouter(fakeSessions(nil))
	registerRoutes(rt, nil, jobConfig{simulationEnabled: true}, lineConfig{}, microcmsWebhookConfig{})

	if len(rt.routes) != len(want) {
		t.Fatalf("routes = %v\nwant %v", rt.routes, want)
	}
	for i := range want {
		if rt.routes[i] != want[i] {
			t.Errorf("routes[%d] = %v, want %v", i, rt.routes[i], want[i])
		}
	}
}
