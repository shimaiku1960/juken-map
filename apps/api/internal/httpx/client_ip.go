package httpx

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP は接続元の IP。本番と開発の nginx は X-Forwarded-For を接続元（$remote_addr）で上書きして渡すので、
// その値を使う（利用者が送ってきた値は nginx が捨てている）。nginx を通らないとき（テスト）は接続そのものの値。
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(first)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
