// juken-map のサーバー。API・ログイン・画面の配信・運用のコマンド・マイグレーションの適用を受け持つ。
// もとは Node の業務 API を1本ずつ Go へ移すために作った（JUK-70）。移し終えて Node を消したあと、
// apps/api-go から apps/api へ名前を変えた（JUK-131）。本番では nginx が全部のパスを Go へ送る（infra/nginx/juken-map-go-routes.conf）。
//
// LINE 連携（JUK-79）と管理画面の API（JUK-78）も Go が受ける。ログイン（/api/auth/*）も Better Auth から移し、
// Go で自作した（JUK-115、auth_*.go）。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/admin"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/auth"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/goals"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/line"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/notifications"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/study"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/textbooks"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/site"
	"github.com/shimaiku1960/juken-map/apps/api/internal/spa"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// requestTimeout は1リクエストにかけてよい時間（middleware.go の withDeadline）。
// 下の WriteTimeout（応答を書き終えるまでの上限）より短くして、打ち切る前に 500 を返せるようにする。
const requestTimeout = 10 * time.Second

func main() {
	// 引数があれば、サーバーではなくコマンド（incident・grant-admin・migrate）として動く（cli.go）。
	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:], os.Stdout, os.Stderr))
	}
	logOut, err := telemetry.LogOutput(os.Stdout, os.Getenv("LOG_FILE"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "LOG_FILE を開けません:", err)
		os.Exit(1)
	}
	slog.SetDefault(telemetry.NewLogger(logOut, telemetry.ParseLevel(os.Getenv("LOG_LEVEL"))))
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err.Error())
		os.Exit(1)
	}
}

