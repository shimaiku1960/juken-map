package main

import (
	"testing"
	"time"
)

func TestDateOnTokyo(t *testing.T) {
	// UTC の 9/30 15:00 は、東京ではもう 10/1 の 0:00。
	got := ymd(dateOnTokyo(time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)))
	if got != "2026-10-01" {
		t.Errorf("dateOnTokyo = %s, want 2026-10-01", got)
	}
}

func TestMonthEnd(t *testing.T) {
	tests := []struct{ in, want string }{
		{"2026-02-10", "2026-02-28"},
		{"2028-02-10", "2028-02-29"}, // うるう年
		{"2026-12-31", "2026-12-31"}, // 年末
	}
	for _, tt := range tests {
		in, _ := time.Parse(time.DateOnly, tt.in)
		if got := ymd(monthEnd(in)); got != tt.want {
			t.Errorf("monthEnd(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestIsoFromDatetime(t *testing.T) {
	if got := isoFromDatetime("2026-09-27 00:00:00.000"); got != "2026-09-27T00:00:00.000Z" {
		t.Errorf("isoFromDatetime = %s", got)
	}
}
