package main

import (
	"time"
)

// 日本には夏時間が無いので、tzdata を読まずに固定の +9時間で足りる。
var tokyo = time.FixedZone("Asia/Tokyo", 9*60*60)

// dateOnTokyo は now が東京で何日かを、その日の 00:00 UTC の time.Time で返す。
// 「日付」だけを扱うときは、時差で日がずれないよう UTC の 0時に揃えておく。
// Node 側の todayYmdTokyo（src/shared/date.ts）にあたる。
func dateOnTokyo(now time.Time) time.Time {
	y, m, d := now.In(tokyo).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// addDays は日付を n 日ずらす。月末や年末をまたいでも time.Date が繰り上げてくれる。
func addDays(t time.Time, n int) time.Time {
	return t.AddDate(0, 0, n)
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// monthEnd は月末日を返す。「翌月の0日」は当月の末日になる。
func monthEnd(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC)
}

func earlier(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func ymd(t time.Time) string {
	return t.Format(time.DateOnly)
}
