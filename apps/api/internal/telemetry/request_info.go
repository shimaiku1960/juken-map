package telemetry

import (
	"context"
	"slices"
	"sync"
)

// RequestInfo はリクエストごとの情報。一番外側の observe（internal/app/middleware.go）が作って ctx に入れる。
// ポインタで持つので、内側（ルーター）が route を書き込むと外側からも見える。
type RequestInfo struct {
	ID    string
	Sim   bool
	Route string // メトリクスの route ラベル。ルーターが登録した型を入れる（apps/api/internal/httpx/router.go）
	// FaultRoute は障害注入（internal/fault）の対象にしてよいルートなら「GET /api/x/:id」、そうでなければ空。
	FaultRoute string

	mu     sync.Mutex // faults を守る。1つのリクエストの中でも、DB を並行して読むことがある
	faults []string
}

// MarkFault は、このリクエストに起こした障害の種類を記録する。ログの行とスパンに「障害注入中」の印として載る。
func (i *RequestInfo) MarkFault(kind string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if !slices.Contains(i.faults, kind) {
		i.faults = append(i.faults, kind)
	}
}

// Faults は MarkFault で記録した障害の種類。起こしていなければ nil。
func (i *RequestInfo) Faults() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return slices.Clone(i.faults)
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
