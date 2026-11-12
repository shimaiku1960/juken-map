package textbookmaster

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり。正は Zod の textbookMasterInputSchema（src/shared/validations/master.ts）。

// Units は総量の単位。Zod のスキーマ（RANGE_UNIT_VALUES）に書いた順。
var Units = []string{"page", "question", "chapter", "number", "part", "section"}

const (
	// metricsMax は総量の候補の上限。単位の数で、単位は重ねられないのでこれ以上は無い。
	metricsMax = 6
	// searchMax は一覧の検索語（q）の上限。前後の空白を削ってから数える。
	searchMax = 100
)

// input は textbookMasterInputSchema を通した本文。Metrics は1つ以上で、既定（IsDefault）はちょうど1つ。
type input struct {
	name               string
	publisher, edition *string
	isbn               string
	metrics            []Metric
}

// decode は textbookMasterInputSchema。
func decode(body any) (input, error) {
	in := validation.ReadObject(body)
	var v input
	v.name = in.String("name", validation.StringRule{
		TrimFirst: true,
		Min:       1, MinMessage: "参考書名を入力してください",
		Max: 150, MaxMessage: "参考書名は150文字以内で入力してください",
	})
	v.publisher = readOptionalText(in, "publisher", 100)
	v.edition = readOptionalText(in, "edition", 50)
	// isbn は z.string() を読んでから整え（transform）、整えた後の形を確かめる（refine）。
	v.isbn = normalizeISBN(in.String("isbn", validation.StringRule{}))
	if in.Issue == nil && !isbnPattern.MatchString(v.isbn) {
		in.AddIssue("invalid_isbn", "isbn", "ISBN は10桁か13桁で入力してください")
	}
	v.metrics = readMetrics(in)
	if err := in.Err(); err != nil {
		return input{}, err
	}
	return v, nil
}

// readOptionalText は任意の文字列：z.string().trim().max(max).nullable().optional() で、
// 空（キーが無い・null・削ったら空）なら null。
func readOptionalText(in *validation.ObjectInput, key string, max int) *string {
	o := in.OptionalString(key, validation.StringRule{TrimFirst: true, Max: max, MaxMessage: fmt.Sprintf("%d文字以内で入力してください", max)}, true)
	if o.Value == nil || *o.Value == "" {
		return nil
	}
	return o.Value
}

// isbnPattern は ISBN-13（数字13桁）か ISBN-10（数字9桁＋数字か X）。
var isbnPattern = regexp.MustCompile(`^(\d{13}|\d{9}[\dX])$`)

// normalizeISBN は、ハイフンと空白（JavaScript の \s と同じ範囲）を取り除いて大文字にする。
// 既存の ISBN は数字だけで入っている。大文字にするのは ISBN-10 の最後の x のため
// （JavaScript の toUpperCase と Go の ToUpper は一部の文字で結果が違うが、どちらも isbnPattern に合わない）。
func normalizeISBN(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '-' || validation.IsJSWhitespace(r) {
			return -1
		}
		return r
	}, s)
	return strings.ToUpper(s)
}

// readMetrics は総量の候補：z.array(metric).min(1).max(6).refine(単位が重ならない).refine(既定がちょうど1つ)。
// 要素 → min・max → refine の順に確かめる。
func readMetrics(in *validation.ObjectInput) []Metric {
	items := in.Array("metrics", "総量を1つ以上入力してください")
	if in.Issue != nil {
		return nil
	}
	metrics := make([]Metric, 0, len(items))
	for i, item := range items {
		m := validation.ReadObjectAt(item, in.Field("metrics")+"."+strconv.Itoa(i))
		metric := Metric{
			Unit: m.String("unit", validation.StringRule{Checks: []validation.StringCheck{
				validation.OneOf(Units, "invalid_range_unit", "単位を選んでください"),
			}}),
			// z.number({ error: "総量を入力してください" }).int(…).min(1, …).max(100000, …)。
			// 整数に限ると min(1) は positive と同じ（0.5 は int で先に弾かれる）。
			TotalAmount: int(m.Number("totalAmount", validation.NumberRule{
				TypeMessage: "総量を入力してください",
				Int:         true, IntMessage: "総量は整数で入力してください",
				Positive: true, PositiveMessage: "総量は1以上で入力してください",
				Max: 100_000, MaxMessage: "総量は100000以下で入力してください",
			})),
			IsDefault: m.Boolean("isDefault"),
		}
		if m.Issue != nil {
			in.Take(m)
			return nil
		}
		metrics = append(metrics, metric)
	}
	if len(metrics) > metricsMax {
		in.AddIssue("too_big", "metrics", "Too big: expected array to have <=6 items")
		return nil
	}
	units := make(map[string]bool, len(metrics))
	defaults := 0
	for _, m := range metrics {
		if units[m.Unit] {
			in.AddIssue("duplicate_units", "metrics", "同じ単位が重なっています")
			return nil
		}
		units[m.Unit] = true
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		in.AddIssue("default_unit_required", "metrics", "既定の単位を1つ選んでください")
		return nil
	}
	return metrics
}
