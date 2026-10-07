package app

import (
	"database/sql"

	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/admin"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/analytics"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/auth"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/blog"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/cspreport"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/goals"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/line"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/notifications"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/sim"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/study"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/textbooks"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/universities"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// registerRoutes は Go が受け持つルートを登録する。本番で Go へ届くのは、このうち
// infra/nginx/juken-map-go-routes.conf に書いたパスだけ。
// 一覧は main_test.go の TestRegisteredRoutes が入口の種類と一緒に確かめている。
func registerRoutes(rt *httpx.Router, db *sql.DB, jobs jobConfig, lineCfg line.Config, microcms blog.WebhookConfig) {
	studyRoutes := study.New(db)

	rt.Public("GET /api/health", healthHandler(db))
	rt.User("GET /api/dashboard", studyRoutes.Dashboard.Serve)
	rt.User("GET /api/study-logs", studyRoutes.Reads.ListLogs)
	rt.User("GET /api/study-logs/daily", studyRoutes.Reads.ListDaily)
	rt.User("GET /api/study-plans", studyRoutes.Reads.ListPlans)

	rt.User("POST /api/study-logs", studyRoutes.Logs.Create)
	rt.User("PATCH /api/study-logs/{id}", studyRoutes.Logs.Update)
	rt.User("DELETE /api/study-logs/{id}", studyRoutes.Logs.Delete)

	rt.User("POST /api/study-plans", studyRoutes.Plans.Create)
	rt.User("PATCH /api/study-plans/{id}", studyRoutes.Plans.Update)
	rt.User("DELETE /api/study-plans/{id}", studyRoutes.Plans.Delete)
	rt.User("POST /api/study-plans/{id}/complete", studyRoutes.Plans.Complete)

	goalRoutes := goals.New(db)
	rt.User("GET /api/goals", goalRoutes.List)
	rt.User("GET /api/goals/first-choice", goalRoutes.FirstChoice)
	rt.User("POST /api/goals", goalRoutes.Create)
	rt.User("PUT /api/goals/{id}", goalRoutes.Replace)
	rt.User("PATCH /api/goals/{id}", goalRoutes.Update)
	rt.User("DELETE /api/goals/{id}", goalRoutes.Delete)

	textbookRoutes := textbooks.New(db)
	rt.User("GET /api/textbooks", textbookRoutes.List)
	rt.User("GET /api/textbook-masters", textbookRoutes.ListMasters)
	rt.User("POST /api/textbooks", textbookRoutes.Create)
	rt.User("PATCH /api/textbooks/{id}", textbookRoutes.UpdateProgress)

	prefs := notifications.NewPreferenceHandlers(db)
	rt.User("GET /api/notification-preferences", prefs.Get)
	rt.User("PUT /api/notification-preferences", prefs.Save)

	profile := auth.NewProfileHandlers(db)
	rt.User("PUT /api/profile", profile.Update)

	universityRoutes := universities.New(db)
	rt.User("GET /api/universities", universityRoutes.List)
	rt.User("GET /api/universities/{id}", universityRoutes.Detail)

	analyticsRoutes := analytics.New(db)
	rt.User("POST /api/analytics/registration", analyticsRoutes.Registration)
	rt.AnonymousWrite("POST /api/csp-report", cspreport.Handle)

	lineRoutes := line.New(db, lineCfg)
	rt.User("GET /api/line/connection", lineRoutes.Connection)
	rt.User("DELETE /api/line/connection", lineRoutes.Disconnect)
	rt.User("POST /api/line/account-link", lineRoutes.AccountLink)
	rt.OAuth("GET /api/line/oauth/start", lineRoutes.OauthStart)
	rt.OAuth("GET /api/line/oauth/callback", lineRoutes.OauthCallback)
	rt.Webhook("POST /api/line/webhook", lineRoutes.Webhook)
	rt.PublicWithSession("GET /line/settings", lineRoutes.Settings)

	// microCMS で記事を変えたら、記事を作り直すデプロイを動かす（JUK-112）。
	microcmsWebhook := blog.NewWebhookHandler(microcms)
	rt.Webhook("POST /api/webhooks/microcms", microcmsWebhook.Serve)

	adminUsers := admin.NewUserHandlers(db)
	rt.Admin("GET /api/admin/overview", adminUsers.Overview)
	rt.Admin("GET /api/admin/users", adminUsers.ListUsers)
	rt.Admin("POST /api/admin/users/{id}/ban", adminUsers.Ban)
	rt.Admin("POST /api/admin/users/{id}/unban", adminUsers.Unban)
	rt.Admin("DELETE /api/admin/users/{id}", adminUsers.DeleteUser)

	// マスター編集。大学・学部を変えたら大学を探す画面の一覧（上の universityRoutes）の、参考書マスターを
	// 変えたら GET /api/textbook-masters（上の textbookRoutes）のキャッシュを捨てる。
	masters := admin.NewMasterHandlers(db, universityRoutes.InvalidateExplore, textbookRoutes.InvalidateMasters)
	rt.Admin("GET /api/admin/universities", masters.ListUniversities)
	rt.Admin("POST /api/admin/universities", masters.CreateUniversity)
	rt.Admin("GET /api/admin/universities/{id}", masters.UniversityDetail)
	rt.Admin("PATCH /api/admin/universities/{id}", masters.UpdateUniversity)
	rt.Admin("DELETE /api/admin/universities/{id}", masters.DeleteUniversity)
	rt.Admin("GET /api/admin/tags", masters.ListTags)
	rt.Admin("POST /api/admin/faculties", masters.CreateFaculty)
	rt.Admin("PATCH /api/admin/faculties/{id}", masters.UpdateFaculty)
	rt.Admin("DELETE /api/admin/faculties/{id}", masters.DeleteFaculty)
	rt.Admin("GET /api/admin/textbook-masters", masters.ListTextbookMasters)
	rt.Admin("POST /api/admin/textbook-masters", masters.CreateTextbookMaster)
	rt.Admin("PATCH /api/admin/textbook-masters/{id}", masters.UpdateTextbookMaster)
	rt.Admin("DELETE /api/admin/textbook-masters/{id}", masters.DeleteTextbookMaster)

	cron := notifications.NewCronHandler(db, jobs.messenger)
	rt.Job("POST /api/cron/daily-study-notifications", jobs.dailyNotificationSecret, cron.DailyNotifications)

	// シミュレーションの API は SIMULATION_ENABLED=on のときだけ存在する（付けなければ 404）。
	if jobs.simulationEnabled {
		simRoutes := sim.New(db)
		rt.Job("GET /api/sim/state", jobs.simulationSecret, simRoutes.State)
		rt.Job("POST /api/sim/users", jobs.simulationSecret, simRoutes.MarkUser)
		rt.Job("PATCH /api/sim/users/{seq}", jobs.simulationSecret, simRoutes.UpdateUser)
	}
}

// jobConfig はジョブ（cron・sim）の入口が使う設定。秘密の値と外部サービスへの送り方。
// cron と sim はトークンが別（片方が漏れても、もう片方は呼べない）。
type jobConfig struct {
	dailyNotificationSecret string
	simulationEnabled       bool
	simulationSecret        string
	messenger               notifications.Messenger
}
