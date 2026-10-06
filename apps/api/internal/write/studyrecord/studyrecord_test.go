package studyrecord

import "testing"

func TestCheckRange(t *testing.T) {
	page, question, chapter := "page", "question", "chapter"
	total := int64(300)
	n := func(v int64) *int64 { return &v }
	tests := []struct {
		name      string
		tb        textbookSettings
		rangeEnd  *int64
		rangeUnit *string
		want      string
	}{
		{"範囲が無ければ見ない", textbookSettings{rangeUnit: &question, totalAmount: &total}, nil, nil, ""},
		{"単位が違う", textbookSettings{rangeUnit: &question}, n(2), &page, "範囲の単位を参考書の逆算設定に合わせてください"},
		{"単位が無い", textbookSettings{rangeUnit: &question}, n(2), nil, "範囲の単位を参考書の逆算設定に合わせてください"},
		{"総量を超える", textbookSettings{rangeUnit: &page, totalAmount: &total}, n(301), &page, "終了位置は参考書の総量（300）以下にしてください"},
		{"総量ちょうど", textbookSettings{rangeUnit: &page, totalAmount: &total}, n(300), &page, ""},
		{"参考書に設定が無い", textbookSettings{}, n(9999), &chapter, ""},
	}
	for _, tt := range tests {
		got := ""
		if err := tt.tb.checkRange(tt.rangeEnd, tt.rangeUnit); err != nil {
			got = err.Error()
		}
		if got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestOptDiffers(t *testing.T) {
	one, two := int64(1), int64(2)
	tests := []struct {
		name    string
		o       Opt[int64]
		current *int64
		want    bool
	}{
		{"キーが無ければ null とも違う", Opt[int64]{}, nil, true},
		{"キーが無ければ値とも違う", Opt[int64]{}, &one, true},
		{"null と null は同じ", Opt[int64]{Present: true}, nil, false},
		{"null と値は違う", Opt[int64]{Present: true}, &one, true},
		{"同じ値", Opt[int64]{Present: true, Value: &one}, &one, false},
		{"違う値", Opt[int64]{Present: true, Value: &two}, &one, true},
	}
	for _, tt := range tests {
		if got := tt.o.differs(tt.current); got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
}
