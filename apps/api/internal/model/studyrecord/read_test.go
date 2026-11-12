package studyrecord

import (
	"reflect"
	"testing"
	"time"
)

func TestPeriodWhere(t *testing.T) {
	from := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)

	where, args := Period{From: from, To: &to}.where("l", "u1")
	if where != "l.userId = ? AND l.date >= ? AND l.date < ?" {
		t.Errorf("where = %q", where)
	}
	// to の当日を含めるため、上限は翌日の 00:00
	want := []any{"u1", from, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}

	where, args = Period{From: from}.where("p", "u1")
	if where != "p.userId = ? AND p.date >= ?" || len(args) != 2 {
		t.Errorf("上限なし: where = %q, args = %v", where, args)
	}
}

func TestReadPeriod(t *testing.T) {
	defFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	defTo := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	p, ok, err := ReadPeriod(map[string][]string{}, defFrom, &defTo)
	if err != nil || !ok || !p.From.Equal(defFrom) || !p.To.Equal(defTo) {
		t.Errorf("省略 = %+v %v %v", p, ok, err)
	}
	p, ok, err = ReadPeriod(map[string][]string{"from": {"2026-09-01"}, "to": {"2026-09-30"}}, defFrom, nil)
	if err != nil || !ok || !p.From.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) ||
		p.To == nil || !p.To.Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("指定 = %+v %v %v", p, ok, err)
	}
	// 形は正しいが暦に無い日付は、誤りではなく「何にも当たらない」
	if _, ok, err := ReadPeriod(map[string][]string{"to": {"2026-13-45"}}, defFrom, nil); ok || err != nil {
		t.Errorf("暦に無い = %v %v", ok, err)
	}

	for name, tt := range map[string]struct {
		query map[string][]string
		want  string
	}{
		"形が違う":        {map[string][]string{"from": {"abc"}}, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "from")},
		"from が先":     {map[string][]string{"from": {"x"}, "to": {"y"}}, bad("日付は YYYY-MM-DD で指定してください", "invalid_format", "from")},
		"同じキーが2つ（配列）": {map[string][]string{"to": {"2026-09-01", "2026-09-02"}}, bad("Invalid input: expected string, received array", "invalid_type", "to")},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := ReadPeriod(tt.query, defFrom, nil)
			checkIssue(t, err, tt.want)
		})
	}
}
