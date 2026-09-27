// api-go は、Node（apps/api）の GET /api/dashboard と同じ応答を Go で返す、比較実験用のサーバー。
// 本番には出さず、手元で Node と同じ負荷をかけて 1リクエストあたりの CPU 時間を比べるためだけにある（JUK-69）。
//
// 置き換えが目的ではないので、ログインの発行や書き込みは持たない。セッションは Node 側
// （Better Auth）が発行したものを、同じ DB と同じ BETTER_AUTH_SECRET で確かめるだけ。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Node 側（pino）と同じく、1行1つの JSON で標準出力へ書く。
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("api-go stopped", "err", err)
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
	db, err := openDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer db.Close()

	auth := &sessionAuth{db: db, secret: []byte(secret)}
	dashboard := &dashboardHandler{db: db}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", healthHandler(db))
	mux.Handle("GET /api/dashboard", auth.requireUser(dashboard.serve))

	srv := &http.Server{
		Addr:    ":" + envOr("PORT", "8080"),
		Handler: accessLog(mux),
		// 既定はどれも無制限。遅いクライアントに接続を握られ続けないよう上限を付ける。
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Ctrl+C（SIGINT）や SIGTERM で ctx が取り消される。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// ListenAndServe は止まるまで戻らないので、別の goroutine で動かし、
	// 「サーバーが落ちた」と「止めるよう言われた」のどちらか早いほうを待つ。
	serveErr := make(chan error, 1)
	go func() {
		slog.Info("api-go listening", "addr", srv.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	// 受け付け済みのリクエストは返し終えてから止める（新しい接続はもう受けない）。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	slog.Info("api-go shutting down")
	return srv.Shutdown(shutdownCtx)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
