package httpx

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/opt"
)

// 書き込みの API の入力チェック（JUK-75）。
//
// 規則の正は src/shared/validations/ の Zod のスキーマで、画面のフォームと Node が使う。Go はそれと
// 同じ規則をここで手で書く（契約の OpenAPI には形だけを書き、規則は2か所に持つと決めた。
// 項目をまたぐ規則や「今日より未来は不可」はスキーマに書けないため）。
// Zod の規則を変えたら、ここと各 *_writes_test.go のケースも直す（Node と応答を比べるテストは JUK-84 で消した）。
//
// Node は Zod の issue のうち最初の1件だけを返す（routes/validation-error.ts）。Zod はスキーマに
// 書いた項目の順に、項目の中では書いたチェックの順に issue を積むので、ここでも同じ順に確かめ、
// 最初に見つかった1件で止める。

// ValidationIssue は弾いた理由の1件。field が "" なら本文そのもの（応答の field は null）。
type ValidationIssue struct {
	Code, Field, Message string
}

func (v *ValidationIssue) Write(w http.ResponseWriter) {
	body := apischema.ValidationError{Error: v.Message, Code: v.Code}
	if v.Field != "" {
		body.Field = &v.Field
	}
	WriteJSON(w, http.StatusBadRequest, body)
}

// ObjectInput は本文のオブジェクトを項目ごとに読む。最初の issue を覚え、それより後の読み取りは
// 何もせずゼロ値を返す。使い方：
//
//	in := ReadObject(body)
//	p := NotificationPreferenceInput{EmailMorningEnabled: in.boolean("emailMorningEnabled"), …}
//	if in.Reject(w) { return }
type ObjectInput struct {
	fields map[string]any
	Issue  *ValidationIssue
	// path は入れ子のオブジェクトの場所（"items.0"）。issue の field はこれに項目名をつないだもの。
	path string
}

// ReadObject は z.object({…}) の入口。本文がオブジェクトでなければ、それが最初の issue になる。
func ReadObject(body any) *ObjectInput {
	return ReadObjectAt(body, "")
}

// ReadObjectAt は入れ子のオブジェクト（配列の要素など）を読む。path は "items.0" のような場所。
func ReadObjectAt(v any, path string) *ObjectInput {
	m, ok := v.(map[string]any)
	if !ok {
		return &ObjectInput{Issue: InvalidType(path, "object", v), path: path}
	}
	return &ObjectInput{fields: m, path: path}
}

// Field は項目名を、Zod の issue の path と同じ "items.0.content" の形にする。
func (in *ObjectInput) Field(key string) string {
	if in.path == "" {
		return key
	}
	return in.path + "." + key
}

// Reject は issue があれば 400 を送って true を返す。
func (in *ObjectInput) Reject(w http.ResponseWriter) bool {
	if in.Issue == nil {
		return false
	}
	in.Issue.Write(w)
	return true
}

func (in *ObjectInput) Value(key string) (any, bool) {
	if in.Issue != nil {
		return nil, false
	}
	v, ok := in.fields[key]
	if !ok {
		return JSUndefined{}, true
	}
	return v, true
}

// Boolean は z.boolean()。
func (in *ObjectInput) Boolean(key string) bool {
	v, ok := in.Value(key)
	if !ok {
		return false
	}
	b, isBool := v.(bool)
	if !isBool {
		in.Issue = InvalidType(in.Field(key), "boolean", v)
	}
	return b
}

// OptionalBool は z.boolean().optional()。
func (in *ObjectInput) OptionalBool(key string) Optional[bool] {
	return readOptional(in, key, false, func(field string, v any) (bool, *ValidationIssue) {
		b, isBool := v.(bool)
		if !isBool {
			return false, InvalidType(field, "boolean", v)
		}
		return b, nil
	})
}

// Array は z.array(…).min(1, minMessage)。minMessage が空なら min は無い（z.array(…) だけ）。
// 要素は呼び出し側が ReadObjectAt(element, in.Field(key)+".0") などで読む。
func (in *ObjectInput) Array(key, minMessage string) []any {
	v, ok := in.Value(key)
	if !ok {
		return nil
	}
	a, isArray := v.([]any)
	switch {
	case !isArray:
		in.Issue = InvalidType(in.Field(key), "array", v)
	case len(a) == 0 && minMessage != "":
		in.Issue = &ValidationIssue{Code: "too_small", Field: in.Field(key), Message: minMessage}
	}
	return a
}