// run は main から切り出した本体。エラーを返す形にしておくと、defer（DB を閉じるなど）が
// 必ず走ってから終了できる。main で os.Exit や log.Fatal を呼ぶと defer は走らない。
func run() error {
	secret := os.Getenv("BETTER_AUTH_SECRET")
	if secret == "" {
		return errors.New("BETTER_AUTH_SECRET が空です")
	}
	maxInFlight, err := envInt("OVERLOAD_MAX_IN_FLIGHT", defaultMaxInFlight)
	if err != nil {
		return err
	}
	// トレースは DB より先に用意する。database.Open の SQL の計測が、ここで決めた送り先を使うため。
	tp, shutdownTracing, err := telemetry.SetupTracing(context.Background())
	if err != nil {
		return err
	}
	// 止めるときに、まだ送っていないスパンを送り切る。DB を閉じた後（defer は逆順に走る）。
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(ctx); err != nil {
			slog.Warn("tracing shutdown failed", "err", err.Error())
		}
	}()

	db, err := database.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	hashConcurrency, err := envInt("AUTH_HASH_CONCURRENCY", auth.DefaultHashConcurrency)
	if err != nil {
		return err
	}
	// TOTP の秘密を暗号化する鍵（internal/feature/auth/totp.go）。AUTH_TOTP_KEYS が無ければ BETTER_AUTH_SECRET から導く。
	totpKeys, err := auth.NewTOTPKeyring(os.Getenv("AUTH_TOTP_KEYS"), secret)
	if err != nil {
		return err
	}
	m := auth.NewMetrics()
	webOrigin := envOr("WEB_ORIGIN", site.URL)
	authHandlers := auth.New(db, auth.Config{
		WebOrigin:       webOrigin,
		TOTPKeys:        totpKeys,
		HashConcurrency: hashConcurrency,
		Metrics:         m,
		AdminTo:         os.Getenv("ADMIN_NOTIFICATION_EMAIL"),
		Sender: &auth.ResendSender{
			Client: telemetry.NewOutboundClient(tp),
			Base:   envOr("RESEND_BASE_URL", "https://api.resend.com"),
			Key:    os.Getenv("RESEND_API_KEY"),
		},
		OAuth: auth.NewOAuthProviders(webOrigin, auth.DefaultOAuthEndpoints,
			os.Getenv("AUTH_GOOGLE_ID"), os.Getenv("AUTH_GOOGLE_SECRET"),
			os.Getenv("AUTH_GITHUB_ID"), os.Getenv("AUTH_GITHUB_SECRET")),
	})
	rt := httpx.NewRouter(authHandlers.LoadSession)
	auth.RegisterRoutes(rt, authHandlers)
	registerBlogRoutes(rt, blogConfig{
		serviceDomain: os.Getenv("MICROCMS_SERVICE_DOMAIN"),
		apiKey:        os.Getenv("MICROCMS_API_KEY"),
		client:        telemetry.NewOutboundClient(tp),
	})
	registerRoutes(rt, db, jobConfig{
		dailyNotificationSecret: os.Getenv("DAILY_NOTIFICATION_SECRET"),
		simulationEnabled:       os.Getenv("SIMULATION_ENABLED") == "on",
		simulationSecret:        os.Getenv("SIMULATION_SECRET"),
		messenger: &notifications.HTTPMessenger{
			Client:     telemetry.NewOutboundClient(tp),
			ResendBase: envOr("RESEND_BASE_URL", "https://api.resend.com"),
			ResendKey:  os.Getenv("RESEND_API_KEY"),
			LineBase:   envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			LineToken:  os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
		},
	}, line.Config{
		ChannelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		WebOrigin:     envOr("WEB_ORIGIN", site.URL),
		Client: &line.HTTPClient{
			HTTP:           telemetry.NewOutboundClient(tp),
			BotBase:        envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			AccessToken:    os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
			LoginBase:      envOr("LINE_LOGIN_API_BASE", "https://api.line.me"),
			AuthorizeURL:   "https://access.line.me/oauth2/v2.1/authorize",
			LoginChannelID: os.Getenv("LINE_LOGIN_CHANNEL_ID"),
			LoginSecret:    os.Getenv("LINE_LOGIN_CHANNEL_SECRET"),
		},
	}, microcmsWebhookConfig{
		secret: os.Getenv("MICROCMS_WEBHOOK_SECRET"),
		deployer: &githubWorkflowDispatcher{
			client:   telemetry.NewOutboundClient(tp),
			apiBase:  envOr("GITHUB_API_BASE", "https://api.github.com"),
			repo:     "shimaiku1960/juken-map",
			workflow: "deploy.yml",
			ref:      "main",
			token:    os.Getenv("GITHUB_DEPLOY_TOKEN"),
		},
	})

	// 画面（apps/web のビルド成果物）を配る（JUK-111）。WEB_DIST_DIR が無ければ配らない（開発は Vite が配る）。
	// GA4 と Faro の設定は画面のバンドルに焼き込まず、ここで HTML に差し込む。
	web, err := spa.Load(os.Getenv("WEB_DIST_DIR"), spa.Scripts{
		GAMeasurementID:  os.Getenv("GA_MEASUREMENT_ID"),
		FaroCollectorURL: os.Getenv("FARO_COLLECTOR_URL"),
	})
	if err != nil {
		return err
	}
	if web != nil {
		spa.Register(rt, web)
		files, prerendered := web.Stats()
		slog.Info("api serving web", "files", files, "prerendered", prerendered)
	}

	srv := &http.Server{
		Addr: ":" + envOr("PORT", "8080"),
		Handler: newServerHandler(rt, m, serverOptions{
			maxInFlight: maxInFlight,
			// 本番は reqId を UUID のまま、開発は短くする（Node と同じ）。
			shortRequestIDs: os.Getenv("NODE_ENV") != "production",
			tracer:          tp.Tracer(telemetry.ServiceName),
		}),
		// 既定はどれも無制限。遅いクライアントに接続を握られ続けないよう上限を付ける。
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	servers := []*http.Server{srv}

	// /metrics はアプリと別のポートで出す（internal/telemetry/metrics.go）。指定したときだけ起動する。
	if port := os.Getenv("METRICS_PORT"); port != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", m.Handler())
		servers = append(servers, &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second})
	}

	// Ctrl+C（SIGINT）や SIGTERM で ctx が取り消される。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 期限の切れたログイン・LINE 連携の行を1時間ごとに消す（expired_cleanup.go）。
	go runExpiredCleanup(ctx, db, time.Now)

	// ListenAndServe は止まるまで戻らないので、別の goroutine で動かし、
	// 「サーバーが落ちた」と「止めるよう言われた」のどちらか早いほうを待つ。
	serveErr := make(chan error, len(servers))
	for _, s := range servers {
		go func() {
			slog.Info("api listening", "addr", s.Addr)
			serveErr <- s.ListenAndServe()
		}()
	}

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// 受け付け済みのリクエストは返し終えてから止める（新しい接続はもう受けない）。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	slog.Info("api shutting down")
	var errs []error
	for _, s := range servers {
		errs = append(errs, s.Shutdown(shutdownCtx))
	}
	return errors.Join(errs...)
}

// registerBlogRoutes はブログの記事の中継（blog.go）を登録する。
func registerBlogRoutes(rt *httpx.Router, c blogConfig) {
	blog := newBlogHandlers(c)
	rt.Public("GET /api/blog", blog.list)
	rt.Public("GET /api/blog/{id}", blog.detail)
}

