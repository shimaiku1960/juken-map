package app

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/telemetry"
)

// ルーターの外側に重ねるミドルウェアの順番と、1リクエストの時間の上限。ミドルウェアの中身は middleware.go・overload.go。

// requestTimeout は1リクエストにかけてよい時間（middleware.go の withDeadline）。
// main.go の run の WriteTimeout（応答を書き終えるまでの上限）より短くして、打ち切る前に 500 を返せるようにする。
const requestTimeout = 10 * time.Second

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