// Take は入れ子のオブジェクトで見つかった issue を、外側の最初の issue にする。
func (in *ObjectInput) Take(inner *ObjectInput) {
	if in.Issue == nil {
		in.Issue = inner.Issue
	}
}

// StringRule は z.string() に続けて書いたチェック。Zod と同じ順（型 → min → max → checks → trim）に確かめる。
type StringRule struct {
	// TypeMessage は z.string({ message }) の文言。空なら Zod の既定の文言。
	TypeMessage string
	Min         int
	MinMessage  string
	Max         int
	MaxMessage  string
	// Checks は min・max の後に書いた .regex()・.refine()。書いた順に確かめる。
	Checks []StringCheck
	// Trim は .trim()。max より後に書いているスキーマは、削る前の長さで max を確かめる（Zod と同じ）。
	Trim bool
	// TrimFirst は z.string().trim().min(…) のように .trim() を先に書いたスキーマ。Zod はチェックを書いた順に
	// 流すので、min・max・checks は削った後の文字列で確かめる（"  " は min(1) に引っかかる）。
	TrimFirst bool
}

// StringCheck は .regex()・.refine() の1つ。ok が false なら code と message の issue になる。
// Code は .regex() なら "invalid_format"、.refine() なら params で付けた名前。
type StringCheck struct {
	OK            func(string) bool
	Code, Message string
}

// String は必須の z.string() と StringRule のチェック。
func (in *ObjectInput) String(key string, rule StringRule) string {
	v, ok := in.Value(key)
	if !ok {
		return ""
	}
	s, issue := CheckString(in.Field(key), v, rule)
	in.Issue = issue
	return s
}

func CheckString(key string, v any, rule StringRule) (string, *ValidationIssue) {
	s, isString := v.(string)
	if !isString {
		issue := InvalidType(key, "string", v)
		if rule.TypeMessage != "" {
			issue.Message = rule.TypeMessage
		}
		return "", issue
	}
	if rule.TrimFirst {
		s = JSTrim(s)
	}
	n := CodePointLength(s)
	switch {
	case rule.Min > 0 && n < rule.Min:
		return "", &ValidationIssue{Code: "too_small", Field: key, Message: rule.MinMessage}
	case rule.Max > 0 && n > rule.Max:
		return "", &ValidationIssue{Code: "too_big", Field: key, Message: rule.MaxMessage}
	}
	for _, c := range rule.Checks {
		if !c.OK(s) {
			return "", &ValidationIssue{Code: c.Code, Field: key, Message: c.Message}
		}
	}
	if rule.Trim {
		s = JSTrim(s)
	}
	return s, nil
}

// NumberRule は z.number() に続けて書いたチェック。Zod と同じ順（型 → int → positive → max）に確かめる。
// 文言が空のものは Zod の既定の文言になる。
type NumberRule struct {
	TypeMessage string // z.number({ message })
	// Int は .int()。小数は invalid_type、安全な整数の範囲（±2^53-1）の外は too_big・too_small。
	Int        bool
	IntMessage string
	// Positive は .positive()（0 より大きい）。
	Positive        bool
	PositiveMessage string
	// Max は .max(n)。0 なら無し。
	Max        float64
	MaxMessage string
}

// PositiveIntRule は z.number().int().positive()（文言は Zod の既定）。ID を受ける項目で使う。
var PositiveIntRule = NumberRule{Int: true, Positive: true}

// Number は必須の z.number() と NumberRule のチェック。
func (in *ObjectInput) Number(key string, rule NumberRule) float64 {
	v, ok := in.Value(key)
	if !ok {
		return 0
	}
	f, issue := CheckNumber(in.Field(key), v, rule)
	in.Issue = issue
	return f
}

