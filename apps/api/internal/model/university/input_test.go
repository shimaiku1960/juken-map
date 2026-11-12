package university

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	vt "github.com/shimaiku1960/juken-map/apps/api/internal/validation/validationtest"
)

// 期待値は Node（Zod の universityInputSchema・facultyInputSchema・createFacultySchema）に同じ本文を渡して得たもの。

const (
	validUniversity = `{"name":"大学","prefecture":"東京都","type":"国立"}`
	validFaculty    = `{"name":"学部","examDate":"2027-02-01","tagIds":[1,2],"universityId":1}`
)

// with は JSON のオブジェクト base の key を value（JSON）に置き換える。value が "" ならキーを消す。
func with(base, key, value string) string {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(base), &m); err != nil {
		panic(err)
	}
	if value == "" {
		delete(m, key)
	} else {
		m[key] = json.RawMessage(value)
	}
	out, _ := json.Marshal(m)
	return string(out)
}

func TestDecodeUniversity(t *testing.T) {
	tests := []struct{ name, body, want string }{
		{"正しい本文", validUniversity, ""},
		{"本文が配列", `[]`, vt.Bad("Invalid input: expected object, received array", "invalid_type", "")},
		{"大学名なし", `{}`, vt.Bad("Invalid input: expected string, received undefined", "invalid_type", "name")},
		{"大学名が空白だけ（削ってから min）", with(validUniversity, "name", `"  "`), vt.Bad("大学名を入力してください", "too_small", "name")},
		{"大学名が101文字", with(validUniversity, "name", `"`+strings.Repeat("a", 101)+`"`), vt.Bad("大学名は100文字以内で入力してください", "too_big", "name")},
		{"大学名が数", with(validUniversity, "name", `1`), vt.Bad("Invalid input: expected string, received number", "invalid_type", "name")},
		{"都道府県でない", with(validUniversity, "prefecture", `"東京"`), vt.Bad("都道府県を選んでください", "invalid_prefecture", "prefecture")},
		{"都道府県が数", with(validUniversity, "prefecture", `1`), vt.Bad("Invalid input: expected string, received number", "invalid_type", "prefecture")},
		{"種別が違う", with(validUniversity, "type", `"x"`), vt.Bad("種別を選んでください", "invalid_value", "type")},
		{"種別が数", with(validUniversity, "type", `1`), vt.Bad("種別を選んでください", "invalid_value", "type")},
		{"種別なし", with(validUniversity, "type", ""), vt.Bad("種別を選んでください", "invalid_value", "type")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeUniversity(vt.Parse(t, tt.body))
			vt.CheckIssue(t, err, tt.want)
		})
	}

	t.Run("名前の前後の空白を削る。削った後で100文字なら通る", func(t *testing.T) {
		name := strings.Repeat("a", 100)
		got, err := decodeUniversity(vt.Parse(t, with(validUniversity, "name", `" `+name+` "`)))
		if err != nil || got.name != name {
			t.Fatalf("name = %q, err = %v", got.name, err)
		}
	})
}

