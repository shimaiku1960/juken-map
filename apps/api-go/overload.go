package main

import (
	"log/slog"
	"net/http"
)

// defaultMaxInFlight は同時に処理するリクエストの上限。Node の DEFAULT_MAX_IN_FLIGHT と同じ値から始める。
// Node で決めた経緯（上限が無いと限界を超えた途端に全員が秒単位で遅くなる）は apps/api/src/overload.ts。
// Go の方が1件あたりの CPU が軽いので、本番に出すときに負荷試験で測り直す。
const defaultMaxInFlight = 160

// limitInFlight は、同時に処理中の数が上限に達していたら、待たせずに 503 で断る。
//
// 数え方は「容量 max のチャネル」。入るときに1つ書き込み、出るときに1つ読み出す。
// 満杯なら select の default に落ちるので、待たずに断れる（Go でよく使うセマフォの形）。
// Node は1スレッドなので整数を足し引きするだけで済んだが、Go は複数のリクエストが
// 本当に同時に走るため、ただの int だと数え間違える。チャネルなら同時に触っても安全。
//
// 死活監視とデプロイ後の確認が叩く /api/health は数えない。混んでいるだけで
// 「落ちている」と判定されると、正常なサーバーまで戻されてしまう。
func limitInFlight(max int, next http.Handler) http.Handler {
	slots := make(chan struct{}, max)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
			next.ServeHTTP(w, r)
		default:
			slog.WarnContext(r.Context(), "request shed: overloaded", "inFlight", len(slots), "maxInFlight", max)
			// すぐ再送されると混雑が続くので、少し待ってもらう。
			w.Header().Set("Retry-After", "1")
			writeErrorBody(w, r, http.StatusServiceUnavailable, codeOverloaded)
		}
	})
}
