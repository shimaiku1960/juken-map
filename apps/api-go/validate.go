package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// 書き込みの API の入力チェック（JUK-75）。
//
// 規則の正は src/shared/validations/ の Zod のスキーマで、画面のフォームと Node が使う。Go はそれと
// 同じ規則をここで手で書く（契約の OpenAPI には形だけを書き、規則は2か所に持つと決めた。
// 項目をまたぐ規則や「今日より未来は不可」はスキーマに書けないため）。
// ずれは応答一致テスト（parity_test.go）に不正な入力を並べて見つける。
//
// Node は Zod の issue のうち最初の1件だけを返す（routes/validation-error.ts）。Zod はスキーマに
// 書いた項目の順に、項目の中では書いたチェックの順に issue を積むので、ここでも同じ順に確かめ、
// 最初に見つかった1件で止める。

// validationIssue は弾いた理由の1件。field が "" なら本文そのもの（応答の field は null）。
type validationIssue struct {
	code, field, message string
}

func (v *validationIssue) write(w http.ResponseWriter) {
	body := ValidationError{Error: v.message, Code: v.code}
	if v.field != "" {
		body.Field = &v.field
	}
	writeJSON(w, http.StatusBadRequest, body)
}

// objectInput は本文のオブジェクトを項目ごとに読む。最初の issue を覚え、それより後の読み取りは
// 何もせずゼロ値を返す。使い方：
//
//	in := readObject(body)
//	p := NotificationPreferenceInput{EmailMorningEnabled: in.boolean("emailMorningEnabled"), …}
//	if in.reject(w) { return }
type objectInput struct {
	fields map[string]any
	issue  *validationIssue
	// path は入れ子のオブジェクトの場所（"items.0"）。issue の field はこれに項目名をつないだもの。
	path string
}

// readObject は z.object({…}) の入口。本文がオブジェクトでなければ、それが最初の issue になる。
func readObject(body any) *objectInput {
	return readObjectAt(body, "")
}

// readObjectAt は入れ子のオブジェクト（配列の要素など）を読む。path は "items.0" のような場所。
func readObjectAt(v any, path string) *objectInput {
	m, ok := v.(map[string]any)
	if !ok {
		return &objectInput{issue: invalidType(path, "object", v), path: path}
	}
	return &objectInput{fields: m, path: path}
}

// field は項目名を、Zod の issue の path と同じ "items.0.content" の形にする。
func (in *objectInput) field(key string) string {
	if in.path == "" {
		return key
	}
	return in.path + "." + key
}

// reject は issue があれば 400 を送って true を返す。
func (in *objectInput) reject(w http.ResponseWriter) bool {
	if in.issue == nil {
		return false
	}
	in.issue.write(w)
	return true
}

func (in *objectInput) value(key string) (any, bool) {
	if in.issue != nil {
		return nil, false
	}
	v, ok := in.fields[key]
	if !ok {
		return jsUndefined{}, true
	}
	return v, true
}

// boolean は z.boolean()。
func (in *objectInput) boolean(key string) bool {
	v, ok := in.value(key)
	if !ok {
		return false
	}
	b, isBool := v.(bool)
	if !isBool {
		in.issue = invalidType(in.field(key), "boolean", v)
	}
	return b
}

// optionalBool は z.boolean().optional()。
func (in *objectInput) optionalBool(key string) optional[bool] {
	return readOptional(in, key, false, func(field string, v any) (bool, *validationIssue) {
		b, isBool := v.(bool)
		if !isBool {
			return false, invalidType(field, "boolean", v)
		}
		return b, nil
	})
}

// array は z.array(…).min(1, minMessage)。minMessage が空なら min は無い（z.array(…) だけ）。
// 要素は呼び出し側が readObjectAt(element, in.field(key)+".0") などで読む。
func (in *objectInput) array(key, minMessage string) []any {
	v, ok := in.value(key)
	if !ok {
		return nil
	}
	a, isArray := v.([]any)
	switch {
	case !isArray:
		in.issue = invalidType(in.field(key), "array", v)
	case len(a) == 0 && minMessage != "":
		in.issue = &validationIssue{code: "too_small", field: in.field(key), message: minMessage}
	}
	return a
}

