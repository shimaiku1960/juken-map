package validation

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// PageQuery は一覧の z.coerce.number().int().positive().max(max).default(1) の page を読む。
func PageQuery(query map[string][]string, max float64) (int, *Issue) {
	values, ok := query["page"]
	if !ok {
		return 1, nil
	}
	// z.coerce.number() は Number(値)。配列は "1,2" のように , でつないだ文字列として数に直る（["3"] は 3）。
	f := jsNumberFromString(strings.Join(values, ","))
	if math.IsNaN(f) {
		return 0, &Issue{Code: "invalid_type", Field: "page", Message: "Invalid input: expected number, received NaN"}
	}
	n, issue := CheckNumber("page", jsonNumberOf(f), NumberRule{Int: true, Positive: true, Max: max})
	if issue != nil {
		return 0, issue
	}
	return int(n), nil
}

// jsNumberFromString は JavaScript の Number(文字列) と同じ規則で数にする。読めなければ NaN。
//   - 前後の空白を削り、空なら 0
//   - Infinity（符号つきも）
//   - 0x・0o・0b で始まる整数（符号は付けられない）
//   - 10進の数（1e3・.5・5. も可）。Go の ParseFloat だけが読む書き方（1_0・inf・0x1p3）は NaN
func jsNumberFromString(s string) float64 {
	s = JSTrim(s)
	switch s {
	case "":
		return 0
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(s) > 2 && s[0] == '0' {
		base := map[byte]int{'x': 16, 'X': 16, 'o': 8, 'O': 8, 'b': 2, 'B': 2}[s[1]]
		if base != 0 {
			n, ok := new(big.Int).SetString(s[2:], base)
			// SetString は "_" や符号も受け付けるので、数字だけかを先に見る。
			if !ok || strings.ContainsAny(s[2:], "_+-") {
				return math.NaN()
			}
			f, _ := new(big.Float).SetInt(n).Float64()
			return f
		}
	}
	if !jsDecimalPattern.MatchString(s) {
		return math.NaN()
	}
	// 範囲外は ±Inf（ErrRange）になり、Number() と同じ。
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

var jsDecimalPattern = regexp.MustCompile(`^[+-]?(\d+\.?\d*|\.\d+)([eE][+-]?\d+)?$`)

// jsonNumberOf は数を CheckNumber に渡せる形（JSON から読んだ数）にする。
func jsonNumberOf(f float64) any {
	if math.IsInf(f, 0) {
		// CheckNumber は JSON から読んだ ±Infinity を json.Number の "±Inf" で受け取る（jsNumber が ParseFloat で読む）。
		if f > 0 {
			return json.Number("+Inf")
		}
		return json.Number("-Inf")
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
}
