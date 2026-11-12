package university

import (
	"slices"
	"strconv"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり。契約は openapi/openapi.yaml の UniversityInput・FacultyInput・CreateFacultyInput。

// Prefectures は47都道府県。画面の src/shared/prefectures.ts の PREFECTURES と同じ並び。
var Prefectures = []string{
	"北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県",
	"茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県",
	"新潟県", "山梨県", "長野県",
	"富山県", "石川県", "福井県",
	"岐阜県", "静岡県", "愛知県", "三重県",
	"滋賀県", "京都府", "大阪府", "兵庫県", "奈良県", "和歌山県",
	"鳥取県", "島根県", "岡山県", "広島県", "山口県",
	"徳島県", "香川県", "愛媛県", "高知県",
	"福岡県", "佐賀県", "長崎県", "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
}

// Types は大学の種別。契約（openapi.yaml の UniversityInput.type）と同じ値で、feature の TestEnumsMatchContract が突き合わせる。
var Types = []string{"国立", "公立", "私立"}

const (
	// tagsMax は学部に付けられるタグの上限。
	tagsMax = 20
	// searchMax は管理画面の一覧の検索語（q）の上限。前後の空白を削ってから数える。
	searchMax = 100
)

type universityInput struct {
	name, prefecture, typ string
}

type facultyInput struct {
	name string
	// examDate はその日の 00:00 UTC。
	examDate time.Time
	tagIDs   []int64
	// universityID は作成のときだけ（書き換えでは大学を移せない）。
	universityID int64
}

// decodeUniversity は universityInputSchema。
func decodeUniversity(body any) (universityInput, error) {
	in := validation.ReadObject(body)
	v := universityInput{
		name:       in.String("name", nameRule("大学名")),
		prefecture: in.String("prefecture", validation.StringRule{Checks: []validation.StringCheck{validation.OneOf(Prefectures, "invalid_prefecture", "都道府県を選んでください")}}),
		typ:        in.Enum("type", func(s string) bool { return slices.Contains(Types, s) }, "種別を選んでください"),
	}
	if err := in.Err(); err != nil {
		return universityInput{}, err
	}
	return v, nil
}

// decodeFaculty は facultyInputSchema（withUniversity なら createFacultySchema）。
// createFacultySchema は facultyInputSchema を extend したものなので、universityId は最後に確かめる。
func decodeFaculty(body any, withUniversity bool) (facultyInput, error) {
	in := validation.ReadObject(body)
	var v facultyInput
	v.name = in.String("name", nameRule("学部名"))
	examDate := in.String("examDate", validation.StringRule{Checks: []validation.StringCheck{
		{OK: validation.YMDPattern.MatchString, Code: "invalid_format", Message: "受験日を選んでください"},
		{OK: func(s string) bool { _, ok := examDateOf(s); return ok }, Code: "invalid_exam_date", Message: "受験日を選んでください"},
	}})
	v.tagIDs = readTagIDs(in, "tagIds")
	if withUniversity {
		v.universityID = int64(in.Number("universityId", validation.NumberRule{Int: true, Positive: true}))
	}
	if err := in.Err(); err != nil {
		return facultyInput{}, err
	}
	v.examDate, _ = examDateOf(examDate)
	return v, nil
}

// nameRule は名前の規則：z.string().trim().min(1).max(100)。
func nameRule(label string) validation.StringRule {
	return validation.StringRule{
		TrimFirst: true,
		Min:       1, MinMessage: label + "を入力してください",
		Max: 100, MaxMessage: label + "は100文字以内で入力してください",
	}
}

// examDateOf は受験日（YYYY-MM-DD）を、その日の UTC の0時にする。月が 01〜12・日が 01〜31 なら受け付け、
// その月に無い日は time.Date が繰り上げる（2027-02-30 は 3月2日。JavaScript の new Date も同じ）。形（validation.YMDPattern）は呼ぶ前に確かめておくこと。
func examDateOf(s string) (time.Time, bool) {
	year, _ := strconv.Atoi(s[0:4])
	month, _ := strconv.Atoi(s[5:7])
	day, _ := strconv.Atoi(s[8:10])
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

// readTagIDs は z.array(z.number().int().positive()).max(20, …).refine(重ならない)。
// Zod は要素 → max → refine の順に確かめる（要素が不正なら、21個でも要素の issue が先）。
func readTagIDs(in *validation.ObjectInput, key string) []int64 {
	items := in.Array(key, "")
	if in.Issue != nil {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for i, item := range items {
		f, issue := validation.CheckNumber(in.Field(key)+"."+strconv.Itoa(i), item, validation.NumberRule{Int: true, Positive: true})
		if issue != nil {
			in.Issue = issue
			return nil
		}
		ids = append(ids, int64(f))
	}
	if len(ids) > tagsMax {
		in.AddIssue("too_big", key, "タグは20個までです")
		return nil
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			in.AddIssue("duplicate_tags", key, "同じタグが重なっています")
			return nil
		}
		seen[id] = true
	}
	return ids
}
