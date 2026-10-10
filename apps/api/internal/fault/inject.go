package fault

import (
	"context"
	"database/sql/driver"
	"net/http"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// 障害を差し込む3か所。どれも Injector が nil なら何もしない。

// InjectHTTP は httpx.Router から、対象のルートのリクエストごとに呼ばれる（httpx.FaultInjector）。
// 遅延はハンドラの前に待つ。5xx はハンドラへ進まずに返す（応答の本文は本物の 500 と同じで、画面には障害注入だと出さない）。
func (in *Injector) InjectHTTP(w http.ResponseWriter, r *http.Request) bool {
	if e, ok := in.pick(r.Context(), KindLatency); ok {
		// 待っている間にリクエストの時間の上限が来たら、そのままハンドラへ進む（ハンドラが ctx の取り消しで失敗する）。
		_ = in.sleep(r.Context(), e.delay())
	}
	if e, ok := in.pick(r.Context(), KindHTTPError); ok {
		httpx.WriteErrorBody(w, r, e.StatusCode, httpx.CodeInternal)
		return true
	}
	return false
}

// WrapConnector は DB の接続に障害を差し込む（internal/database の Open に渡す）。
func (in *Injector) WrapConnector(c driver.Connector) driver.Connector {
	if in == nil {
		return c
	}
	return &faultConnector{base: c, in: in}
}

type faultConnector struct {
	base driver.Connector
	in   *Injector
}

func (c *faultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.base.Connect(ctx)
	if err != nil {
		return nil, err
	}
	full, ok := conn.(fullConn)
	if !ok {
		// 足りない機能を database/sql が別の道で補うと、振る舞いが変わる。包まずに返す。
		return conn, nil
	}
	return &faultConn{fullConn: full, in: c.in}, nil
}

func (c *faultConnector) Driver() driver.Driver {
	return c.base.Driver()
}

// fullConn は MySQL のドライバの接続が持つ機能の全部。包んでも、database/sql から見た機能が減らないようにする。
type fullConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.Pinger
	driver.SessionResetter
	driver.Validator
	driver.NamedValueChecker
}

// faultConn は SQL を流す入口（照会・書き込み・準備・トランザクションの開始）の手前で障害を起こす。
// ほかのメソッドはそのまま元の接続に渡る。
type faultConn struct {
	fullConn
	in *Injector
}

func (c *faultConn) fault(ctx context.Context) error {
	if _, ok := c.in.pick(ctx, KindDBError); ok {
		return ErrInjected
	}
	return nil
}

func (c *faultConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := c.fault(ctx); err != nil {
		return nil, err
	}
	return c.fullConn.QueryContext(ctx, query, args)
}

func (c *faultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := c.fault(ctx); err != nil {
		return nil, err
	}
	return c.fullConn.ExecContext(ctx, query, args)
}

func (c *faultConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := c.fault(ctx); err != nil {
		return nil, err
	}
	return c.fullConn.PrepareContext(ctx, query)
}

func (c *faultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if err := c.fault(ctx); err != nil {
		return nil, err
	}
	return c.fullConn.BeginTx(ctx, opts)
}

// Transport は外部 API のクライアントに障害を差し込む（telemetry.NewOutboundClient の base に渡す）。
func (in *Injector) Transport(base http.RoundTripper) http.RoundTripper {
	if in == nil {
		return base
	}
	return &faultTransport{base: base, in: in}
}

type faultTransport struct {
	base http.RoundTripper
	in   *Injector
}

func (t *faultTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	e, ok := t.in.pick(req.Context(), KindOutboundTimeout)
	if !ok {
		return t.base.RoundTrip(req)
	}
	// RoundTrip は、失敗するときも本文を閉じる決まり。
	if req.Body != nil {
		_ = req.Body.Close()
	}
	if err := t.in.sleep(req.Context(), e.delay()); err != nil {
		return nil, err
	}
	return nil, outboundTimeout{}
}

// outboundTimeout は相手が返事をしなかったときの誤り。net.Error として、タイムアウトだと分かる形にする。
type outboundTimeout struct{}

func (outboundTimeout) Error() string {
	// 本物の応答待ちのタイムアウト（http.Transport.ResponseHeaderTimeout）と同じ文言にする（JUK-178）。
	return "net/http: timeout awaiting response headers"
}
func (outboundTimeout) Timeout() bool   { return true }
func (outboundTimeout) Temporary() bool { return true }