// take は入れ子のオブジェクトで見つかった issue を、外側の最初の issue にする。
func (in *objectInput) take(inner *objectInput) {
	if in.issue == nil {
		in.issue = inner.issue
	}
}

// stringRule は z.string() に続けて書いたチェック。Zod と同じ順（型 → min → max → checks → trim）に確かめる。
type stringRule struct {
	// typeMessage は z.string({ message }) の文言。空なら Zod の既定の文言。
	typeMessage string
	min         int
	minMessage  string
	max         int
	maxMessage  string
	// checks は min・max の後に書いた .regex()・.refine()。書いた順に確かめる。
	checks []stringCheck
	// trim は .trim()。max より後に書いているスキーマは、削る前の長さで max を確かめる（Zod と同じ）。
	trim bool
	// trimFirst は z.string().trim().min(…) のように .trim() を先に書いたスキーマ。Zod はチェックを書いた順に
	// 流すので、min・max・checks は削った後の文字列で確かめる（"  " は min(1) に引っかかる）。
	trimFirst bool
}

// stringCheck は .regex()・.refine() の1つ。ok が false なら code と message の issue になる。
// code は .regex() なら "invalid_format"、.refine() なら params で付けた名前。
type stringCheck struct {
	ok            func(string) bool
	code, message string
}

// string は必須の z.string() と stringRule のチェック。
func (in *objectInput) string(key string, rule stringRule) string {
	v, ok := in.value(key)
	if !ok {
		return ""
	}
	s, issue := checkString(in.field(key), v, rule)
	in.issue = issue
	return s
}

func checkString(key string, v any, rule stringRule) (string, *validationIssue) {
	s, isString := v.(string)
	if !isString {
		issue := invalidType(key, "string", v)
		if rule.typeMessage != "" {
			issue.message = rule.typeMessage
		}
		return "", issue
	}
	if rule.trimFirst {
		s = jsTrim(s)
	}
	n := codePointLength(s)
	switch {
	case rule.min > 0 && n < rule.min:
		return "", &validationIssue{code: "too_small", field: key, message: rule.minMessage}
	case rule.max > 0 && n > rule.max:
		return "", &validationIssue{code: "too_big", field: key, message: rule.maxMessage}
	}
	for _, c := range rule.checks {
		if !c.ok(s) {
			return "", &validationIssue{code: c.code, field: key, message: c.message}
		}
	}
	if rule.trim {
		s = jsTrim(s)
	}
	return s, nil
}

// numberRule は z.number() に続けて書いたチェック。Zod と同じ順（型 → int → positive → max）に確かめる。
// 文言が空のものは Zod の既定の文言になる。
type numberRule struct {
	typeMessage string // z.number({ message })
	// int は .int()。小数は invalid_type、安全な整数の範囲（±2^53-1）の外は too_big・too_small。
	int        bool
	intMessage string
	// positive は .positive()（0 より大きい）。
	positive        bool
	positiveMessage string
	// max は .max(n)。0 なら無し。
	max        float64
	maxMessage string
}

// number は必須の z.number() と numberRule のチェック。
func (in *objectInput) number(key string, rule numberRule) float64 {
	v, ok := in.value(key)
	if !ok {
		return 0
	}
	f, issue := checkNumber(in.field(key), v, rule)
	in.issue = issue
	return f
}

