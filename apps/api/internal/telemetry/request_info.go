package telemetry

import "context"

// RequestInfo はリクエストごとの情報。一番外側の observe（internal/app/middleware.go）が作って ctx に入れる。
// ポインタで持つので、内側（ルーター）が route を書き込むと外側からも見える。
type RequestInfo struct {
	ID    string
	Sim   bool
	Route string // メトリクスの route ラベル。ルーターが登録した型を入れる（apps/api/internal/httpx/router.go）
	// FaultRoute は障害注入（internal/fault）の対象にしてよいルートなら「GET /api/x/:id」、そうでなければ空。
	// 障害を起こしたかどうかは、ログ・trace・数値のどこにも載せない。調べる練習で答えにならないようにするため（JUK-178）。
	FaultRoute string
}

// ctx のキーは、他のパッケージのキーとぶつからないよう専用の型にする（Go の決まり）。
type requestInfoKey struct{}

// WithRequestInfo は info を入れた ctx を返す。
func WithRequestInfo(ctx context.Context, info *RequestInfo) context.Context {
	return context.WithValue(ctx, requestInfoKey{}, info)
}

func RequestInfoFrom(ctx context.Context) *RequestInfo {
	info, _ := ctx.Value(requestInfoKey{}).(*RequestInfo)
	return info
}

// RequestIDFrom はログとエラー応答に載せる reqId。リクエストの外では空文字。
func RequestIDFrom(ctx context.Context) string {
	if info := RequestInfoFrom(ctx); info != nil {
		return info.ID
	}
	return ""
}
