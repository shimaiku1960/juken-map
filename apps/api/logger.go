package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

// ログは Node（pino）と同じ形の JSON を1行ずつ書く。本番では Alloy が Docker のログを読み、
// pino の形を前提に Loki へ送っている（observability/alloy/production.alloy）。形が違うと、
// 時刻とレベルの取り出しや、Grafana の「5xx の行」パネル（req_url・res_statusCode）が空になる。
//
// pino との違いは次の2つで、slog の書き方を ReplaceAttr で pino に寄せる。
//   - level は数字（30 = info、50 = error）。slog は "INFO" の文字列
//   - time は UNIX ミリ秒の数字。slog は RFC 3339 の文字列

// pinoLevels は slog のレベルを pino の数字にする表。
var pinoLevels = map[slog.Level]int{
	slog.LevelDebug: 20,
	slog.LevelInfo:  30,
	slog.LevelWarn:  40,
	slog.LevelError: 50,
}

// newLogger は pino と同じ形で w に書くロガーを作る。pid・hostname も pino と同じく毎行に付ける。
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.TimeKey:
				return slog.Int64("time", a.Value.Time().UnixMilli())
			case slog.LevelKey:
				return slog.Int("level", pinoLevels[a.Value.Any().(slog.Level)])
			}
			return a
		},
	})
	hostname, _ := os.Hostname()
	return slog.New(requestContextHandler{h}).With("pid", os.Getpid(), "hostname", hostname)
}

// logOutput は、LOG_FILE を設定したときだけ、標準出力に加えてそのファイルにも同じ行を書く。
// 手元の Alloy がこのファイルを読んで Loki へ送る（observability/alloy/config.alloy）。本番は Docker の
// ログを読むので使わない。相対パスは起動したディレクトリから（pnpm dev ではリポジトリのルート）。
func logOutput(stdout io.Writer, path string) (io.Writer, error) {
	if path == "" {
		return stdout, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// 起動し直しても前のログを消さず、後ろに足す。閉じずにプロセスの終了まで開いたままにする。
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return io.MultiWriter(stdout, f), nil
}

// parseLevel は LOG_LEVEL（pino と同じ名前）を slog のレベルにする。知らない値は info にする。
func parseLevel(name string) slog.Level {
	switch strings.ToLower(name) {
	case "trace", "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error", "fatal":
		return slog.LevelError
	}
	return slog.LevelInfo
}

// requestContextHandler は、ctx にリクエストの情報があれば reqId（と sim）を行に足す。
// トレースのスパンがあれば trace_id・span_id も足す（Node の pino の計測と同じ名前）。Grafana で
// Loki のログから Tempo のトレースを開くリンクは、この trace_id を拾って作る（provisioning/datasources/loki.yml）。
//
// Node は AsyncLocalStorage で「今どのリクエストの処理中か」を持ち回っていた
// （observability/requestContext.ts）。Go は ctx を引数で渡すのが決まりなので、
// slog.InfoContext(ctx, ...) のように ctx 付きで書けば、ここで reqId が付く。
type requestContextHandler struct {
	slog.Handler
}

func (h requestContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if info := requestInfoFrom(ctx); info != nil {
		r.AddAttrs(slog.String("reqId", info.id))
		if info.sim {
			r.AddAttrs(slog.Bool("sim", true))
		}
	}
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	return h.Handler.Handle(ctx, r)
}

// With や WithGroup で作った子のロガーも、この Handle を通るように包み直す。
// 包み直さないと、中の JSONHandler がそのまま返り、reqId が付かなくなる。

func (h requestContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestContextHandler{h.Handler.WithAttrs(attrs)}
}

func (h requestContextHandler) WithGroup(name string) slog.Handler {
	return requestContextHandler{h.Handler.WithGroup(name)}
}
