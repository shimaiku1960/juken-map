package main

import (
	"context"
	"database/sql/driver"
	"log/slog"
	"net/http"
	"os"

	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// OpenTelemetry のトレース。1回のリクエストの中で、どこに何 ms かかったか（リクエスト全体・SQL 1本ずつ・
// 外部 API の呼び出し）を記録し、OTLP/HTTP で送る（手元は Tempo、本番は Alloy → Grafana Cloud）。
// Node の instrumentation.ts にあたる（JUK-126）。
//
// Node は import を横取りしてライブラリに計測を仕込んだが、Go にその仕組みは無いので、
// 計測する場所を自分で包む。リクエストは observe（middleware.go）、SQL は openDB の otelsql、
// 外部 API は newOutboundClient。
//
// トークンを残さないため、スパンには URL のパスも ? 以降も入れない。リクエストはルートの型
// （/api/x/:id）で名付け、外部 API は送り先のホスト名だけを入れる。Node は URL をそのまま入れる
// ライブラリの計測を使ったので、送る直前に取り除いていた（redact.ts）。

// serviceName は Tempo で探すときの名前。Node と同じにして、Grafana のダッシュボードの絞り込み
// （resource.service.name）をそのまま使う。
const serviceName = "juken-map-api"

// setupTracing は OTEL_EXPORTER_OTLP_ENDPOINT を設定したときだけトレースを送る準備をする。
// 返す関数は、溜めたスパンを送り切ってから止める（サーバーを止めるときに呼ぶ）。
//
// 送り先（+ /v1/traces）や間引き（OTEL_TRACES_SAMPLER）は、SDK が OTEL_* の環境変数を読んで決める。
// 設定しなければ何も送らず、スパンも作らない（noop）。
func setupTracing(ctx context.Context) (trace.TracerProvider, func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		// まとめて送る。送り先が落ちていてもリクエストは待たされない（溢れた分は捨てる）。
		sdktrace.WithSpanProcessor(skipWebSpans{sdktrace.NewBatchSpanProcessor(exporter)}),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
	)
	// openDB の otelsql は、指定が無ければこの全体の設定を使う。
	otel.SetTracerProvider(tp)
	// 送れなかったとき（Alloy の入れ替え中など）の誤りは、既定だと JSON でない素の文で標準エラーに出る。
	// ほかのログと同じ形にして、Loki で拾えるようにする。
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("otel error", "err", err.Error())
	}))
	return tp, tp.Shutdown, nil
}

// tracedDBOptions は SQL のスパンの出し方。1本の照会につき1つのスパンにして、SQL の文（? のまま。
// 値はドライバが後で埋めるので入らない）を属性に残す。
var tracedDBOptions = otelsql.WithSpanOptions(otelsql.SpanOptions{
	// 結果を読む時間・接続の使い回しの準備は、照会のスパンと別に出すと数が倍になる割に読むことが無い。
	OmitRows:             true,
	OmitConnResetSession: true,
	OmitConnPrepare:      true,
	// リクエストの外（起動時の確認など）の SQL は、親の無いスパンになって一覧を埋めるので出さない。
	SpanFilter: func(ctx context.Context, _ otelsql.Method, _ string, _ []driver.NamedValue) bool {
		return trace.SpanContextFromContext(ctx).IsValid()
	},
})

// webRoute は API 以外（画面の HTML・JS・CSS・画像）をまとめたルート（spa.go）。
const webRoute = "(web)"

// tracedRoute は、そのルートのトレースを送るか。画面のファイル配信は SQL も外部 API も呼ばず、
// 1枚のスパンで終わるので見るものが無い。画面を1回開くだけで十数件になり、Tempo の一覧を埋める
// （本番で、デプロイ後の168件のうち135件がこれだった）。
func tracedRoute(route string) bool {
	return route != webRoute
}

// skipWebSpans は、送らないルートのスパンを送る前に捨てる。ルートは返し終えるまで決まらないので、
// スパンを作らずに済ませることはできない（間引き＝Sampler は始める時点で決める）。
type skipWebSpans struct{ sdktrace.SpanProcessor }

func (p skipWebSpans) OnEnd(s sdktrace.ReadOnlySpan) {
	for _, kv := range s.Attributes() {
		if kv.Key == "http.route" && !tracedRoute(kv.Value.AsString()) {
			return
		}
	}
	p.SpanProcessor.OnEnd(s)
}

// startRequestSpan は1リクエスト全体のスパンを始める。名前はルートが決まってから付け直す（endRequestSpan）。
//
// 外から来た traceparent ヘッダーは読まず（propagator を設定していない）、毎回ここを根にする。
// 外の誰かが決めた ID でトレースを作らせないため（この API を呼ぶのは自分の画面と外部サービスで、
// どちらもトレースを渡してこない）。
func startRequestSpan(ctx context.Context, tracer trace.Tracer, method string) (context.Context, trace.Span) {
	return tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("http.request.method", method)))
}

// endRequestSpan はルートの型で名付け直し、結果を書いて閉じる。
// 名前は Node（Fastify の計測）と同じ「GET /api/x/:id」にする。ダッシュボードが
// 「GET /api/health」の根を除く絞り込みを持っているため。
func endRequestSpan(span trace.Span, method, route string, status int, info *requestInfo) {
	span.SetName(method + " " + route)
	span.SetAttributes(
		attribute.String("http.route", route),
		attribute.Int("http.response.status_code", status),
		attribute.String("reqId", info.id),
	)
	if info.sim {
		span.SetAttributes(attribute.Bool("sim", true))
	}
	// OpenTelemetry の決まりで、サーバーの 4xx は呼び出し側の誤りなのでエラーにしない。
	if status >= 500 {
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	span.End()
}

// newOutboundClient は外部 API（Resend・LINE・microCMS・GitHub）を呼ぶクライアント。
// 呼び出し1回ごとにスパンを作る。tracer は呼び出し元（ctx）のスパンの子にするために使う。
func newOutboundClient(tp trace.TracerProvider) *http.Client {
	return &http.Client{Transport: &tracedTransport{
		base:   http.DefaultTransport,
		tracer: tp.Tracer(serviceName),
	}}
}

// tracedTransport は http.RoundTripper を包み、送り先のホスト名・メソッド・ステータスだけを残す。
// パスや ? 以降にはトークンが載ることがあり（LINE の連携など）、ヘッダーには鍵が載るので入れない。
// traceparent も送らない（外部サービスへ自分のトレースの ID を渡す意味が無い）。
type tracedTransport struct {
	base   http.RoundTripper
	tracer trace.Tracer
}

func (t *tracedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// リクエストの外（起動時など）の呼び出しは、親の無いスパンになるので出さない。
	if !trace.SpanContextFromContext(req.Context()).IsValid() {
		return t.base.RoundTrip(req)
	}
	_, span := t.tracer.Start(req.Context(), req.Method+" "+req.URL.Host,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("http.request.method", req.Method),
			attribute.String("server.address", req.URL.Host),
		))
	defer span.End()
	res, err := t.base.RoundTrip(req)
	if err != nil {
		// 誤りの文には URL がまるごと入る（*url.Error）ので、記録しない。
		span.SetStatus(codes.Error, "request failed")
		return nil, err
	}
	span.SetAttributes(attribute.Int("http.response.status_code", res.StatusCode))
	if res.StatusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(res.StatusCode))
	}
	return res, nil
}
