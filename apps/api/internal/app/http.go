package app

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// healthHandler は死活監視とデプロイ後の確認が叩く。DB まで繋がるかを見る。
// 繋がらなければ 500 を返す。
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
			httpx.InternalError(w, r, err)
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "commit": commit})
	}
}
