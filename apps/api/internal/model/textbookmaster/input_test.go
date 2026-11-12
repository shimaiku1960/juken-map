package textbookmaster

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	vt "github.com/shimaiku1960/juken-map/apps/api/internal/validation/validationtest"
)

// 期待値は Node（Zod の textbookMasterInputSchema）に同じ本文を渡して得たもの。

const valid = `{"name":"本","publisher":"社","edition":"第2版","isbn":"978-4-00-000000-0","metrics":[{"unit":"page","totalAmount":100,"isDefault":true}]}`

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

func TestDecode(t *testing.T) {
	metric := func(unit, total, isDefault string) string {
		return `{"unit":` + unit + `,"totalAmount":` + total + `,"isDefault":` + isDefault + `}`
	}
	seq := func(n int, item func(i int) string) string {
		items := make([]string, n)
		for i := range items {
			items[i] = item(i)
		}
		return "[" + strings.Join(items, ",") + "]"
	}
	units := []string{`"page"`, `"question"`, `"chapter"`, `"number"`, `"part"`, `"section"`}

	tests := []struct{ name, body, want string }{
		{"正しい本文", valid, ""},
		{"参考書名なし", `{}`, vt.Bad("Invalid input: expected string, received undefined", "invalid_type", "name")},
		{"参考書名が空白だけ", with(valid, "name", `"  "`), vt.Bad("参考書名を入力してください", "too_small", "name")},
		{"参考書名が151文字", with(valid, "name", `"`+strings.Repeat("a", 151)+`"`), vt.Bad("参考書名は150文字以内で入力してください", "too_big", "name")},
		{"出版社が数", with(valid, "publisher", `1`), vt.Bad("Invalid input: expected string, received number", "invalid_type", "publisher")},
		{"出版社が101文字", with(valid, "publisher", `"`+strings.Repeat("a", 101)+`"`), vt.Bad("100文字以内で入力してください", "too_big", "publisher")},
		{"版が51文字", with(valid, "edition", `"`+strings.Repeat("a", 51)+`"`), vt.Bad("50文字以内で入力してください", "too_big", "edition")},
		{"ISBN が3桁", with(valid, "isbn", `"123"`), vt.Bad("ISBN は10桁か13桁で入力してください", "invalid_isbn", "isbn")},
		{"ISBN が空", with(valid, "isbn", `""`), vt.Bad("ISBN は10桁か13桁で入力してください", "invalid_isbn", "isbn")},
		{"ISBN が数", with(valid, "isbn", `9784000000000`), vt.Bad("Invalid input: expected string, received number", "invalid_type", "isbn")},
		{"総量の候補が空", with(valid, "metrics", `[]`), vt.Bad("総量を1つ以上入力してください", "too_small", "metrics")},
		{"総量の候補が文字列", with(valid, "metrics", `"x"`), vt.Bad("Invalid input: expected array, received string", "invalid_type", "metrics")},
		{"候補が null", with(valid, "metrics", `[null]`), vt.Bad("Invalid input: expected object, received null", "invalid_type", "metrics.0")},
		{"単位が違う", with(valid, "metrics", "["+metric(`"x"`, "1", "true")+"]"), vt.Bad("単位を選んでください", "invalid_range_unit", "metrics.0.unit")},
		{"単位が数", with(valid, "metrics", "["+metric(`1`, "1", "true")+"]"), vt.Bad("Invalid input: expected string, received number", "invalid_type", "metrics.0.unit")},
		{"総量が文字列", with(valid, "metrics", "["+metric(`"page"`, `"1"`, "true")+"]"), vt.Bad("総量を入力してください", "invalid_type", "metrics.0.totalAmount")},
		{"総量なし", with(valid, "metrics", `[{"unit":"page","isDefault":true}]`), vt.Bad("総量を入力してください", "invalid_type", "metrics.0.totalAmount")},
		{"総量が小数", with(valid, "metrics", "["+metric(`"page"`, "1.5", "true")+"]"), vt.Bad("総量は整数で入力してください", "invalid_type", "metrics.0.totalAmount")},
		{"総量が0", with(valid, "metrics", "["+metric(`"page"`, "0", "true")+"]"), vt.Bad("総量は1以上で入力してください", "too_small", "metrics.0.totalAmount")},
		{"総量が100001", with(valid, "metrics", "["+metric(`"page"`, "100001", "true")+"]"), vt.Bad("総量は100000以下で入力してください", "too_big", "metrics.0.totalAmount")},
		{"既定が文字列", with(valid, "metrics", "["+metric(`"page"`, "1", `"true"`)+"]"), vt.Bad("Invalid input: expected boolean, received string", "invalid_type", "metrics.0.isDefault")},
		{"単位が重なる", with(valid, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"page"`, "1", "false")+"]"), vt.Bad("同じ単位が重なっています", "duplicate_units", "metrics")},
		{"既定が無い", with(valid, "metrics", "["+metric(`"page"`, "1", "false")+"]"), vt.Bad("既定の単位を1つ選んでください", "default_unit_required", "metrics")},
		{"既定が2つ", with(valid, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"question"`, "1", "true")+"]"), vt.Bad("既定の単位を1つ選んでください", "default_unit_required", "metrics")},
		{"既定が2つで単位も重なる（重なりが先）", with(valid, "metrics", "["+metric(`"page"`, "1", "true")+","+metric(`"page"`, "1", "true")+"]"), vt.Bad("同じ単位が重なっています", "duplicate_units", "metrics")},
		{"候補が7つ（個数が重なりより先）", with(valid, "metrics", seq(7, func(i int) string { return metric(units[i%6], "1", "false") })), vt.Bad("Too big: expected array to have <=6 items", "too_big", "metrics")},
		{"候補が7つで先頭の単位が違う（要素が先）", with(valid, "metrics", seq(7, func(i int) string {
			if i == 0 {
				return metric(`"x"`, "1", "false")
			}
			return metric(units[i-1], "1", "false")
		})), vt.Bad("単位を選んでください", "invalid_range_unit", "metrics.0.unit")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decode(vt.Parse(t, tt.body))
			vt.CheckIssue(t, err, tt.want)
		})
	}
}