// registerRoutes は Go が受け持つルートを登録する。本番で Go へ届くのは、このうち
// infra/nginx/juken-map-go-routes.conf に書いたパスだけ。
// 一覧は main_test.go の TestRegisteredRoutes が入口の種類と一緒に確かめている。
func registerRoutes(rt *httpx.Router, db *sql.DB, jobs jobConfig, lineCfg line.Config, microcms microcmsWebhookConfig) {
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

	universityStore := newUniversityStore(db)
	universities := &universityHandlers{store: universityStore}
	rt.User("GET /api/universities", universities.list)
	rt.User("GET /api/universities/{id}", universities.detail)

	analytics := &analyticsHandlers{store: &analyticsStore{db: db}}
	rt.User("POST /api/analytics/registration", analytics.registration)
	rt.AnonymousWrite("POST /api/csp-report", cspReport)

	lineRoutes := line.New(db, lineCfg)
	rt.User("GET /api/line/connection", lineRoutes.Connection)
	rt.User("DELETE /api/line/connection", lineRoutes.Disconnect)
	rt.User("POST /api/line/account-link", lineRoutes.AccountLink)
	rt.OAuth("GET /api/line/oauth/start", lineRoutes.OauthStart)
	rt.OAuth("GET /api/line/oauth/callback", lineRoutes.OauthCallback)
	rt.Webhook("POST /api/line/webhook", lineRoutes.Webhook)
	rt.PublicWithSession("GET /line/settings", lineRoutes.Settings)

	// microCMS で記事を変えたら、記事を作り直すデプロイを動かす（JUK-112）。
	microcmsWebhook := &microcmsWebhookHandler{secret: microcms.secret, trigger: newDeployTrigger(microcms.deployer)}
	rt.Webhook("POST /api/webhooks/microcms", microcmsWebhook.serve)

	adminUsers := admin.NewUserHandlers(db)
	rt.Admin("GET /api/admin/overview", adminUsers.Overview)
	rt.Admin("GET /api/admin/users", adminUsers.ListUsers)
	rt.Admin("POST /api/admin/users/{id}/ban", adminUsers.Ban)
	rt.Admin("POST /api/admin/users/{id}/unban", adminUsers.Unban)
	rt.Admin("DELETE /api/admin/users/{id}", adminUsers.DeleteUser)

	// マスター編集。大学・学部を変えたら大学を探す画面の一覧（上の universities）の、参考書マスターを
	// 変えたら GET /api/textbook-masters（上の textbookRoutes）のキャッシュを捨てる。
	masters := admin.NewMasterHandlers(db, universityStore.explore.Invalidate, textbookRoutes.InvalidateMasters)
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
		sim := &simHandlers{store: &simStore{db: db}}
		rt.Job("GET /api/sim/state", jobs.simulationSecret, sim.state)
		rt.Job("POST /api/sim/users", jobs.simulationSecret, sim.markUser)
		rt.Job("PATCH /api/sim/users/{seq}", jobs.simulationSecret, sim.updateUser)
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

type serverOptions struct {
	maxInFlight     int
	shortRequestIDs bool
	tracer          trace.Tracer // 無ければトレースを取らない
}

// newServerHandler はルーターの外側にミドルウェアを重ねる。外側から順に走る。
//
//  1. observe       reqId を振り、返し終えたらログ1行とメトリクスとトレース（断った応答も数える）
//  2. securityHeaders  どの応答にも付ける
//  3. recoverPanic  ハンドラの panic を 500 にする
//  4. limitInFlight 同時処理数の上限を超えたら 503
//  5. withDeadline  1リクエストの時間の上限
//  6. ルーター       入口の種類ごとの拒否（internal/httpx/router.go）→ ハンドラ
//
// 順番は Node の server.ts と同じ考え方（メトリクス → エラー処理 → 過負荷 → 認証）。
func newServerHandler(rt *httpx.Router, m *telemetry.Metrics, opts serverOptions) http.Handler {
	var h http.Handler = rt
	h = withDeadline(requestTimeout, h)
	h = limitInFlight(opts.maxInFlight, h)
	h = recoverPanic(h)
	h = securityHeaders(h)
	tracer := opts.tracer
	if tracer == nil {
		tracer = noop.NewTracerProvider().Tracer("")
	}
	return observe(m, tracer, opts.shortRequestIDs, h)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, errors.New(key + " は正の整数で指定してください")
	}
	return n, nil
}