func TestDecodeFaculty(t *testing.T) {
	seq := func(n int, item func(i int) string) string {
		items := make([]string, n)
		for i := range items {
			items[i] = item(i)
		}
		return "[" + strings.Join(items, ",") + "]"
	}

	tests := []struct{ name, body, want string }{
		{"正しい本文", validFaculty, ""},
		{"学部名が空", with(validFaculty, "name", `""`), vt.Bad("学部名を入力してください", "too_small", "name")},
		{"受験日の形が違う", with(validFaculty, "examDate", `"2027-2-1"`), vt.Bad("受験日を選んでください", "invalid_format", "examDate")},
		{"受験日が空", with(validFaculty, "examDate", `""`), vt.Bad("受験日を選んでください", "invalid_format", "examDate")},
		{"受験日が数", with(validFaculty, "examDate", `1`), vt.Bad("Invalid input: expected string, received number", "invalid_type", "examDate")},
		{"受験日の日が32", with(validFaculty, "examDate", `"2027-02-32"`), vt.Bad("受験日を選んでください", "invalid_exam_date", "examDate")},
		{"受験日の月が13", with(validFaculty, "examDate", `"2027-13-01"`), vt.Bad("受験日を選んでください", "invalid_exam_date", "examDate")},
		{"受験日の月が0", with(validFaculty, "examDate", `"2027-00-10"`), vt.Bad("受験日を選んでください", "invalid_exam_date", "examDate")},
		{"受験日の日が0", with(validFaculty, "examDate", `"2027-01-00"`), vt.Bad("受験日を選んでください", "invalid_exam_date", "examDate")},
		{"タグが文字列", with(validFaculty, "tagIds", `"1"`), vt.Bad("Invalid input: expected array, received string", "invalid_type", "tagIds")},
		{"タグなし", with(validFaculty, "tagIds", ""), vt.Bad("Invalid input: expected array, received undefined", "invalid_type", "tagIds")},
		{"タグが重なる", with(validFaculty, "tagIds", `[1,1]`), vt.Bad("同じタグが重なっています", "duplicate_tags", "tagIds")},
		{"タグが0", with(validFaculty, "tagIds", `[0]`), vt.Bad("Too small: expected number to be >0", "too_small", "tagIds.0")},
		{"タグが文字列の要素", with(validFaculty, "tagIds", `["1"]`), vt.Bad("Invalid input: expected number, received string", "invalid_type", "tagIds.0")},
		{"タグが小数", with(validFaculty, "tagIds", `[1.5]`), vt.Bad("Invalid input: expected int, received number", "invalid_type", "tagIds.0")},
		{"タグが21個", with(validFaculty, "tagIds", seq(21, func(i int) string { return strconv.Itoa(i + 1) })), vt.Bad("タグは20個までです", "too_big", "tagIds")},
		{"タグが21個で先頭が0（要素が先）", with(validFaculty, "tagIds", seq(22, strconv.Itoa)), vt.Bad("Too small: expected number to be >0", "too_small", "tagIds.0")},
		{"タグが21個で全部同じ（個数が重なりより先）", with(validFaculty, "tagIds", seq(21, func(int) string { return "1" })), vt.Bad("タグは20個までです", "too_big", "tagIds")},
		{"タグが重なり、3つ目が0（要素が先）", with(validFaculty, "tagIds", `[1,1,0]`), vt.Bad("Too small: expected number to be >0", "too_small", "tagIds.2")},
		{"大学が文字列", with(validFaculty, "universityId", `"1"`), vt.Bad("Invalid input: expected number, received string", "invalid_type", "universityId")},
		{"大学が0", with(validFaculty, "universityId", `0`), vt.Bad("Too small: expected number to be >0", "too_small", "universityId")},
		{"大学なし", with(validFaculty, "universityId", ""), vt.Bad("Invalid input: expected number, received undefined", "invalid_type", "universityId")},
		{"名前と大学の両方が不正なら名前（大学は最後）", with(with(validFaculty, "name", `""`), "universityId", `0`), vt.Bad("学部名を入力してください", "too_small", "name")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeFaculty(vt.Parse(t, tt.body), true)
			vt.CheckIssue(t, err, tt.want)
		})
	}

	t.Run("受験日は JavaScript の new Date と同じく繰り上げ、その日の UTC 0時にする", func(t *testing.T) {
		for in, want := range map[string]string{
			"2027-02-01": "2027-02-01", "2027-02-29": "2027-03-01", "2027-02-31": "2027-03-03", "2027-04-31": "2027-05-01",
		} {
			got, err := decodeFaculty(vt.Parse(t, with(validFaculty, "examDate", `"`+in+`"`)), true)
			if err != nil {
				t.Fatalf("%s: %v", in, err)
			}
			wantDate, _ := time.Parse(time.DateOnly, want)
			if !got.examDate.Equal(wantDate) || got.examDate.Location() != time.UTC {
				t.Errorf("%s → %v, want %s の UTC 0時", in, got.examDate, want)
			}
		}
	})

	t.Run("書き換えは universityId を読まない。タグなしも通る", func(t *testing.T) {
		got, err := decodeFaculty(vt.Parse(t, `{"name":"学部","examDate":"2027-02-01","tagIds":[],"universityId":"x"}`), false)
		if err != nil || got.universityID != 0 || len(got.tagIDs) != 0 {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
	})
}

// TestListQuery は一覧の q・page。形が違えば DB を引く前に断る（db は nil で渡す）。
func TestListQuery(t *testing.T) {
	for name, tt := range map[string]struct {
		query map[string][]string
		want  string
	}{
		"q が101文字":        {map[string][]string{"q": {strings.Repeat("a", 101)}}, vt.Bad("Too big: expected string to have <=100 characters", "too_big", "q")},
		"q が2つ（page より先）": {map[string][]string{"q": {"a", "b"}, "page": {"x"}}, vt.Bad("Invalid input: expected string, received array", "invalid_type", "q")},
		"page が1001":      {map[string][]string{"page": {"1001"}}, vt.Bad("Too big: expected number to be <=1000", "too_big", "page")},
		"page が0":         {map[string][]string{"page": {"0"}}, vt.Bad("Too small: expected number to be >0", "too_small", "page")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := List(context.Background(), nil, tt.query)
			vt.CheckIssue(t, err, tt.want)
		})
	}
}
