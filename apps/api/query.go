package main

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
)

// クエリ文字列の読み方と、不正なときの 400 の形を Node（Fastify ＋ Zod）に揃える。

// parseQuery は ?a=1&b=2 を Fastify の既定（fast-querystring）と同じ規則で読む。
// Go の url.ParseQuery と違うところがあり、そのままでは Node と応答がずれるため自前で読む。
//   - 区切りは & だけ。; は値の一部（Go は ; を含む組を捨てる）
//   - = の無い組（?from）は空文字の値
//   - % のデコードに失敗した値（?from=%ZZ）は元の文字のまま（Go は組ごと捨てる）
//   - キーもデコードする（?fr%6Fm=… は from）
//
// 同じキーが2回以上出ると、Fastify ではその値が配列になる。ここでは値の数で表す。
func parseQuery(raw string) map[string][]string {
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

// writeValidationError は 400 を返す。Node と同じく、弾いた理由の最初の1件だけを返す。
func writeValidationError(w http.ResponseWriter, code, field, message string) {
	writeJSON(w, http.StatusBadRequest, apischema.ValidationError{Error: message, Code: code, Field: &field})
}

// ymdPattern は Node の ymdField（z.string().regex(/^\d{4}-\d{2}-\d{2}$/)）と同じ形。
// 形だけを見て、13月や2月30日は通す（下の parseYMD で扱う）。
var ymdPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

const ymdMessage = "日付は YYYY-MM-DD で指定してください"

// dateRangeQuery は ?from=&to= を読んだ結果。省かれたものは nil。
type dateRangeQuery struct {
	from, to *string
}

// readDateRangeQuery は ?from=&to= を読む。形が不正なら 400 を送って false を返す。
// Node の rangeQuerySchema（routes/study-logs.ts・study-plans.ts）と同じ判定で、
// 不正な項目が複数あれば from → to の順に見て、最初の1件を返す（Node と同じ）。
func readDateRangeQuery(w http.ResponseWriter, r *http.Request) (dateRangeQuery, bool) {
	q := parseQuery(r.URL.RawQuery)
	var out dateRangeQuery
	for _, f := range []struct {
		key string
		dst **string
	}{{"from", &out.from}, {"to", &out.to}} {
		values, ok := q[f.key]
		switch {
		case !ok:
		case len(values) > 1:
			// 同じキーが2回以上あると Fastify では配列になり、Zod の z.string() が invalid_type で弾く。
			writeValidationError(w, "invalid_type", f.key, "Invalid input: expected string, received array")
			return dateRangeQuery{}, false
		case !ymdPattern.MatchString(values[0]):
			writeValidationError(w, "invalid_format", f.key, ymdMessage)
			return dateRangeQuery{}, false
		default:
			*f.dst = &values[0]
		}
	}
	return out, true
}

// resolve は省かれた方に既定値を入れて、期間にする。defaultTo が nil なら上限なし。
//
// 2つ目の戻り値が false のときは「何にも当たらない期間」で、呼び出し側は空の一覧を返す。
// 形は正しいが暦に無い日付（2026-13-45 など）がそれにあたる。Node はその値を new Date に通して
// Invalid Date にし、SQL の比較がどの行にも当たらず [] を返している。400 にはしていないので揃える。
func (q dateRangeQuery) resolve(defaultFrom time.Time, defaultTo *time.Time) (dateRange, bool) {
	r := dateRange{from: defaultFrom, to: defaultTo}
	if q.from != nil {
		from, ok := parseYMD(*q.from)
		if !ok {
			return dateRange{}, false
		}
		r.from = from
	}
	if q.to != nil {
		to, ok := parseYMD(*q.to)
		if !ok {
			return dateRange{}, false
		}
		r.to = &to
	}
	return r, true
}

// parseYMD は YYYY-MM-DD（形は確かめ済み）を、JavaScript の new Date("YYYY-MM-DD") と同じ規則で日付にする。
//   - 月が 1〜12、日が 1〜31 でなければ Invalid Date（false）
//   - 日がその月に無い（2月30日・4月31日）ときは翌月へ繰り越す（2026-02-30 → 2026-03-02）
//
// time.Parse はどちらもエラーにするので使わない。繰り越しは time.Date が同じ規則で行う。
func parseYMD(s string) (time.Time, bool) {
	y, _ := strconv.Atoi(s[0:4])
	m, _ := strconv.Atoi(s[5:7])
	d, _ := strconv.Atoi(s[8:10])
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}