func checkNumber(key string, v any, rule numberRule) (float64, *validationIssue) {
	or := func(message, fallback string) string {
		if message != "" {
			return message
		}
		return fallback
	}
	n, isNumber := v.(json.Number)
	if !isNumber {
		issue := invalidType(key, "number", v)
		issue.message = or(rule.typeMessage, issue.message)
		return 0, issue
	}
	f := jsNumber(n)
	// JSON.parse は範囲外の数を ±Infinity にする。Zod はそれを数として受け付けない。
	if math.IsInf(f, 0) {
		received := "Infinity"
		if f < 0 {
			received = "-Infinity"
		}
		return 0, &validationIssue{code: "invalid_type", field: key,
			message: or(rule.typeMessage, "Invalid input: expected number, received "+received)}
	}
	if rule.int {
		switch {
		case f != math.Trunc(f):
			return 0, &validationIssue{code: "invalid_type", field: key,
				message: or(rule.intMessage, "Invalid input: expected int, received number")}
		case f > maxSafeInteger:
			return 0, &validationIssue{code: "too_big", field: key,
				message: or(rule.intMessage, "Too big: expected int to be <=9007199254740991")}
		case f < -maxSafeInteger:
			return 0, &validationIssue{code: "too_small", field: key,
				message: or(rule.intMessage, "Too small: expected int to be >=-9007199254740991")}
		}
	}
	if rule.positive && !(f > 0) {
		return 0, &validationIssue{code: "too_small", field: key,
			message: or(rule.positiveMessage, "Too small: expected number to be >0")}
	}
	if rule.max != 0 && f > rule.max {
		return 0, &validationIssue{code: "too_big", field: key,
			message: or(rule.maxMessage, fmt.Sprintf("Too big: expected number to be <=%v", rule.max))}
	}
	return f, nil
}

// optional は .optional()（と .nullable()）の付いた項目の値。Zod と同じく、キーが無い（undefined）・null・値を区別する。
// Node の `data.x ?? null` は ptr()、`data.x !== current` は differs() にあたる。
type optional[T comparable] struct {
	present bool // キーがある（null を含む）
	value   *T   // null かキーが無いなら nil
}

// ptr は DB に書く値。キーが無いときも null として書く（Node の ?? null）。
func (o optional[T]) ptr() *T {
	return o.value
}

// isNull は Node の `x == null`（undefined と null のどちらも true）。
func (o optional[T]) isNull() bool {
	return o.value == nil
}

// differs は Node の `data.x !== current`（current は DB の値で、null か値）。
// キーが無い（undefined）ときは、どんな値とも違う（undefined !== null も true）。
func (o optional[T]) differs(current *T) bool {
	switch {
	case !o.present:
		return true
	case o.value == nil || current == nil:
		return o.value != current
	}
	return *o.value != *current
}

// readOptional は .optional() の付いた項目を読む。nullable なら null も受け付ける（.nullable()）。
// check は値があるときのチェック（checkString・checkNumber など）。
func readOptional[T comparable](in *objectInput, key string, nullable bool, check func(key string, v any) (T, *validationIssue)) optional[T] {
	v, ok := in.value(key)
	if !ok {
		return optional[T]{}
	}
	switch {
	case v == jsUndefined{}:
		return optional[T]{}
	case v == nil && nullable:
		return optional[T]{present: true}
	}
	value, issue := check(in.field(key), v)
	if issue != nil {
		in.issue = issue
		return optional[T]{}
	}
	return optional[T]{present: true, value: &value}
}

// optionalString は z.string()….optional()（nullable なら .nullable().optional()）。
func (in *objectInput) optionalString(key string, rule stringRule, nullable bool) optional[string] {
	return readOptional(in, key, nullable, func(key string, v any) (string, *validationIssue) {
		return checkString(key, v, rule)
	})
}

// optionalInt は z.number().int()….nullable().optional() のような整数の項目。
func (in *objectInput) optionalInt(key string, rule numberRule, nullable bool) optional[int64] {
	rule.int = true
	return readOptional(in, key, nullable, func(key string, v any) (int64, *validationIssue) {
		f, issue := checkNumber(key, v, rule)
		return int64(f), issue
	})
}

// enum は z.enum(VALUES, { error: message })。文字列でない値・キーが無いときも、型の issue ではなく
// invalid_value の message になる（Zod 4 の z.enum は型と値をまとめて1つの issue にする）。
func (in *objectInput) enum(key string, valid func(string) bool, message string) string {
	v, ok := in.value(key)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString || !valid(s) {
		in.issue = &validationIssue{code: "invalid_value", field: in.field(key), message: message}
		return ""
	}
	return s
}

// oneOf は .refine((v) => VALUES.includes(v)) のチェック。
func oneOf(values []string, code, message string) stringCheck {
	return stringCheck{ok: func(s string) bool { return slices.Contains(values, s) }, code: code, message: message}
}

