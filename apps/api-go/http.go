package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// writeJSON は値を JSON にして返す。Fastify がハンドラの戻り値を JSON にしている部分にあたる。
// Go ではハンドラが値を返すのではなく、ResponseWriter に自分で書き込む。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// ヘッダー → ステータス → 本文の順。本文を書き始めた後のヘッダー変更は効かない。
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	// 既定では < > & を < などに書き換える（HTML に埋め込む場合の備え）。
	// Node の JSON.stringify は書き換えないので、応答を揃えるために止める。
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		// ステータスはもう送ってしまったので、ここではログに残すことしかできない。
		slog.Error("write json", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func healthHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// r.Context() はクライアントが切断すると取り消される。そこに上限時間を足す。
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			slog.Error("health", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]bool{"ok": false})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// statusRecorder は、ハンドラが書いたステータスを後から読めるようにする。
// ResponseWriter は書いたステータスを教えてくれないので、包んで横取りする。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// accessLog はリクエストごとに完了時のログを1行出す。Node 側の「request completed」
// （apps/api/src/observability/logger.ts）と同じく1リクエスト1行にして、比べる条件を揃える。
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request completed",
			"method", r.Method,
			"url", r.URL.Path,
			"status", rec.status,
			"responseTime", float64(time.Since(start).Microseconds())/1000,
		)
	})
}
