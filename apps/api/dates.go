package main

import (
	"strconv"
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

// isCalendarYMD は "YYYY-MM-DD"（形は確かめ済み）が暦にある日付か。2026-02-30・2026-13-01 は false。
// Node の isCalendarYmd（src/shared/date.ts）と同じ算数で判定する。time.Date は範囲外の日を翌月へ
// 繰り越してしまうので使わない。
func isCalendarYMD(s string) bool {
	y, _ := strconv.Atoi(s[0:4])
	m, _ := strconv.Atoi(s[5:7])
	d, _ := strconv.Atoi(s[8:10])
	if m < 1 || m > 12 || d < 1 {
		return false
	}
	leap := (y%4 == 0 && y%100 != 0) || y%400 == 0
	days := [12]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if leap {
		days[1] = 29
	}
	return d <= days[m-1]
}
