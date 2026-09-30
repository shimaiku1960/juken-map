package main

import (
	"fmt"
	"net/http"
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
}

// readObject は z.object({…}) の入口。本文がオブジェクトでなければ、それが最初の issue になる。
func readObject(body any) *objectInput {
	m, ok := body.(map[string]any)
	if !ok {
		return &objectInput{issue: invalidType("", "object", body)}
	}
	return &objectInput{fields: m}
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
		in.issue = invalidType(key, "boolean", v)
	}
	return b
}

// stringRule は z.string() に続けて書いたチェック。Zod と同じ順（型 → min → max → trim）に確かめる。
type stringRule struct {
	// typeMessage は z.string({ message }) の文言。空なら Zod の既定の文言。
	typeMessage string
	min         int
	minMessage  string
	max         int
	maxMessage  string
	// trim は .trim()。max より後に書いているスキーマは、削る前の長さで max を確かめる（Zod と同じ）。
	trim bool
}

// string は z.string() と stringRule のチェック。
func (in *objectInput) string(key string, rule stringRule) string {
	v, ok := in.value(key)
	if !ok {
		return ""
	}
	s, isString := v.(string)
	if !isString {
		in.issue = invalidType(key, "string", v)
		if rule.typeMessage != "" {
			in.issue.message = rule.typeMessage
		}
		return ""
	}
	n := codePointLength(s)
	switch {
	case rule.min > 0 && n < rule.min:
		in.issue = &validationIssue{code: "too_small", field: key, message: rule.minMessage}
		return ""
	case rule.max > 0 && n > rule.max:
		in.issue = &validationIssue{code: "too_big", field: key, message: rule.maxMessage}
		return ""
	}
	if rule.trim {
		s = jsTrim(s)
	}
	return s
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
	switch x := v.(type) {
	case jsUndefined:
		return "undefined"
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x != x {
			return "NaN"
		}
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
