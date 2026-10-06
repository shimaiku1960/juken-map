package dates

import (
	"testing"
	"time"
)

func TestDateOnTokyo(t *testing.T) {
	// UTC の 9/30 15:00 は、東京ではもう 10/1 の 0:00。
	got := YMD(OnTokyo(time.Date(2026, 9, 30, 15, 0, 0, 0, time.UTC)))
	if got != "2026-10-01" {
		t.Errorf("OnTokyo = %s, want 2026-10-01", got)
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
		if got := YMD(MonthEnd(in)); got != tt.want {
			t.Errorf("MonthEnd(%s) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestParseYMD(t *testing.T) {
	// 期待値は Node で new Date("YYYY-MM-DD").toISOString() を確かめたもの。"" は Invalid Date。
	tests := map[string]string{
		"2026-09-29": "2026-09-29",
		"2024-02-29": "2024-02-29",
		"2026-02-29": "2026-03-01",
		"2026-02-30": "2026-03-02",
		"2026-04-31": "2026-05-01",
		"0000-01-01": "0000-01-01",
		"9999-12-31": "9999-12-31",
		"2026-13-45": "",
		"2026-00-10": "",
		"2026-01-00": "",
		"2026-01-32": "",
	}
	for in, want := range tests {
		got, ok := ParseYMD(in)
		switch {
		case want == "" && ok:
			t.Errorf("ParseYMD(%q) = %s, want Invalid", in, YMD(got))
		case want != "" && !ok:
			t.Errorf("ParseYMD(%q) = Invalid, want %s", in, want)
		case want != "" && YMD(got) != want:
			t.Errorf("ParseYMD(%q) = %s, want %s", in, YMD(got), want)
		}
	}
}
