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
)

// requestTimeout は1リクエストにかけてよい時間（middleware.go の withDeadline）。
// 下の WriteTimeout（応答を書き終えるまでの上限）より短くして、打ち切る前に 500 を返せるようにする。
const requestTimeout = 10 * time.Second

func main() {
	// 引数があれば、サーバーではなくコマンド（incident・grant-admin・migrate）として動く（cli.go）。
	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:], os.Stdout, os.Stderr))
	}
	logOut, err := logOutput(os.Stdout, os.Getenv("LOG_FILE"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "LOG_FILE を開けません:", err)
		os.Exit(1)
	}
	slog.SetDefault(newLogger(logOut, parseLevel(os.Getenv("LOG_LEVEL"))))
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
	// トレースは DB より先に用意する。openDB の SQL の計測が、ここで決めた送り先を使うため。
	tp, shutdownTracing, err := setupTracing(context.Background())
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

	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	hashConcurrency, err := envInt("AUTH_HASH_CONCURRENCY", defaultHashConcurrency)
	if err != nil {
		return err
	}
	// TOTP の秘密を暗号化する鍵（auth_totp.go）。AUTH_TOTP_KEYS が無ければ BETTER_AUTH_SECRET から導く。
	totpKeys, err := newTOTPKeyring(os.Getenv("AUTH_TOTP_KEYS"), secret)
	if err != nil {
		return err
	}
	m := newMetrics()
	webOrigin := envOr("WEB_ORIGIN", siteURL)
	authHandlers := newAuthHandlers(db, authConfig{
		webOrigin:       webOrigin,
		totpKeys:        totpKeys,
		hashConcurrency: hashConcurrency,
		metrics:         m,
		adminTo:         os.Getenv("ADMIN_NOTIFICATION_EMAIL"),
		sender: &resendSender{
			client: newOutboundClient(tp),
			base:   envOr("RESEND_BASE_URL", "https://api.resend.com"),
			key:    os.Getenv("RESEND_API_KEY"),
		},
		oauth: newOAuthProviders(webOrigin, defaultOAuthEndpoints,
			os.Getenv("AUTH_GOOGLE_ID"), os.Getenv("AUTH_GOOGLE_SECRET"),
			os.Getenv("AUTH_GITHUB_ID"), os.Getenv("AUTH_GITHUB_SECRET")),
	})
	rt := newRouter((&sessionAuth{store: authHandlers.sessions}).load)
	registerAuthRoutes(rt, authHandlers)
	registerBlogRoutes(rt, blogConfig{
		serviceDomain: os.Getenv("MICROCMS_SERVICE_DOMAIN"),
		apiKey:        os.Getenv("MICROCMS_API_KEY"),
		client:        newOutboundClient(tp),
	})
	registerRoutes(rt, db, jobConfig{
		dailyNotificationSecret: os.Getenv("DAILY_NOTIFICATION_SECRET"),
		simulationEnabled:       os.Getenv("SIMULATION_ENABLED") == "on",
		simulationSecret:        os.Getenv("SIMULATION_SECRET"),
		messenger: &httpMessenger{
			client:     newOutboundClient(tp),
			resendBase: envOr("RESEND_BASE_URL", "https://api.resend.com"),
			resendKey:  os.Getenv("RESEND_API_KEY"),
			lineBase:   envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			lineToken:  os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
		},
	}, lineConfig{
		channelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		webOrigin:     envOr("WEB_ORIGIN", siteURL),
		client: &httpLineClient{
			client:         newOutboundClient(tp),
			botBase:        envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			accessToken:    os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
			loginBase:      envOr("LINE_LOGIN_API_BASE", "https://api.line.me"),
			authorizeURL:   "https://access.line.me/oauth2/v2.1/authorize",
			loginChannelID: os.Getenv("LINE_LOGIN_CHANNEL_ID"),
			loginSecret:    os.Getenv("LINE_LOGIN_CHANNEL_SECRET"),
		},
	}, microcmsWebhookConfig{
		secret: os.Getenv("MICROCMS_WEBHOOK_SECRET"),
		deployer: &githubWorkflowDispatcher{
			client:   newOutboundClient(tp),
			apiBase:  envOr("GITHUB_API_BASE", "https://api.github.com"),
			repo:     "shimaiku1960/juken-map",
			workflow: "deploy.yml",
			ref:      "main",
			token:    os.Getenv("GITHUB_DEPLOY_TOKEN"),
		},
	})

	// 画面（apps/web のビルド成果物）を配る（JUK-111）。WEB_DIST_DIR が無ければ配らない（開発は Vite が配る）。
	// GA4 と Faro の設定は画面のバンドルに焼き込まず、ここで HTML に差し込む。
	site, err := loadSPA(os.Getenv("WEB_DIST_DIR"), pageScripts{
		gaMeasurementID:  os.Getenv("GA_MEASUREMENT_ID"),
		faroCollectorURL: os.Getenv("FARO_COLLECTOR_URL"),
	})
	if err != nil {
		return err
	}
	if site != nil {
		registerSPA(rt, site)
		slog.Info("api serving web", "files", len(site.assets), "prerendered", len(site.pages))
	}

	srv := &http.Server{
		Addr: ":" + envOr("PORT", "8080"),
		Handler: newServerHandler(rt, m, serverOptions{
			maxInFlight: maxInFlight,
			// 本番は reqId を UUID のまま、開発は短くする（Node と同じ）。
			shortRequestIDs: os.Getenv("NODE_ENV") != "production",
			tracer:          tp.Tracer(serviceName),
		}),
		// 既定はどれも無制限。遅いクライアントに接続を握られ続けないよう上限を付ける。
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	servers := []*http.Server{srv}

	// /metrics はアプリと別のポートで出す（metrics.go）。指定したときだけ起動する。
	if port := os.Getenv("METRICS_PORT"); port != "" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", m.handler())
		servers = append(servers, &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second})
	}

	// Ctrl+C（SIGINT）や SIGTERM で ctx が取り消される。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

