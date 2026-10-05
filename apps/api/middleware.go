package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// ミドルウェアは「http.Handler を受け取り、前後に処理を足した http.Handler を返す関数」。
// Fastify のフック（onRequest・onResponse など）にあたるものを、包む順番で表す。
// 組み立ては main.go の newServerHandler にある。

// requestInfo はリクエストごとの情報。一番外側の observe が作って ctx に入れる。
// ポインタで持つので、内側（ルーター）が route を書き込むと外側からも見える。
type requestInfo struct {
	id    string
	sim   bool
	route string // メトリクスの route ラベル。ルーターが登録した型を入れる（router.go）
}

// ctx のキーは、他のパッケージのキーとぶつからないよう専用の型にする（Go の決まり）。
type requestInfoKey struct{}

func requestInfoFrom(ctx context.Context) *requestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*requestInfo)
	return info
}

// requestIDFrom はログとエラー応答に載せる reqId。リクエストの外では空文字。
func requestIDFrom(ctx context.Context) string {
	if info := requestInfoFrom(ctx); info != nil {
		return info.id
	}
	return ""
}

// newRequestID は UUID（v4）を作る。Node と同じく、本番は36文字、開発は先頭8文字にする
// （開発は人が目で読むので短さを取る。observability/logger.ts の genReqId）。
func newRequestID(short bool) string {
	var b [16]byte
	rand.Read(b[:])         // crypto/rand の Read は失敗しない（失敗したらプロセスが止まる）
	b[6] = b[6]&0x0f | 0x40 // 版（4）
	b[8] = b[8]&0x3f | 0x80 // 種類（RFC 9562）
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	if short {
		return id[:8]
	}
	return id
}

// statusRecorder は、ハンドラが書いたステータスを後から読めるようにする。
// ResponseWriter は書いたステータスを教えてくれないので、包んで横取りする。
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(status int) {
	if !r.wroteHeader {
		r.status = status
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(status)
}

// Write はヘッダーを書かずに本文を書き始めると、暗黙に 200 になる。その場合も記録する。
func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap は http.ResponseController が元の ResponseWriter の機能（Flush など）を探すのに使う。
func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// observe は一番外側で、reqId を振り、返し終えたらアクセスログ1行とメトリクスとトレースを残す。
// Node の genReqId・requestContext・RequestLogController・registerMetrics と、
// instrumentation.ts の HTTP の計測をまとめたもの。
func observe(m *metrics, tracer trace.Tracer, shortIDs bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &requestInfo{
			id: newRequestID(shortIDs),
			// シミュレーションからのリクエストに印を付ける（Grafana で実利用者と分ける）。
			sim: r.Header.Get("X-Sim-Run") != "",
		}
		// トレースのスパンも ctx に入れる。内側の SQL・外部 API のスパンはこの子になり、
		// ログの行には trace_id が付く（logger.go）。
		ctx, span := startRequestSpan(r.Context(), tracer, r.Method)
		r = r.WithContext(context.WithValue(ctx, requestInfoKey{}, info))
		// 調査のときに画面の Network タブの値でログを引けるよう、応答ヘッダーに載せる
		// （Node の error-handling.ts。この API はすべて /api/ なので常に付ける）。
		w.Header().Set("X-Request-Id", info.id)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		elapsed := time.Since(start)
		route := info.route
		if route == "" {
			route = "(unmatched)"
		}
		m.observe(r.Method, route, rec.status, elapsed)
		endRequestSpan(span, r.Method, route, rec.status, info)
		logCtx := r.Context()
		if !tracedRoute(route) {
			// 送らないトレースの ID をログに書くと、Grafana で開いても見つからないリンクになる。
			logCtx = trace.ContextWithSpanContext(logCtx, trace.SpanContext{})
		}

		// Node の「request completed」と同じ形。URL はパスだけにして、? 以降は残さない
		// （クエリにトークンが載る入口があるため。Node の redactPath）。
		slog.LogAttrs(logCtx, slog.LevelInfo, "request completed",
			slog.Group("req", slog.String("method", r.Method), slog.String("url", r.URL.Path)),
			slog.Group("res", slog.Int("statusCode", rec.status)),
			slog.Float64("responseTime", math.Round(float64(elapsed.Microseconds())/100)/10),
		)
	})
}

// recoverPanic は、ハンドラの panic を 500 の応答に変える。
//
// net/http も panic でプロセスを落とさないが、その接続を切るだけで応答を返さず、
// ログも reqId の無い素のテキストになる。Node の setErrorHandler と同じく、
// 固定文言の 500 を返して、原因はログにだけ残す。
func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			// ErrAbortHandler は「応答を打ち切る」合図として net/http が使う値なので、そのまま投げ直す。
			if v == http.ErrAbortHandler {
				panic(v)
			}
			slog.ErrorContext(r.Context(), "request failed", "err", fmt.Sprint(v), "statusCode", 500, "code", codeInternal)
			// 本文を書き始めた後だと、ステータスはもう変えられない。書く前なら 500 を返す。
			if rec, ok := w.(*statusRecorder); !ok || !rec.wroteHeader {
				writeErrorBody(w, r, http.StatusInternalServerError, codeInternal)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// securityHeaders は、Node（security-headers.ts）が全応答に付けているヘッダーのうち、
// JSON の API にも意味があるものを付ける。
//
// CSP だけは Node と変える。Node の CSP は画面（HTML）向けで、読み込んでよいスクリプトや
// 画像の一覧になっている。この API は JSON しか返さないので、何も読み込ませない
// 'none' にする（OWASP の REST の推奨と同じ）。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Strict-Transport-Security", "max-age=31536000")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// withDeadline は、1リクエストにかけてよい時間の上限を ctx に付ける。
//
// DB の照会も接続プールの空き待ちも、ハンドラが渡す ctx が取り消された時点で止まる。
// 上限が無いと、DB が詰まったときに待ちが積み上がり、全員が遅くなる（Node の
// timeout.ts が外部サービスについて書いている理由と同じ）。止まった照会はエラーとして
// ハンドラへ戻り、500 になる。
func withDeadline(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
