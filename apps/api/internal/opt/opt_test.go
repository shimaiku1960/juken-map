package opt

import "testing"

func TestDiffers(t *testing.T) {
	// 送られた値が DB の値と違うか。current は DB の値で、null か値。
	one, two := int64(1), int64(2)
	missing, null, value := Field[int64]{}, Field[int64]{Present: true}, Of(one)
	tests := []struct {
		name    string
		f       Field[int64]
		current *int64
		want    bool
	}{
		{"無い（undefined）は null とも違う", missing, nil, true},
		{"無い（undefined）は値とも違う", missing, &one, true},
		{"null と null は同じ", null, nil, false},
		{"null と値は違う", null, &one, true},
		{"値と null は違う", value, nil, true},
		{"同じ値", value, &one, false},
		{"違う値", value, &two, true},
	}
	for _, tt := range tests {
		if got := tt.f.Differs(tt.current); got != tt.want {
			t.Errorf("%s: Differs = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestOr(t *testing.T) {
	one, two := int64(1), int64(2)
	if got := (Field[int64]{}).Or(&one); got != &one {
		t.Errorf("無ければ fallback: %v", got)
	}
	if got := (Field[int64]{Present: true}).Or(&one); got != nil {
		t.Errorf("null なら null: %v", *got)
	}
	if got := Of(two).Or(&one); got == nil || *got != 2 {
		t.Errorf("送られた値: %v", got)
	}
}