func CheckNumber(key string, v any, rule NumberRule) (float64, *ValidationIssue) {
	or := func(message, fallback string) string {
		if message != "" {
			return message
		}
		return fallback
	}
	n, isNumber := v.(json.Number)
	if !isNumber {
		issue := InvalidType(key, "number", v)
		issue.Message = or(rule.TypeMessage, issue.Message)
		return 0, issue
	}
	f := jsNumber(n)
	// JSON.parse は範囲外の数を ±Infinity にする。Zod はそれを数として受け付けない。
	if math.IsInf(f, 0) {
		received := "Infinity"
		if f < 0 {
			received = "-Infinity"
		}
		return 0, &ValidationIssue{Code: "invalid_type", Field: key,
			Message: or(rule.TypeMessage, "Invalid input: expected number, received "+received)}
	}
	if rule.Int {
		switch {
		case f != math.Trunc(f):
			return 0, &ValidationIssue{Code: "invalid_type", Field: key,
				Message: or(rule.IntMessage, "Invalid input: expected int, received number")}
		case f > MaxSafeInteger:
			return 0, &ValidationIssue{Code: "too_big", Field: key,
				Message: or(rule.IntMessage, "Too big: expected int to be <=9007199254740991")}
		case f < -MaxSafeInteger:
			return 0, &ValidationIssue{Code: "too_small", Field: key,
				Message: or(rule.IntMessage, "Too small: expected int to be >=-9007199254740991")}
		}
	}
	if rule.Positive && !(f > 0) {
		return 0, &ValidationIssue{Code: "too_small", Field: key,
			Message: or(rule.PositiveMessage, "Too small: expected number to be >0")}
	}
	if rule.Max != 0 && f > rule.Max {
		return 0, &ValidationIssue{Code: "too_big", Field: key,
			Message: or(rule.MaxMessage, fmt.Sprintf("Too big: expected number to be <=%v", rule.Max))}
	}
	return f, nil
}

// Optional は .optional()（と .nullable()）の付いた項目の値。Zod と同じく、キーが無い（undefined）・null・値を区別する。
// Node の `data.x ?? null` は ptr()、`data.x !== current` は differs() にあたる。
type Optional[T comparable] struct {
	Present bool // キーがある（null を含む）
	Value   *T   // null かキーが無いなら nil
}

// Ptr は DB に書く値。キーが無いときも null として書く（Node の ?? null）。
func (o Optional[T]) Ptr() *T {
	return o.Value
}

// IsNull は Node の `x == null`（undefined と null のどちらも true）。
func (o Optional[T]) IsNull() bool {
	return o.Value == nil
}

// Field は持ち主（internal/write）の操作に渡す形にする。
func (o Optional[T]) Field() opt.Field[T] {
	return opt.Field[T]{Present: o.Present, Value: o.Value}
}

// readOptional は .optional() の付いた項目を読む。nullable なら null も受け付ける（.nullable()）。
// check は値があるときのチェック（CheckString・CheckNumber など）。
func readOptional[T comparable](in *ObjectInput, key string, nullable bool, check func(key string, v any) (T, *ValidationIssue)) Optional[T] {
	v, ok := in.Value(key)
	if !ok {
		return Optional[T]{}
	}
	switch {
	case v == JSUndefined{}:
		return Optional[T]{}
	case v == nil && nullable:
		return Optional[T]{Present: true}
	}
	value, issue := check(in.Field(key), v)
	if issue != nil {
		in.Issue = issue
		return Optional[T]{}
	}
	return Optional[T]{Present: true, Value: &value}
}

// OptionalString は z.string()….optional()（nullable なら .nullable().optional()）。
func (in *ObjectInput) OptionalString(key string, rule StringRule, nullable bool) Optional[string] {
	return readOptional(in, key, nullable, func(key string, v any) (string, *ValidationIssue) {
		return CheckString(key, v, rule)
	})
}

// OptionalInt は z.number().int()….nullable().optional() のような整数の項目。
func (in *ObjectInput) OptionalInt(key string, rule NumberRule, nullable bool) Optional[int64] {
	rule.Int = true
	return readOptional(in, key, nullable, func(key string, v any) (int64, *ValidationIssue) {
		f, issue := CheckNumber(key, v, rule)
		return int64(f), issue
	})
}

// Enum は z.enum(VALUES, { error: message })。文字列でない値・キーが無いときも、型の issue ではなく
// invalid_value の message になる（Zod 4 の z.enum は型と値をまとめて1つの issue にする）。
func (in *ObjectInput) Enum(key string, valid func(string) bool, message string) string {
	v, ok := in.Value(key)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString || !valid(s) {
		in.Issue = &ValidationIssue{Code: "invalid_value", Field: in.Field(key), Message: message}
		return ""
	}
	return s
}

// OneOf は .refine((v) => VALUES.includes(v)) のチェック。
func OneOf(values []string, code, message string) StringCheck {
	return StringCheck{OK: func(s string) bool { return slices.Contains(values, s) }, Code: code, Message: message}
}

// OptionalEnum は文言を指定しない z.enum(values).optional()（nullable なら .nullable().optional()）。
// .nullable() の無い項目に null を送ると、型の issue ではなく invalid_value になる（Zod 4 の z.enum）。
func (in *ObjectInput) OptionalEnum(key string, values []string, nullable bool) Optional[string] {
	return readOptional(in, key, nullable, func(field string, v any) (string, *ValidationIssue) {
		return CheckEnum(field, v, values)
	})
}