// optionalEnum は文言を指定しない z.enum(values).optional()（nullable なら .nullable().optional()）。
// .nullable() の無い項目に null を送ると、型の issue ではなく invalid_value になる（Zod 4 の z.enum）。
func (in *objectInput) optionalEnum(key string, values []string, nullable bool) optional[string] {
	return readOptional(in, key, nullable, func(field string, v any) (string, *validationIssue) {
		return checkEnum(field, v, values)
	})
}

func checkEnum(field string, v any, values []string) (string, *validationIssue) {
	s, isString := v.(string)
	if !isString || !slices.Contains(values, s) {
		return "", &validationIssue{code: "invalid_value", field: field, message: enumMessage(values)}
	}
	return s, nil
}

// enumMessage は z.enum の既定の文言（Invalid option: expected one of "a"|"b"）。値は Zod のスキーマに書いた順。
func enumMessage(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = `"` + v + `"`
	}
	return "Invalid option: expected one of " + strings.Join(quoted, "|")
}

// isoDateCheck は z.iso.date()：YYYY-MM-DD の形で、暦にある日付（グレゴリオ暦のうるう年）。
// Zod は1つの正規表現で確かめるので、形の誤りも暦に無い日付も同じ invalid_format の1件になる。
var isoDateCheck = stringCheck{
	ok:      func(s string) bool { return ymdPattern.MatchString(s) && isCalendarYMD(s) },
	code:    "invalid_format",
	message: "Invalid ISO date",
}

// addIssue は項目の読み取りの後に、superRefine の ctx.addIssue にあたる issue を足す。
// すでに issue があれば何もしない（最初の1件だけを返すので）。
func (in *objectInput) addIssue(code, field, message string) {
	if in.issue == nil {
		in.issue = &validationIssue{code: code, field: in.field(field), message: message}
	}
}

// invalidType は Zod の invalid_type の issue。文言は Zod（en）の既定と同じ。
func invalidType(field, expected string, got any) *validationIssue {
	return &validationIssue{
		code:    "invalid_type",
		field:   field,
		message: fmt.Sprintf("Invalid input: expected %s, received %s", expected, jsTypeName(got)),
	}
}

// jsTypeName は Zod の parsedType と同じ名前を返す（JSON から来る値だけを考えればよい）。
func jsTypeName(v any) string {
	switch v.(type) {
	case jsUndefined:
		return "undefined"
	case nil:
		return "null"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}

// codePointLength は Zod 4.5 の min・max と同じく、文字列をコードポイントの数で数える。
// JavaScript の String#length（UTF-16 の単位。絵文字は 2）ではない（zod の util.codePointLength）。
// JSON から読んだ単独のサロゲートは Go では U+FFFD になるが、Zod もそれを1と数えるので数は同じ。
func codePointLength(s string) int {
	return utf8.RuneCountInString(s)
}

// jsTrim は String#trim と同じ文字を前後から削る。
// strings.TrimSpace とは対象が違う（JavaScript は U+FEFF を削り、U+0085 は削らない）。
// 全角スペース（U+3000）はどちらも削る。
func jsTrim(s string) string {
	return strings.TrimFunc(s, isJSWhitespace)
}

// isJSWhitespace は ECMAScript の WhiteSpace と LineTerminator。
func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// ymdDateRule は Zod の ymdDate（src/shared/validations/studyPlan.ts）：min(1) の後に
// 「YYYY-MM-DD の形」と「暦にある日付」を確かめる。minMessage が空なら Zod の既定の文言。
// extra はその後ろに続けて書いた .refine()（実績の「未来日は不可」など）。
func ymdDateRule(minMessage string, extra ...stringCheck) stringRule {
	if minMessage == "" {
		minMessage = "Too small: expected string to have >=1 characters"
	}
	return stringRule{
		min:        1,
		minMessage: minMessage,
		checks: append([]stringCheck{
			{ok: ymdPattern.MatchString, code: "invalid_format", message: ymdMessage},
			{ok: func(s string) bool { return !ymdPattern.MatchString(s) || isCalendarYMD(s) }, code: "invalid_date", message: "存在しない日付です"},
		}, extra...),
	}
}
