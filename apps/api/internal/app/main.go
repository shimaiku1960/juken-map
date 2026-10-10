// Package app は juken-map のサーバーの組み立て。API・ログイン・画面の配信・運用のコマンド・マイグレーションの適用を
// 1つのバイナリで受け持つ。入口は cmd/api/main.go で、ここの Main を呼ぶだけ（JUK-156）。
// 本番では nginx が全部のパスをここへ送る（infra/nginx/juken-map-go-routes.conf）。
// ログイン（/api/auth/*）は internal/feature/auth にある。
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/fault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/auth"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/blog"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/chaos"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/line"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/notifications"
	"github.com/shimaiku1960/juken-map/apps/api/internal/hostfault"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/site"
	"github.com/shimaiku1960/juken-map/apps/api/internal/spa"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// Main は cmd/api の main から呼ばれる本体で、終了コードを返す。args は os.Args[1:]。
func Main(args []string) int {
	// 引数があれば、サーバーではなくコマンド（incident・grant-admin・chaos・migrate）として動く（cli.go）。
	if len(args) > 0 {
		return runCommand(args, os.Stdout, os.Stderr)
	}
	logOut, err := telemetry.LogOutput(os.Stdout, os.Getenv("LOG_FILE"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "LOG_FILE を開けません:", err)
		return 1
	}
	slog.SetDefault(telemetry.NewLogger(logOut, telemetry.ParseLevel(os.Getenv("LOG_LEVEL"))))
	if err := run(); err != nil {
		slog.Error("api stopped", "err", err.Error())
		return 1
	}
	return 0
}

