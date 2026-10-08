package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"

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
// 外部 API の呼び出し）を記録し、OTLP/HTTP で送る（手元は Tempo、本番は Alloy → Grafana Cloud。JUK-126）。
//
// Go には読み込んだライブラリへ自動で計測を仕込む仕組みが無いので、計測する場所を自分で包む。リクエストは observe（internal/app/middleware.go）、SQL は database.Open の otelsql、
// 外部 API は NewOutboundClient。
//
// トークンを残さないため、スパンには URL のパスも ? 以降も入れない。リクエストはルートの型
// （/api/x/:id）で名付け、外部 API は送り先のホスト名だけを入れる。

// ServiceName は Tempo で探すときの名前。Grafana のダッシュボードの絞り込み
// （resource.service.name）がこの名前で探す。
const ServiceName = "juken-map-api"

// SetupTracing は OTEL_EXPORTER_OTLP_ENDPOINT を設定したときだけトレースを送る準備をする。
// 返す関数は、溜めたスパンを送り切ってから止める（サーバーを止めるときに呼ぶ）。
//
// 送り先（+ /v1/traces）や間引き（OTEL_TRACES_SAMPLER）は、SDK が OTEL_* の環境変数を読んで決める。
// 設定しなければ何も送らず、スパンも作らない（noop）。
func SetupTracing(ctx context.Context) (trace.TracerProvider, func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return noop.NewTracerProvider(), func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, nil, err
	}
	tp := sdktrace.NewTracerProvider(
		// まとめて送る。送り先が落ちていてもリクエストは待たされない（溢れた分は捨てる）。
		sdktrace.WithSpanProcessor(SkipWebSpans{sdktrace.NewBatchSpanProcessor(redactingExporter{exporter})}),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", ServiceName))),
	)
	// database.Open の otelsql は、指定が無ければこの全体の設定を使う。
	otel.SetTracerProvider(tp)
	// 送れなかったとき（Alloy の入れ替え中など）の誤りは、既定だと JSON でない素の文で標準エラーに出る。
	// ほかのログと同じ形にして、Loki で拾えるようにする。
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		slog.Warn("otel error", "err", err.Error())
	}))
	return tp, tp.Shutdown, nil
}

// WebRoute は API 以外（画面の HTML・JS・CSS・画像）をまとめたルート（apps/api/internal/spa/spa.go）。
const WebRoute = "(web)"

// TracedRoute は、そのルートのトレースを送るか。画面のファイル配信は SQL も外部 API も呼ばず、
// 1枚のスパンで終わるので見るものが無い。画面を1回開くだけで十数件になり、Tempo の一覧を埋める
// （本番で、デプロイ後の168件のうち135件がこれだった）。
func TracedRoute(route string) bool {
	return route != WebRoute
}

// SkipWebSpans は、送らないルートのスパンを送る前に捨てる。ルートは返し終えるまで決まらないので、
// スパンを作らずに済ませることはできない（間引き＝Sampler は始める時点で決める）。
type SkipWebSpans struct{ sdktrace.SpanProcessor }

func (p SkipWebSpans) OnEnd(s sdktrace.ReadOnlySpan) {
	for _, kv := range s.Attributes() {
		if kv.Key == "http.route" && !TracedRoute(kv.Value.AsString()) {
			return
		}
	}
	p.SpanProcessor.OnEnd(s)
}

// redactingExporter は送る直前に、スパンの属性・イベント（記録した誤り）・状態の文から
// メールアドレスを伏せる（redact.go）。SQL が失敗すると otelsql が誤りの文をそのままイベントに入れるため。
type redactingExporter struct{ sdktrace.SpanExporter }

func (e redactingExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	redacted := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		redacted[i] = redactedSpan{s}
	}
	return e.SpanExporter.ExportSpans(ctx, redacted)
}

// redactedSpan は、送るときに読まれる値だけを伏せた形で返す。元のスパンは書き換えない。
type redactedSpan struct{ sdktrace.ReadOnlySpan }

func (s redactedSpan) Attributes() []attribute.KeyValue {
	return redactAttributes(s.ReadOnlySpan.Attributes())
}

func (s redactedSpan) Events() []sdktrace.Event {
	events := s.ReadOnlySpan.Events()
	out := make([]sdktrace.Event, len(events))
	for i, ev := range events {
		ev.Attributes = redactAttributes(ev.Attributes)
		out[i] = ev
	}
	return out
}

func (s redactedSpan) Status() sdktrace.Status {
	st := s.ReadOnlySpan.Status()
	st.Description = redactEmails(st.Description)
	return st
}

func redactAttributes(attrs []attribute.KeyValue) []attribute.KeyValue {
	out := make([]attribute.KeyValue, len(attrs))
	for i, kv := range attrs {
		if kv.Value.Type() == attribute.STRING {
			kv.Value = attribute.StringValue(redactEmails(kv.Value.AsString()))
		}
		out[i] = kv
	}
	return out
}

// StartRequestSpan は1リクエスト全体のスパンを始める。名前はルートが決まってから付け直す（EndRequestSpan）。
//
// 外から来た traceparent ヘッダーは読まず（propagator を設定していない）、毎回ここを根にする。
// 外の誰かが決めた ID でトレースを作らせないため（この API を呼ぶのは自分の画面と外部サービスで、
// どちらもトレースを渡してこない）。
func StartRequestSpan(ctx context.Context, tracer trace.Tracer, method string) (context.Context, trace.Span) {
	return tracer.Start(ctx, method, trace.WithSpanKind(trace.SpanKindServer),
		trace.WithAttributes(attribute.String("http.request.method", method)))
}

// EndRequestSpan はルートの型で名付け直し、結果を書いて閉じる。
// 名前は「GET /api/x/:id」の形にする。ダッシュボードが
// 「GET /api/health」の根を除く絞り込みを持っているため。
func EndRequestSpan(span trace.Span, method, route string, status int, info *RequestInfo) {
	span.SetName(method + " " + route)
	span.SetAttributes(
		attribute.String("http.route", route),
		attribute.Int("http.response.status_code", status),
		attribute.String("reqId", info.ID),
	)
	if info.Sim {
		span.SetAttributes(attribute.Bool("sim", true))
	}
	// OpenTelemetry の決まりで、サーバーの 4xx は呼び出し側の誤りなのでエラーにしない。
	if status >= 500 {
		span.SetStatus(codes.Error, http.StatusText(status))
	}
	span.End()
}

// NewOutboundClient は外部 API（Resend・LINE・microCMS・GitHub）を呼ぶクライアント。
// 呼び出し1回ごとにスパンを作る。tracer は呼び出し元（ctx）のスパンの子にするために使う。
func NewOutboundClient(tp trace.TracerProvider) *http.Client {
	return &http.Client{Transport: &tracedTransport{
		base:   http.DefaultTransport,
		tracer: tp.Tracer(ServiceName),
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
