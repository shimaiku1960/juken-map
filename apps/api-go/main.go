// api-go は、Node（apps/api）の業務 API を1本ずつ Go へ移すためのサーバー（JUK-70）。
// 最初の1本の GET /api/dashboard（JUK-69）に続けて、読み取りの API（JUK-73）と書き込みの API（JUK-75）を移している。
// 本番では nginx が移したパスだけを Go へ振り分ける（infra/nginx/juken-map-go-routes.conf、JUK-72）。
//
// LINE 連携も Go が受ける（JUK-79）。ログインの発行と管理画面は Node に残す。セッションは Node 側（Better Auth）が
// 発行したものを、同じ DB と同じ BETTER_AUTH_SECRET で確かめるだけ。
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// requestTimeout は1リクエストにかけてよい時間（middleware.go の withDeadline）。
// 下の WriteTimeout（応答を書き終えるまでの上限）より短くして、打ち切る前に 500 を返せるようにする。
const requestTimeout = 10 * time.Second

func main() {
	slog.SetDefault(newLogger(os.Stdout, parseLevel(os.Getenv("LOG_LEVEL"))))
	if err := run(); err != nil {
		slog.Error("api-go stopped", "err", err.Error())
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
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	auth := &sessionAuth{db: db, secret: []byte(secret)}
	rt := newRouter(auth.load)
	registerRoutes(rt, db, jobConfig{
		dailyNotificationSecret: os.Getenv("DAILY_NOTIFICATION_SECRET"),
		simulationEnabled:       os.Getenv("SIMULATION_ENABLED") == "on",
		simulationSecret:        os.Getenv("SIMULATION_SECRET"),
		messenger: &httpMessenger{
			client:     &http.Client{},
			resendBase: envOr("RESEND_BASE_URL", "https://api.resend.com"),
			resendKey:  os.Getenv("RESEND_API_KEY"),
			lineBase:   envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			lineToken:  os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
		},
	}, lineConfig{
		channelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		webOrigin:     envOr("WEB_ORIGIN", siteURL),
		client: &httpLineClient{
			client:         &http.Client{},
			botBase:        envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			accessToken:    os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
			loginBase:      envOr("LINE_LOGIN_API_BASE", "https://api.line.me"),
			authorizeURL:   "https://access.line.me/oauth2/v2.1/authorize",
			loginChannelID: os.Getenv("LINE_LOGIN_CHANNEL_ID"),
			loginSecret:    os.Getenv("LINE_LOGIN_CHANNEL_SECRET"),
		},
	})

	m := newMetrics()
	srv := &http.Server{
		Addr: ":" + envOr("PORT", "8080"),
		Handler: newServerHandler(rt, m, serverOptions{
			maxInFlight: maxInFlight,
			// 本番は reqId を UUID のまま、開発は短くする（Node と同じ）。
			shortRequestIDs: os.Getenv("NODE_ENV") != "production",
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
			slog.Info("api-go listening", "addr", s.Addr)
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
	slog.Info("api-go shutting down")
	var errs []error
	for _, s := range servers {
		errs = append(errs, s.Shutdown(shutdownCtx))
	}
	return errors.Join(errs...)
}

// registerRoutes は Go が受け持つルートを登録する。本番で Go へ届くのは、このうち
// infra/nginx/juken-map-go-routes.conf に書いたパスだけ。
// 一覧は main_test.go の TestRegisteredRoutes が入口の種類と一緒に確かめている。
func registerRoutes(rt *router, db *sql.DB, jobs jobConfig, line lineConfig) {
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

	goals := &goalHandlers{store: &goalStore{db: db}}
	rt.user("GET /api/goals", goals.list)
	rt.user("GET /api/goals/first-choice", goals.firstChoice)

	textbooks := &textbookHandlers{store: &textbookStore{db: db}}
	rt.user("GET /api/textbooks", textbooks.list)
	rt.user("GET /api/textbook-masters", textbooks.listMasters)

	prefs := &notificationPreferenceHandlers{store: &notificationPreferenceStore{db: db}}
	rt.user("GET /api/notification-preferences", prefs.get)
	rt.user("PUT /api/notification-preferences", prefs.save)

	profile := &profileHandlers{store: &userStore{db: db}}
	rt.user("PUT /api/profile", profile.update)

	universities := &universityHandlers{store: newUniversityStore(db)}
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
}

// newServerHandler はルーターの外側にミドルウェアを重ねる。外側から順に走る。
//
//  1. observe       reqId を振り、返し終えたらログ1行とメトリクス（断った応答も数える）
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
	return observe(m, opts.shortRequestIDs, h)
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