// run は Main から切り出したサーバーの本体。エラーを返す形にしておくと、defer（DB を閉じるなど）が
// 必ず走ってから終了できる。os.Exit や log.Fatal を呼ぶと defer は走らない。
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

	m := auth.NewMetrics()
	// 障害注入（JUK-173）。CHAOS_ENABLED=on のときだけ作る。nil なら DB・外部 API・ルーターのどれにも差し込まない。
	var injector *fault.Injector
	if os.Getenv("CHAOS_ENABLED") == "on" {
		injector = fault.New()
	}
	db, err := database.Open(os.Getenv("DATABASE_URL"), injector.WrapConnector)
	if err != nil {
		return err
	}
	defer db.Close()
	// 外部 API のクライアントの土台。障害注入が無効なら http.DefaultTransport のまま。
	outbound := injector.Transport(http.DefaultTransport)

	hashConcurrency, err := envInt("AUTH_HASH_CONCURRENCY", auth.DefaultHashConcurrency)
	if err != nil {
		return err
	}
	// TOTP の秘密を暗号化する鍵（internal/feature/auth/totp.go）。AUTH_TOTP_KEYS が無ければ BETTER_AUTH_SECRET から導く。
	totpKeys, err := auth.NewTOTPKeyring(os.Getenv("AUTH_TOTP_KEYS"), secret)
	if err != nil {
		return err
	}
	m.ObserveDB(db)
	webOrigin := envOr("WEB_ORIGIN", site.URL)
	authHandlers := auth.New(db, auth.Config{
		WebOrigin:       webOrigin,
		TOTPKeys:        totpKeys,
		HashConcurrency: hashConcurrency,
		Metrics:         m,
		AdminTo:         os.Getenv("ADMIN_NOTIFICATION_EMAIL"),
		Sender: &auth.ResendSender{
			Client: telemetry.NewOutboundClient(tp, outbound),
			Base:   envOr("RESEND_BASE_URL", "https://api.resend.com"),
			Key:    os.Getenv("RESEND_API_KEY"),
		},
		OAuth: auth.NewOAuthProviders(webOrigin, auth.DefaultOAuthEndpoints,
			os.Getenv("AUTH_GOOGLE_ID"), os.Getenv("AUTH_GOOGLE_SECRET"),
			os.Getenv("AUTH_GITHUB_ID"), os.Getenv("AUTH_GITHUB_SECRET")),
	})
	rt := httpx.NewRouter(authHandlers.LoadSession)
	if injector != nil {
		rt.Faults = injector
	}
	auth.RegisterRoutes(rt, authHandlers)
	blog.RegisterRoutes(rt, blog.Config{
		ServiceDomain: os.Getenv("MICROCMS_SERVICE_DOMAIN"),
		APIKey:        os.Getenv("MICROCMS_API_KEY"),
		Client:        telemetry.NewOutboundClient(tp, outbound),
	})
	messenger := &notifications.HTTPMessenger{
		Client:     telemetry.NewOutboundClient(tp, outbound),
		ResendBase: envOr("RESEND_BASE_URL", "https://api.resend.com"),
		ResendKey:  os.Getenv("RESEND_API_KEY"),
		LineBase:   envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
		LineToken:  os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
	}
	registerRoutes(rt, db, jobConfig{
		dailyNotificationSecret: os.Getenv("DAILY_NOTIFICATION_SECRET"),
		simulationEnabled:       os.Getenv("SIMULATION_ENABLED") == "on",
		simulationSecret:        os.Getenv("SIMULATION_SECRET"),
		chaos:                   injector,
		chaosSecret:             os.Getenv("CHAOS_SECRET"),
		messenger:               messenger,
	}, line.Config{
		ChannelSecret: os.Getenv("LINE_CHANNEL_SECRET"),
		WebOrigin:     envOr("WEB_ORIGIN", site.URL),
		Client: &line.HTTPClient{
			HTTP:           telemetry.NewOutboundClient(tp, outbound),
			BotBase:        envOr("LINE_API_BASE", "https://api.line.me/v2/bot"),
			AccessToken:    os.Getenv("LINE_CHANNEL_ACCESS_TOKEN"),
			LoginBase:      envOr("LINE_LOGIN_API_BASE", "https://api.line.me"),
			AuthorizeURL:   "https://access.line.me/oauth2/v2.1/authorize",
			LoginChannelID: os.Getenv("LINE_LOGIN_CHANNEL_ID"),
			LoginSecret:    os.Getenv("LINE_LOGIN_CHANNEL_SECRET"),
		},
	}, blog.WebhookConfig{
		Secret: os.Getenv("MICROCMS_WEBHOOK_SECRET"),
		Deployer: &blog.GitHubWorkflowDispatcher{
			Client:   telemetry.NewOutboundClient(tp, outbound),
			APIBase:  envOr("GITHUB_API_BASE", "https://api.github.com"),
			Repo:     "shimaiku1960/juken-map",
			Workflow: "deploy.yml",
			Ref:      "main",
			Token:    os.Getenv("GITHUB_DEPLOY_TOKEN"),
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
			// 本番は reqId を UUID のまま、開発は短くする。
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
	// 実行中の実験を5秒ごとに DB から読み直す（internal/fault）。
	if injector != nil {
		go injector.Run(ctx, db)
	}
	// 予告なしの実験を平日の昼に起こす（internal/feature/chaos の schedule.go、JUK-176）。CHAOS_ENABLED=on も要る。
	if injector != nil && os.Getenv("CHAOS_SCHEDULE") == "on" {
		var notify func(ctx context.Context, subject, body string) error
		if to := os.Getenv("ADMIN_NOTIFICATION_EMAIL"); to != "" {
			notify = func(ctx context.Context, subject, body string) error {
				return messenger.SendAdminEmail(ctx, to, subject, body)
			}
		}
		// ホストの層の障害（internal/hostfault、JUK-174）は CHAOS_HOST=on のときだけ。EC2 の外（ローカル）では作れない。
		var host hostfault.Runner
		if os.Getenv("CHAOS_HOST") == "on" {
			ictx, cancel := context.WithTimeout(ctx, 5*time.Second)
			h, err := hostfault.NewSSM(ictx)
			cancel()
			if err != nil {
				slog.Warn("[schedule] Host runner is not available.", "err", err.Error())
			} else {
				host = h
			}
		}
		go chaos.NewScheduler(db, injector, rt.FaultRoutes, notify, host).Run(ctx)
	}

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
