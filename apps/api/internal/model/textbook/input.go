package textbook

import (
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 入力の決まり。正は Zod の createTextbookSchema・updateTextbookProgressSchema（src/shared/validations/textbook.ts）。

// Subjects・RangeUnits は Zod のスキーマに書いた順（z.enum の文言に出る）。
// 科目は src/shared/subjects.ts の SUBJECT_VALUES。
var (
	Subjects   = []string{"english", "math", "japanese", "science", "social", "other"}
	RangeUnits = []string{"page", "question", "chapter", "number", "part", "section"}
)

// nameRule は z.string().trim().min(1, …).max(100, …)。削ってから長さを確かめる。
var nameRule = validation.StringRule{
	TrimFirst: true,
	Min:       1, MinMessage: "参考書名を入力してください",
	Max: 100, MaxMessage: "100文字以内で入力してください",
}

// totalAmountRule は z.number().int("整数で入力してください").min(1, …).max(100000, …)。
// 整数の min(1) は positive と同じ判定・同じ issue になる。
var totalAmountRule = validation.NumberRule{
	Int: true, IntMessage: "整数で入力してください",
	Positive: true, PositiveMessage: "1以上で入力してください",
	Max: 100000, MaxMessage: "100000以下で入力してください",
}

// createInput は createTextbookSchema を通した本文。fromMaster なら masterID だけを使う。null の項目は nil。
type createInput struct {
	fromMaster         bool
	masterID           int64
	name               string
	subject, rangeUnit *string
}

// decodeCreate は createTextbookSchema（z.union の2択）。
//
// Zod 4 の union は、選択肢を順に試して最初に通ったものを使う（名前と masterId の両方を送ると名前で作る。
// 知らない項目は捨てる）。どれも通らなければ、型の誤り（invalid_type・invalid_value）を1つも含まない
// 選択肢がちょうど1つのときだけその issue を返し、それ以外は invalid_union（field は null）の1件にする。
// 長さや範囲の誤り（too_small・too_big）は型の誤りではないので、たとえば {"name":""} は名前の too_small になる。
//
// その判定には選択肢の issue を全部見る必要があるので、ここは validation.ObjectInput（最初の1件で止まる）を使わずに読む。
func decodeCreate(body any) (createInput, error) {
	m, isObject := body.(map[string]any)
	if !isObject {
		return createInput{}, invalidUnion()
	}
	get := func(key string) (any, bool) {
		v, ok := m[key]
		if !ok {
			return validation.JSUndefined{}, false
		}
		return v, true
	}

	// 1つ目の選択肢：名前で作る。issue は項目の順（name・subject・rangeUnit）に積む。
	var byName []*validation.Issue
	nameValue, _ := get("name")
	name, issue := validation.CheckString("name", nameValue, nameRule)
	byName = appendIssue(byName, issue)
	var subject, rangeUnit *string
	if v, ok := get("subject"); ok && v != nil {
		s, issue := validation.CheckEnum("subject", v, Subjects)
		byName = appendIssue(byName, issue)
		subject = &s
	}
	if v, ok := get("rangeUnit"); ok {
		s, issue := validation.CheckEnum("rangeUnit", v, RangeUnits)
		byName = appendIssue(byName, issue)
		rangeUnit = &s
	}
	if len(byName) == 0 {
		return createInput{name: name, subject: subject, rangeUnit: rangeUnit}, nil
	}

	// 2つ目の選択肢：参考書マスターから作る。
	masterValue, _ := get("masterId")
	masterID, masterIssue := validation.CheckNumber("masterId", masterValue, validation.PositiveIntRule)
	if masterIssue == nil {
		return createInput{fromMaster: true, masterID: int64(masterID)}, nil
	}

	nameOK, masterOK := !anyTypeIssue(byName), !isTypeIssue(masterIssue)
	switch {
	case nameOK && !masterOK:
		return createInput{}, byName[0]
	case masterOK && !nameOK:
		return createInput{}, masterIssue
	}
	return createInput{}, invalidUnion()
}

func appendIssue(issues []*validation.Issue, issue *validation.Issue) []*validation.Issue {
	if issue == nil {
		return issues
	}
	return append(issues, issue)
}

// isTypeIssue は、Zod 4 がその選択肢をそこで打ち切る issue（型の誤り）か。
func isTypeIssue(issue *validation.Issue) bool {
	return issue.Code == "invalid_type" || issue.Code == "invalid_value"
}

func anyTypeIssue(issues []*validation.Issue) bool {
	for _, issue := range issues {
		if isTypeIssue(issue) {
			return true
		}
	}
	return false
}

func invalidUnion() *validation.Issue {
	return &validation.Issue{Code: "invalid_union", Message: "Invalid input"}
}

// progress は逆算設定の変更。Present の項目だけを書き換える。targetDate はその日の 00:00 UTC。
type progress struct {
	totalAmount        opt.Field[int64]
	rangeUnit, subject opt.Field[string]
	targetDate         opt.Field[time.Time]
}

// decodeProgress は updateTextbookProgressSchema。送られた項目だけを書き換える。
// 目標日は日付だけを持つので、UTC の 0 時にする。
func decodeProgress(body any) (progress, error) {
	in := validation.ReadObject(body)
	totalAmount := in.OptionalInt("totalAmount", totalAmountRule, false)
	rangeUnit := in.OptionalEnum("rangeUnit", RangeUnits, false)
	day := in.OptionalString("targetDate", validation.StringRule{Checks: []validation.StringCheck{validation.ISODateCheck}}, true)
	subject := in.OptionalEnum("subject", Subjects, true)
	if err := in.Err(); err != nil {
		return progress{}, err
	}
	targetDate := opt.Field[time.Time]{Present: day.Present}
	if v := day.Ptr(); v != nil {
		d := dates.FromYMD(*v)
		targetDate.Value = &d
	}
	return progress{
		totalAmount: totalAmount.Field(), rangeUnit: rangeUnit.Field(), subject: subject.Field(), targetDate: targetDate,
	}, nil
}