func CheckEnum(field string, v any, values []string) (string, *ValidationIssue) {
	s, isString := v.(string)
	if !isString || !slices.Contains(values, s) {
		return "", &ValidationIssue{Code: "invalid_value", Field: field, Message: enumMessage(values)}
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

// ISODateCheck は z.iso.date()：YYYY-MM-DD の形で、暦にある日付（グレゴリオ暦のうるう年）。
// Zod は1つの正規表現で確かめるので、形の誤りも暦に無い日付も同じ invalid_format の1件になる。
var ISODateCheck = StringCheck{
	OK:      func(s string) bool { return YMDPattern.MatchString(s) && IsCalendarYMD(s) },
	Code:    "invalid_format",
	Message: "Invalid ISO date",
}

// AddIssue は項目の読み取りの後に、superRefine の ctx.addIssue にあたる issue を足す。
// すでに issue があれば何もしない（最初の1件だけを返すので）。
func (in *ObjectInput) AddIssue(code, field, message string) {
	if in.Issue == nil {
		in.Issue = &ValidationIssue{Code: code, Field: in.Field(field), Message: message}
	}
}

// InvalidType は Zod の invalid_type の issue。文言は Zod（en）の既定と同じ。
func InvalidType(field, expected string, got any) *ValidationIssue {
	return &ValidationIssue{
		Code:    "invalid_type",
		Field:   field,
		Message: fmt.Sprintf("Invalid input: expected %s, received %s", expected, jsTypeName(got)),
	}
}

// jsTypeName は Zod の parsedType と同じ名前を返す（JSON から来る値だけを考えればよい）。
func jsTypeName(v any) string {
	switch v.(type) {
	case JSUndefined:
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

// CodePointLength は Zod 4.5 の min・max と同じく、文字列をコードポイントの数で数える。
// JavaScript の String#length（UTF-16 の単位。絵文字は 2）ではない（zod の util.codePointLength）。
// JSON から読んだ単独のサロゲートは Go では U+FFFD になるが、Zod もそれを1と数えるので数は同じ。
func CodePointLength(s string) int {
	return utf8.RuneCountInString(s)
}

// JSTrim は String#trim と同じ文字を前後から削る。
// strings.TrimSpace とは対象が違う（JavaScript は U+FEFF を削り、U+0085 は削らない）。
// 全角スペース（U+3000）はどちらも削る。
func JSTrim(s string) string {
	return strings.TrimFunc(s, IsJSWhitespace)
}

// IsJSWhitespace は ECMAScript の WhiteSpace と LineTerminator。
func IsJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0x00A0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// YMDDateRule は Zod の ymdDate（src/shared/validations/studyPlan.ts）：min(1) の後に
// 「YYYY-MM-DD の形」と「暦にある日付」を確かめる。minMessage が空なら Zod の既定の文言。
// extra はその後ろに続けて書いた .refine()（実績の「未来日は不可」など）。
func YMDDateRule(minMessage string, extra ...StringCheck) StringRule {
	if minMessage == "" {
		minMessage = "Too small: expected string to have >=1 characters"
	}
	return StringRule{
		Min:        1,
		MinMessage: minMessage,
		Checks: append([]StringCheck{
			{OK: YMDPattern.MatchString, Code: "invalid_format", Message: YMDMessage},
			{OK: func(s string) bool { return !YMDPattern.MatchString(s) || IsCalendarYMD(s) }, Code: "invalid_date", Message: "存在しない日付です"},
		}, extra...),
	}
}

// YMDPattern は Node の ymdField（z.string().regex(/^\d{4}-\d{2}-\d{2}$/)）と同じ形。
// 形だけを見て、13月や2月30日は通す（dates.ParseYMD で扱う）。
var YMDPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

const YMDMessage = "日付は YYYY-MM-DD で指定してください"

// IsCalendarYMD は "YYYY-MM-DD"（形は確かめ済み）が暦にある日付か。2026-02-30・2026-13-01 は false。
// Node の isCalendarYmd（src/shared/date.ts）と同じ算数で判定する。time.Date は範囲外の日を翌月へ
// 繰り越してしまうので使わない。
func IsCalendarYMD(s string) bool {
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

// MaxSafeInteger は JavaScript の Number.MAX_SAFE_INTEGER。Zod の int() はこれを超える数を弾く。
const MaxSafeInteger = 1<<53 - 1