// registerAuthRoutes はログインの入口（auth_handlers.go の一覧）を登録する。本番では /api/auth/ で始まるものを
// すべて Go へ送る（infra/nginx/juken-map-go-routes.conf）。
func registerAuthRoutes(rt *router, h *authHandlers) {
	rt.auth("GET /api/auth/session", h.session)
	rt.auth("POST /api/auth/sign-up", h.signUp)
	rt.auth("POST /api/auth/sign-in", h.signIn)
	rt.auth("POST /api/auth/sign-out", h.signOut)
	rt.auth("POST /api/auth/verify-email", h.verifyEmail)
	rt.auth("POST /api/auth/verify-email/resend", h.resendVerification)
	rt.auth("POST /api/auth/password/forgot", h.forgotPassword)
	rt.auth("POST /api/auth/password/reset", h.resetPassword)
	rt.auth("POST /api/auth/password/change", h.changePassword)
	rt.auth("GET /api/auth/accounts", h.accounts)
	rt.auth("POST /api/auth/delete-account", h.deleteAccount)
	rt.auth("POST /api/auth/mfa/setup", h.mfaSetup)
	rt.auth("POST /api/auth/mfa/confirm", h.mfaConfirm)
	rt.auth("POST /api/auth/mfa/verify", h.mfaVerify)
	rt.auth("POST /api/auth/oauth/{provider}", h.oauthStart)
	rt.auth("GET /api/auth/callback/{provider}", h.oauthCallback)
}

// registerBlogRoutes はブログの記事の中継（blog.go）を登録する。
func registerBlogRoutes(rt *router, c blogConfig) {
	blog := newBlogHandlers(c)
	rt.public("GET /api/blog", blog.list)
	rt.public("GET /api/blog/{id}", blog.detail)
}

