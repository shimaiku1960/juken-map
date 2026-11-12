package studyrecord

import (
	"slices"
	"strconv"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり（Rails の validates にあたる）。
//
// 操作は送られた本文（JSON を読んだ値。数は json.Number）をそのまま受け取り、ここで形と値を確かめてから書く。
// 確かめ済みの値の型は外に出さないので、入力チェックを通らずに書き込むことはできない。
// 誤りは最初の1件を *validation.Issue で返し、コントローラーがそれを 400 にする。
//
// 規則の正は Zod の createStudyLogSchema・completeStudyPlanSchema（src/shared/validations/studyLog.ts）と
// createStudyPlansSchema・updateStudyPlanSchema（studyPlan.ts）。項目は Zod と同じ順に見る。

// Subjects は科目の値。RangeUnits は範囲の単位。契約（openapi/openapi.yaml）の enum と同じであることを
// コントローラー側のテスト（feature/study）で確かめている。
var (
	Subjects   = []string{"english", "japanese", "math", "other", "science", "social"}
	RangeUnits = []string{"chapter", "number", "page", "part", "question", "section"}
)

// MaxMinutes は実績1件の時間（分）の上限。
const MaxMinutes = 1440

// IsSubject は s が科目の値かを返す。
func IsSubject(s string) bool { return slices.Contains(Subjects, s) }

var (
	subjectRule   = validation.StringRule{Checks: []validation.StringCheck{{OK: IsSubject, Code: "invalid_subject", Message: "科目の値が不正です"}}}
	rangeUnitRule = validation.StringRule{Checks: []validation.StringCheck{{
		OK: func(s string) bool { return slices.Contains(RangeUnits, s) }, Code: "invalid_range_unit", Message: "単位の値が不正です",
	}}}
	// memoRule は実績のメモ・予定の内容：.max(500).trim()（null は不可）。
	memoRule = validation.StringRule{Max: 500, MaxMessage: "500文字以内で入力してください", Trim: true}
	// minutesRule は実績の時間（分）。実績の記録と予定の完了で同じ。
	minutesRule = validation.NumberRule{
		TypeMessage:     "学習時間を入力してください",
		Int:             true,
		IntMessage:      "整数で入力してください",
		Positive:        true,
		PositiveMessage: "1分以上を入力してください",
		Max:             MaxMinutes,
		MaxMessage:      "24時間（1440分）以内で入力してください",
	}
)

// tokyoToday は now の日本時間の日付（YYYY-MM-DD）。
func tokyoToday(now time.Time) string { return dates.YMD(dates.OnTokyo(now)) }

// rangeRules は superRefine の範囲の3つの規則（実績・予定・予定の完了で同じ）。Zod と同じ順に足す。
func rangeRules(in *validation.ObjectInput, start, end validation.Optional[int64], unit validation.Optional[string]) {
	hasStart, hasEnd := !start.IsNull(), !end.IsNull()
	if hasStart != hasEnd {
		field := "rangeStart"
		if hasStart {
			field = "rangeEnd"
		}
		in.AddIssue("range_incomplete", field, "範囲は開始と終了の両方を入力してください")
	}
	if hasStart && hasEnd && *start.Value > *end.Value {
		in.AddIssue("range_end_before_start", "rangeEnd", "終了は開始以上にしてください")
	}
	if (hasStart || hasEnd) && unit.IsNull() {
		in.AddIssue("range_unit_required", "rangeUnit", "単位を選択してください")
	}
}

// logInput は実績の作成・変更で書く値。Date はその日の 00:00 UTC（JavaScript の new Date("YYYY-MM-DD") と同じ値）。
// 任意の項目は「送られなかった」と null を区別する（変更で「範囲が変わったか」を見るため。opt.Field.Differs）。
type logInput struct {
	Date       time.Time
	Minutes    int64
	Subject    opt.Field[string]
	TextbookID opt.Field[int64]
	RangeStart opt.Field[int64]
	RangeEnd   opt.Field[int64]
	RangeUnit  opt.Field[string]
	Memo       opt.Field[string]
}

// decodeLogInput は createStudyLogSchema と同じ順に確かめる。today は日本時間の今日（YYYY-MM-DD）。
func decodeLogInput(attrs any, today string) (logInput, error) {
	in := validation.ReadObject(attrs)
	date := in.String("date", validation.YMDDateRule("日付を選択してください",
		// 文字列のまま比べる（形は validation.YMDDateRule で確かめてある）。
		validation.StringCheck{OK: func(s string) bool { return s <= today }, Code: "future_date", Message: "未来日は実績として記録できません"}))
	minutes := int64(in.Number("minutes", minutesRule))
	subject := in.OptionalString("subject", subjectRule, true)
	textbookID := in.OptionalInt("textbookId", validation.PositiveIntRule, true)
	rangeStart := in.OptionalInt("rangeStart", validation.PositiveIntRule, true)
	rangeEnd := in.OptionalInt("rangeEnd", validation.PositiveIntRule, true)
	rangeUnit := in.OptionalString("rangeUnit", rangeUnitRule, true)
	memo := in.OptionalString("memo", memoRule, false)
	rangeRules(in, rangeStart, rangeEnd, rangeUnit)
	if err := in.Err(); err != nil {
		return logInput{}, err
	}
	return logInput{
		Date: dates.FromYMD(date), Minutes: minutes, Subject: subject.Field(), TextbookID: textbookID.Field(),
		RangeStart: rangeStart.Field(), RangeEnd: rangeEnd.Field(), RangeUnit: rangeUnit.Field(), Memo: memo.Field(),
	}, nil
}

// planItem は予定1件の中身（studyPlanItemSchema）。
type planItem struct {
	TextbookID, RangeStart, RangeEnd *int64
	RangeUnit, Content, Subject      *string
}

func decodePlanItem(in *validation.ObjectInput) planItem {
	textbookID := in.OptionalInt("textbookId", validation.PositiveIntRule, true)
	rangeStart := in.OptionalInt("rangeStart", validation.PositiveIntRule, true)
	rangeEnd := in.OptionalInt("rangeEnd", validation.PositiveIntRule, true)
	rangeUnit := in.OptionalString("rangeUnit", rangeUnitRule, true)
	content := in.OptionalString("content", memoRule, false)
	subject := in.OptionalString("subject", subjectRule, true)
	rangeRules(in, rangeStart, rangeEnd, rangeUnit)
	// 中身ゼロ（参考書・範囲・メモが全部空）は不可。メモは trim した後の値で見る（Zod と同じ）。
	if textbookID.IsNull() && rangeStart.IsNull() && rangeEnd.IsNull() && (content.IsNull() || *content.Value == "") {
		in.AddIssue("plan_content_required", "content", "参考書・範囲・メモのいずれかを入力してください")
	}
	return planItem{
		TextbookID: textbookID.Ptr(), RangeStart: rangeStart.Ptr(), RangeEnd: rangeEnd.Ptr(),
		RangeUnit: rangeUnit.Ptr(), Content: content.Ptr(), Subject: subject.Ptr(),
	}
}

// decodePlans は createStudyPlansSchema。要素は番号の順に読み、最初の issue で止まる
// （Zod も要素ごとに、項目の確かめと superRefine を終えてから次の要素へ進む）。
func decodePlans(attrs any) (time.Time, []planItem, error) {
	in := validation.ReadObject(attrs)
	date := in.String("date", validation.YMDDateRule("日付を選択してください"))
	elements := in.Array("items", "内容を1つ以上入力してください")
	var items []planItem
	for i, element := range elements {
		if in.Issue != nil {
			break
		}
		item := validation.ReadObjectAt(element, in.Field("items")+"."+strconv.Itoa(i))
		items = append(items, decodePlanItem(item))
		in.Take(item)
	}
	if err := in.Err(); err != nil {
		return time.Time{}, nil, err
	}
	return dates.FromYMD(date), items, nil
}

// planPatch は予定の変更（updateStudyPlanSchema）。Present の項目だけを書き換える。Date はその日の 00:00 UTC。
type planPatch struct {
	Date                             opt.Field[time.Time]
	TextbookID, RangeStart, RangeEnd opt.Field[int64]
	RangeUnit, Content, Subject      opt.Field[string]
	Done                             opt.Field[bool]
}

func decodePlanPatch(attrs any) (planPatch, error) {
	in := validation.ReadObject(attrs)
	date := in.OptionalString("date", validation.YMDDateRule(""), false)
	textbookID := in.OptionalInt("textbookId", validation.PositiveIntRule, true)
	rangeStart := in.OptionalInt("rangeStart", validation.PositiveIntRule, true)
	rangeEnd := in.OptionalInt("rangeEnd", validation.PositiveIntRule, true)
	rangeUnit := in.OptionalString("rangeUnit", rangeUnitRule, true)
	content := in.OptionalString("content", memoRule, false)
	subject := in.OptionalString("subject", subjectRule, true)
	done := in.OptionalBool("done")
	rangeRules(in, rangeStart, rangeEnd, rangeUnit)
	if err := in.Err(); err != nil {
		return planPatch{}, err
	}
	patch := planPatch{
		Date:       opt.Field[time.Time]{Present: date.Present},
		TextbookID: textbookID.Field(), RangeStart: rangeStart.Field(), RangeEnd: rangeEnd.Field(),
		RangeUnit: rangeUnit.Field(), Content: content.Field(), Subject: subject.Field(),
		Done: done.Field(),
	}
	if date.Present {
		day := dates.FromYMD(*date.Value)
		patch.Date.Value = &day
	}
	return patch, nil
}

// completion は予定の完了で書く実績の値（completeStudyPlanSchema）。範囲は、送られていなければ
// （キーが無ければ）予定の値を使う。
type completion struct {
	Minutes              int64
	RangeStart, RangeEnd opt.Field[int64]
	RangeUnit            opt.Field[string]
	Memo                 *string
}

func decodeCompletion(attrs any) (completion, error) {
	in := validation.ReadObject(attrs)
	minutes := int64(in.Number("minutes", minutesRule))
	rangeStart := in.OptionalInt("rangeStart", validation.PositiveIntRule, true)
	rangeEnd := in.OptionalInt("rangeEnd", validation.PositiveIntRule, true)
	rangeUnit := in.OptionalString("rangeUnit", rangeUnitRule, true)
	memo := in.OptionalString("memo", memoRule, false)
	rangeRules(in, rangeStart, rangeEnd, rangeUnit)
	if err := in.Err(); err != nil {
		return completion{}, err
	}
	return completion{
		Minutes: minutes, RangeStart: rangeStart.Field(), RangeEnd: rangeEnd.Field(),
		RangeUnit: rangeUnit.Field(), Memo: memo.Ptr(),
	}, nil
}
