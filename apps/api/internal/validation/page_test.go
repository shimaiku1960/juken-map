package validation

import (
	"math"
	"testing"
)

func TestJSNumberFromString(t *testing.T) {
	nan := math.NaN()
	tests := map[string]float64{
		"": 0, "  ": 0, "1": 1, " 2 ": 2, "\n3\t": 3, "+4": 4, "-5": -5, "1.5": 1.5, ".5": 0.5, "5.": 5,
		"1e3": 1000, "1E-1": 0.1, "0x10": 16, "0X1f": 31, "0o17": 15, "0b101": 5, "007": 7,
		"Infinity": math.Inf(1), "-Infinity": math.Inf(-1), "1e400": math.Inf(1),
		"abc": nan, "1_0": nan, "inf": nan, "NaN": nan, "0x1p3": nan, "-0x10": nan, "0x": nan, "1,2": nan, "1 2": nan, "e3": nan,
	}
	for in, want := range tests {
		got := jsNumberFromString(in)
		if math.IsNaN(want) != math.IsNaN(got) || (!math.IsNaN(want) && got != want) {
			t.Errorf("Number(%q) = %v, want %v", in, got, want)
		}
	}
}
