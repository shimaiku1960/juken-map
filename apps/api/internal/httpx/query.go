package httpx

import (
	"net/url"
	"strings"
)

// ParseQuery は ?a=1&b=2 を Fastify の既定（fast-querystring）と同じ規則で読む。
// Go の url.ParseQuery と違うところがあり、そのままでは Node と応答がずれるため自前で読む。
//   - 区切りは & だけ。; は値の一部（Go は ; を含む組を捨てる）
//   - = の無い組（?from）は空文字の値
//   - % のデコードに失敗した値（?from=%ZZ）は元の文字のまま（Go は組ごと捨てる）
//   - キーもデコードする（?fr%6Fm=… は from）
//
// 同じキーが2回以上出ると、Fastify ではその値が配列になる。ここでは値の数で表す。
func ParseQuery(raw string) map[string][]string {
	q := map[string][]string{}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		key := unescapeQuery(k)
		q[key] = append(q[key], unescapeQuery(v))
	}
	return q
}

func unescapeQuery(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if decoded, err := url.PathUnescape(s); err == nil {
		return decoded
	}
	return s
}
