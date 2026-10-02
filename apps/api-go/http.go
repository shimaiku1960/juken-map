package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
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

// healthHandler は死活監視とデプロイ後の確認が叩く。DB まで繋がるかを見る。
// Node と同じく、繋がらなければ 500 を返す（Node は SELECT 1 が throw して 500 になる）。
//
// commit はイメージを作ったコミット（Dockerfile が APP_COMMIT に埋め込む）。デプロイ後の E2E が、
// 本番で新しい版が動いているかを見る（JUK-101）。埋め込まずに動かしたときは null。
func healthHandler(db *sql.DB) http.HandlerFunc {
	var commit *string
	if c := os.Getenv("APP_COMMIT"); c != "" {
		commit = &c
	}
	return func(w http.ResponseWriter, r *http.Request) {
		// r.Context() はクライアントが切断すると取り消される。そこに上限時間を足す。
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			internalError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "commit": commit})
	}
}