// TestDecodeValues は、通った入力を整えること（削る・整える・null にする）。
func TestDecodeValues(t *testing.T) {
	t.Run("ISBN を整え、空の出版社・版は null にする", func(t *testing.T) {
		for _, c := range []struct{ isbn, want string }{
			{"978-4-00-000000-0", "9784000000000"}, {"4-00-000000-x", "400000000X"}, {"4 00 000000 X", "400000000X"},
			{"978\u30004000000000", "9784000000000"}, {"\ufeff9784000000000\u00a0", "9784000000000"},
		} {
			got, err := decode(vt.Parse(t, with(with(with(valid, "isbn", `"`+c.isbn+`"`), "publisher", `" "`), "edition", "")))
			if err != nil {
				t.Fatalf("%q: %v", c.isbn, err)
			}
			if got.isbn != c.want || got.publisher != nil || got.edition != nil {
				t.Errorf("%q: isbn = %q, publisher = %v, edition = %v", c.isbn, got.isbn, got.publisher, got.edition)
			}
		}
	})

	t.Run("名前・出版社・版は削る", func(t *testing.T) {
		got, err := decode(vt.Parse(t, with(with(valid, "name", `" 本 "`), "publisher", `"  社  "`)))
		if err != nil || got.name != "本" || *got.publisher != "社" {
			t.Fatalf("name = %q, publisher = %v, err = %v", got.name, got.publisher, err)
		}
	})
}

// TestListQuery は一覧の q。形が違えば DB を引く前に断る（db は nil で渡す）。
func TestListQuery(t *testing.T) {
	for name, tt := range map[string]struct {
		query map[string][]string
		want  string
	}{
		"q が2つ":    {map[string][]string{"q": {"a", "b"}}, vt.Bad("Invalid input: expected string, received array", "invalid_type", "q")},
		"q が101文字": {map[string][]string{"q": {strings.Repeat("a", 101)}}, vt.Bad("Too big: expected string to have <=100 characters", "too_big", "q")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := List(context.Background(), nil, tt.query)
			vt.CheckIssue(t, err, tt.want)
		})
	}
}
