// Package dates は日付と時刻の共通の扱い。日本時間の「今日」と、Node（JavaScript の Date）と同じ値への
// そろえ方を持つ。機能をまたいで使うので、feature の外に置く（JUK-156）。
package dates

import (
	"strconv"
	"time"
)

// Tokyo は日本時間。日本には夏時間が無いので、tzdata を読まずに固定の +9時間で足りる。
var Tokyo = time.FixedZone("Asia/Tokyo", 9*60*60)

// OnTokyo は now が東京で何日かを、その日の 00:00 UTC の time.Time で返す。
// 「日付」だけを扱うときは、時差で日がずれないよう UTC の 0時に揃えておく。
// Node 側の todayYmdTokyo（src/shared/date.ts）にあたる。
func OnTokyo(now time.Time) time.Time {
	y, m, d := now.In(Tokyo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// AddDays は日付を n 日ずらす。月末や年末をまたいでも time.Date が繰り上げてくれる。
func AddDays(t time.Time, n int) time.Time {
	return t.AddDate(0, 0, n)
}

func MonthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// MonthEnd は月末日を返す。「翌月の0日」は当月の末日になる。
func MonthEnd(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC)
}

func Earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func Later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func YMD(t time.Time) string {
	return t.Format(time.DateOnly)
}

// NowMillis は今の時刻をミリ秒で切り捨てたもの。DB の DATETIME(3) に書く値に使う。
// Node の new Date() はミリ秒までしか持たない。Go の time.Now() はナノ秒まで持ち、そのまま渡すと
// MySQL が小数第3位へ丸める（切り上がることがある）ので、先に切り捨てて Node と同じ値にする。
func NowMillis() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

// ISOMillis は時刻を Node の Date#toISOString と同じ形にする。
func ISOMillis(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// FromYMD は "YYYY-MM-DD"（暦にある日付だと確かめ済み）を、その日の 00:00 UTC にする。
// Node の new Date("YYYY-MM-DD") と同じ値。
func FromYMD(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t
}

// ParseYMD は YYYY-MM-DD（形は確かめ済み）を、JavaScript の new Date("YYYY-MM-DD") と同じ規則で日付にする。
//   - 月が 1〜12、日が 1〜31 でなければ Invalid Date（false）
//   - 日がその月に無い（2月30日・4月31日）ときは翌月へ繰り越す（2026-02-30 → 2026-03-02）
//
// time.Parse はどちらもエラーにするので使わない。繰り越しは time.Date が同じ規則で行う。
func ParseYMD(s string) (time.Time, bool) {
	y, _ := strconv.Atoi(s[0:4])
	m, _ := strconv.Atoi(s[5:7])
	d, _ := strconv.Atoi(s[8:10])
	if m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, false
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), true
}
