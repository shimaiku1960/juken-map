package main

import (
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
)

// クエリ文字列の読み方と、不正なときの 400 の形を Node（Fastify ＋ Zod）に揃える。

// dateRangeQuery は ?from=&to= を読んだ結果。省かれたものは nil。
type dateRangeQuery struct {
	from, to *string
}

// readDateRangeQuery は ?from=&to= を読む。形が不正なら 400 を送って false を返す。
// Node の rangeQuerySchema（routes/study-logs.ts・study-plans.ts）と同じ判定で、
// 不正な項目が複数あれば from → to の順に見て、最初の1件を返す（Node と同じ）。
func readDateRangeQuery(w http.ResponseWriter, r *http.Request) (dateRangeQuery, bool) {
	q := httpx.ParseQuery(r.URL.RawQuery)
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
			httpx.WriteValidationError(w, "invalid_type", f.key, "Invalid input: expected string, received array")
			return dateRangeQuery{}, false
		case !httpx.YMDPattern.MatchString(values[0]):
			httpx.WriteValidationError(w, "invalid_format", f.key, httpx.YMDMessage)
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
		from, ok := dates.ParseYMD(*q.from)
		if !ok {
			return dateRange{}, false
		}
		r.from = from
	}
	if q.to != nil {
		to, ok := dates.ParseYMD(*q.to)
		if !ok {
			return dateRange{}, false
		}
		r.to = &to
	}
	return r, true
}