// registerRoutes は Go が受け持つルートを登録する。本番で Go へ届くのは、このうち
// infra/nginx/juken-map-go-routes.conf に書いたパスだけ。
// 一覧は main_test.go の TestRegisteredRoutes が入口の種類と一緒に確かめている。
func registerRoutes(rt *router, db *sql.DB, jobs jobConfig, line lineConfig, microcms microcmsWebhookConfig) {
	study := &studyStore{db: db}
	studyHandlers := &studyHandlers{store: study}

	rt.public("GET /api/health", healthHandler(db))
	rt.user("GET /api/dashboard", (&dashboardHandler{store: study}).serve)
	rt.user("GET /api/study-logs", studyHandlers.listLogs)
	rt.user("GET /api/study-logs/daily", studyHandlers.listDaily)
	rt.user("GET /api/study-plans", studyHandlers.listPlans)

	studyLogWrites := &studyLogWriteHandlers{store: &studyLogWriteStore{db: db}, now: time.Now}
	rt.user("POST /api/study-logs", studyLogWrites.create)
	rt.user("PATCH /api/study-logs/{id}", studyLogWrites.update)
	rt.user("DELETE /api/study-logs/{id}", studyLogWrites.delete)

	studyPlanWrites := &studyPlanWriteHandlers{store: &studyPlanWriteStore{db: db}}
	rt.user("POST /api/study-plans", studyPlanWrites.create)
	rt.user("PATCH /api/study-plans/{id}", studyPlanWrites.update)
	rt.user("DELETE /api/study-plans/{id}", studyPlanWrites.delete)
	rt.user("POST /api/study-plans/{id}/complete", studyPlanWrites.complete)

	goals := &goalHandlers{store: &goalStore{db: db}}
	rt.user("GET /api/goals", goals.list)
	rt.user("GET /api/goals/first-choice", goals.firstChoice)
	rt.user("POST /api/goals", goals.create)
	rt.user("PUT /api/goals/{id}", goals.replace)
	rt.user("PATCH /api/goals/{id}", goals.update)
	rt.user("DELETE /api/goals/{id}", goals.delete)

	textbooks := &textbookHandlers{store: &textbookStore{db: db}}
	rt.user("GET /api/textbooks", textbooks.list)
	rt.user("GET /api/textbook-masters", textbooks.listMasters)
	rt.user("POST /api/textbooks", textbooks.create)
	rt.user("PATCH /api/textbooks/{id}", textbooks.updateProgress)

	prefs := &notificationPreferenceHandlers{store: &notificationPreferenceStore{db: db}}
	rt.user("GET /api/notification-preferences", prefs.get)
	rt.user("PUT /api/notification-preferences", prefs.save)

	profile := &profileHandlers{store: &userStore{db: db}}
	rt.user("PUT /api/profile", profile.update)

	universityStore := newUniversityStore(db)
	universities := &universityHandlers{store: universityStore}
	rt.user("GET /api/universities", universities.list)
	rt.user("GET /api/universities/{id}", universities.detail)

	analytics := &analyticsHandlers{store: &analyticsStore{db: db}}
	rt.user("POST /api/analytics/registration", analytics.registration)
	rt.anonymousWrite("POST /api/csp-report", cspReport)

	lineRoutes := &lineHandlers{store: &sqlLineStore{db: db}, line: line.client, channelSecret: line.channelSecret, webOrigin: line.webOrigin}
	rt.user("GET /api/line/connection", lineRoutes.connection)
	rt.user("DELETE /api/line/connection", lineRoutes.disconnect)
	rt.user("POST /api/line/account-link", lineRoutes.accountLink)
	rt.oauth("GET /api/line/oauth/start", lineRoutes.oauthStart)
	rt.oauth("GET /api/line/oauth/callback", lineRoutes.oauthCallback)
	rt.webhook("POST /api/line/webhook", lineRoutes.webhook)
	rt.publicWithSession("GET /line/settings", lineRoutes.settings)

	// microCMS で記事を変えたら、記事を作り直すデプロイを動かす（JUK-112）。
	microcmsWebhook := &microcmsWebhookHandler{secret: microcms.secret, trigger: newDeployTrigger(microcms.deployer)}
	rt.webhook("POST /api/webhooks/microcms", microcmsWebhook.serve)

	adminUsers := &adminUserHandlers{store: &sqlAdminUserStore{db: db}, now: time.Now}
	rt.admin("GET /api/admin/overview", adminUsers.overview)
	rt.admin("GET /api/admin/users", adminUsers.listUsers)
	rt.admin("POST /api/admin/users/{id}/ban", adminUsers.ban)
	rt.admin("POST /api/admin/users/{id}/unban", adminUsers.unban)
	rt.admin("DELETE /api/admin/users/{id}", adminUsers.deleteUser)

	// マスター編集。大学・学部を変えたら、大学を探す画面の一覧（上の universities）のキャッシュを捨てる。
	masters := &adminMasterHandlers{store: &sqlAdminMasterStore{db: db, universitiesChanged: universityStore.invalidate}}
	rt.admin("GET /api/admin/universities", masters.listUniversities)
	rt.admin("POST /api/admin/universities", masters.createUniversity)
	rt.admin("GET /api/admin/universities/{id}", masters.universityDetail)
	rt.admin("PATCH /api/admin/universities/{id}", masters.updateUniversity)
	rt.admin("DELETE /api/admin/universities/{id}", masters.deleteUniversity)
	rt.admin("GET /api/admin/tags", masters.listTags)
	rt.admin("POST /api/admin/faculties", masters.createFaculty)
	rt.admin("PATCH /api/admin/faculties/{id}", masters.updateFaculty)
	rt.admin("DELETE /api/admin/faculties/{id}", masters.deleteFaculty)
	rt.admin("GET /api/admin/textbook-masters", masters.listTextbookMasters)
	rt.admin("POST /api/admin/textbook-masters", masters.createTextbookMaster)
	rt.admin("PATCH /api/admin/textbook-masters/{id}", masters.updateTextbookMaster)
	rt.admin("DELETE /api/admin/textbook-masters/{id}", masters.deleteTextbookMaster)

	cron := &cronHandler{notifier: newDailyNotifier(&sqlNotificationStore{db: db}, jobs.messenger), now: time.Now}
	rt.job("POST /api/cron/daily-study-notifications", jobs.dailyNotificationSecret, cron.dailyNotifications)

	// シミュレーションの API は SIMULATION_ENABLED=on のときだけ存在する（付けなければ 404）。
	if jobs.simulationEnabled {
		sim := &simHandlers{store: &simStore{db: db}}
		rt.job("GET /api/sim/state", jobs.simulationSecret, sim.state)
		rt.job("POST /api/sim/users", jobs.simulationSecret, sim.markUser)
		rt.job("PATCH /api/sim/users/{seq}", jobs.simulationSecret, sim.updateUser)
	}
}

// jobConfig はジョブ（cron・sim）の入口が使う設定。秘密の値と外部サービスへの送り方。
// cron と sim はトークンが別（片方が漏れても、もう片方は呼べない）。
type jobConfig struct {
	dailyNotificationSecret string
	simulationEnabled       bool
	simulationSecret        string
	messenger               messenger
}

// lineConfig は LINE 連携（line.go）の設定。
type lineConfig struct {
	channelSecret string // Webhook の署名を確かめる。空なら Webhook は必ず 401
	webOrigin     string // 画面のオリジン。OAuth の戻り先とリダイレクト先に使う
	client        lineClient
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
//  6. ルーター       入口の種類ごとの拒否（router.go）→ ハンドラ
//
// 順番は Node の server.ts と同じ考え方（メトリクス → エラー処理 → 過負荷 → 認証）。
func newServerHandler(rt *router, m *metrics, opts serverOptions) http.Handler {
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
