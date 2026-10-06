package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// ブラウザが送ってくる CSP の違反の報告を、ログ（本番は Grafana の Loki）に残す（JUK-80）。
// Node の routes/csp-report.ts にあたる。CSP は止めるモードなので、ここに出たものは
// 「実際にブラウザが読み込みを拒んだ箇所」になる。
//
// 報告の形は2種類ある。
//   - report-uri: application/csp-report、{"csp-report": {"document-uri": ...}}（今使っているのはこちら）
//   - report-to（Reporting API）: application/reports+json、[{"type": "csp-violation", "body": {"documentURL": ...}}]

// 認証なしで誰でも送れる口なので、1回で読む件数と大きさに上限を置く。
const (
	cspMaxReports = 20
	cspBodyLimit  = 16 * 1024
)

// cspViolation はログに残す1件。無い項目はキーごと出さない（Node の undefined と同じ）。
type cspViolation struct {
	DocumentURL string `json:"documentUrl,omitempty"`
	// Directive は Node が String(... ?? "") で必ず文字列にしているので、空でも出す。
	Directive   string      `json:"directive"`
	BlockedURL  string      `json:"blockedUrl,omitempty"`
	SourceFile  string      `json:"sourceFile,omitempty"`
	LineNumber  json.Number `json:"lineNumber,omitempty"`
	Disposition string      `json:"disposition,omitempty"`
}

// resetPasswordToken は Better Auth のパスワード再設定のリンク（トークンがパスに入る）。Node の redactPath と同じ。
var resetPasswordToken = regexp.MustCompile(`^(/api/auth/reset-password/)[^/]+`)

// safeURL はログに残す URL から、クエリと再設定のトークンを取り除く（origin ＋ パス）。
// "inline"・"eval"・"data" のような URL でない値は、200 文字までにしてそのまま残す。
//
// Node は new URL() で読めるものを URL として扱うので、data: や blob: も「null」＋パスの形になる。
// Go では http・https だけを URL として扱い、それ以外は長さを切ってそのまま残す
// （data: の中身を丸ごとログに入れないため。ログの形だけの違いで、応答は変わらない）。
func safeURL(value any) string {
	s, ok := value.(string)
	if !ok {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return truncateRunes(s, 200)
	}
	// URL の origin と同じく、ホストは小文字にし、既定のポートは書かない。
	host := strings.ToLower(u.Host)
	host = strings.TrimSuffix(host, map[string]string{"http": ":80", "https": ":443"}[u.Scheme])
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	return u.Scheme + "://" + host + resetPasswordToken.ReplaceAllString(path, "${1}:token")
}

// truncateRunes は JavaScript の slice(0, n) に近い切り方（バイトではなく文字で数える）。
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func toViolation(report map[string]any) cspViolation {
	pick := func(keys ...string) any {
		for _, k := range keys {
			if v, ok := report[k]; ok {
				return v
			}
		}
		return nil
	}
	v := cspViolation{
		DocumentURL: safeURL(pick("documentURL", "document-uri")),
		BlockedURL:  safeURL(pick("blockedURL", "blocked-uri")),
		SourceFile:  safeURL(pick("sourceFile", "source-file")),
	}
	// ブラウザが送るのは文字列。それ以外（Node なら String() で "[object Object]" など）は空にする。
	v.Directive, _ = pick("effectiveDirective", "effective-directive", "violated-directive").(string)
	v.LineNumber, _ = pick("lineNumber", "line-number").(json.Number)
	v.Disposition, _ = pick("disposition").(string)
	return v
}

// parseCSPReports は2種類の形の報告を、同じ形の一覧にそろえる。読めないものは捨てる。
func parseCSPReports(body any) []cspViolation {
	var out []cspViolation
	switch b := body.(type) {
	case []any:
		for _, item := range b {
			if len(out) == cspMaxReports {
				break
			}
			m, ok := item.(map[string]any)
			if !ok || m["type"] != "csp-violation" {
				continue
			}
			if report, ok := m["body"].(map[string]any); ok {
				out = append(out, toViolation(report))
			}
		}
	case map[string]any:
		if report, ok := b["csp-report"].(map[string]any); ok {
			out = append(out, toViolation(report))
		}
	}
	return out
}

// cspReport は POST /api/csp-report。ブラウザは中身を読まないので、報告の形が崩れていても、
// 送り直させないよう常に 204（Content-Type が違う・大きすぎるときは、本文を読む前に 415・413）。
func cspReport(w http.ResponseWriter, r *http.Request) {
	body, ok := httpx.ReadBody(w, r, cspBodyLimit)
	if !ok {
		return
	}
	for _, v := range parseCSPReports(body.JSON) {
		slog.WarnContext(r.Context(), "csp violation", "csp", v)
	}
	w.WriteHeader(http.StatusNoContent)
}
