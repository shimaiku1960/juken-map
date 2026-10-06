package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/textbook"
)

// 参考書の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/textbooks.ts の POST /api/textbooks と PATCH /api/textbooks/:id
//   - services/textbook-service.ts の createTextbook・createTextbookFromMaster・updateTextbookProgress・findOwnedTextbook
//
// 書き込みは持ち主の internal/write/textbook にある（JUK-154）。ここは本文を確かめ、操作を呼び、結果を応答の形にする。
//
// 入力チェックの規則の正は Zod の createTextbookSchema・updateTextbookProgressSchema（src/shared/validations/textbook.ts）。

// subjectValues・textbookRangeUnits は Zod のスキーマに書いた順（z.enum の文言に出る）。
// 科目は src/shared/subjects.ts の SUBJECT_VALUES。
var (
	subjectValues      = []string{"english", "math", "japanese", "science", "social", "other"}
	textbookRangeUnits = []string{"page", "question", "chapter", "number", "part", "section"}
)

// textbookNameRule は z.string().trim().min(1, …).max(100, …)。削ってから長さを確かめる。
var textbookNameRule = stringRule{
	trimFirst: true,
	min:       1, minMessage: "参考書名を入力してください",
	max: 100, maxMessage: "100文字以内で入力してください",
}

// totalAmountRule は z.number().int("整数で入力してください").min(1, …).max(100000, …)。
// 整数の min(1) は positive と同じ判定・同じ issue になる。
var totalAmountRule = numberRule{
	int: true, intMessage: "整数で入力してください",
	positive: true, positiveMessage: "1以上で入力してください",
	max: 100000, maxMessage: "100000以下で入力してください",
}

// textbookInput は createTextbookSchema を通した本文。fromMaster なら masterID だけを使う。
type textbookInput struct {
	fromMaster bool
	masterID   int64
	name       string
	subject    optional[string]
	rangeUnit  optional[string]
}

// readTextbookInput は createTextbookSchema（z.union の2択）。
//
// Zod 4 の union は、選択肢を順に試して最初に通ったものを使う（名前と masterId の両方を送ると名前で作る。
// 知らない項目は捨てる）。どれも通らなければ、型の誤り（invalid_type・invalid_value）を1つも含まない
// 選択肢がちょうど1つのときだけその issue を返し、それ以外は invalid_union（field は null）の1件にする。
// 長さや範囲の誤り（too_small・too_big）は型の誤りではないので、たとえば {"name":""} は名前の too_small になる。
//
// その判定には選択肢の issue を全部見る必要があるので、ここは objectInput（最初の1件で止まる）を使わずに読む。
func readTextbookInput(body any) (textbookInput, *validationIssue) {
	m, isObject := body.(map[string]any)
	if !isObject {
		return textbookInput{}, invalidUnion()
	}
	get := func(key string) (any, bool) {
		v, ok := m[key]
		if !ok {
			return jsUndefined{}, false
		}
		return v, true
	}

	// 1つ目の選択肢：名前で作る。issue は項目の順（name・subject・rangeUnit）に積む。
	var byName []*validationIssue
	nameValue, _ := get("name")
	name, issue := checkString("name", nameValue, textbookNameRule)
	byName = appendIssue(byName, issue)
	subject := optional[string]{}
	if v, ok := get("subject"); ok {
		subject.present = true
		if v != nil {
			s, issue := checkEnum("subject", v, subjectValues)
			byName = appendIssue(byName, issue)
			subject.value = &s
		}
	}
	rangeUnit := optional[string]{}
	if v, ok := get("rangeUnit"); ok {
		s, issue := checkEnum("rangeUnit", v, textbookRangeUnits)
		byName = appendIssue(byName, issue)
		rangeUnit = optional[string]{present: true, value: &s}
	}
	if len(byName) == 0 {
		return textbookInput{name: name, subject: subject, rangeUnit: rangeUnit}, nil
	}

	// 2つ目の選択肢：参考書マスターから作る。
	masterValue, _ := get("masterId")
	masterID, masterIssue := checkNumber("masterId", masterValue, positiveIntRule)
	if masterIssue == nil {
		return textbookInput{fromMaster: true, masterID: int64(masterID)}, nil
	}

	nameOK, masterOK := !anyTypeIssue(byName), !isTypeIssue(masterIssue)
	switch {
	case nameOK && !masterOK:
		return textbookInput{}, byName[0]
	case masterOK && !nameOK:
		return textbookInput{}, masterIssue
	}
	return textbookInput{}, invalidUnion()
}

func appendIssue(issues []*validationIssue, issue *validationIssue) []*validationIssue {
	if issue == nil {
		return issues
	}
	return append(issues, issue)
}

// isTypeIssue は、Zod 4 がその選択肢をそこで打ち切る issue（型の誤り）か。
func isTypeIssue(issue *validationIssue) bool {
	return issue.code == "invalid_type" || issue.code == "invalid_value"
}

func anyTypeIssue(issues []*validationIssue) bool {
	for _, issue := range issues {
		if isTypeIssue(issue) {
			return true
		}
	}
	return false
}

func invalidUnion() *validationIssue {
	return &validationIssue{code: "invalid_union", message: "Invalid input"}
}

// readTextbookProgress は updateTextbookProgressSchema。送られた項目だけを書き換える。
func readTextbookProgress(body any) (*objectInput, textbookProgress) {
	in := readObject(body)
	var v textbookProgress
	v.totalAmount = in.optionalInt("totalAmount", totalAmountRule, false)
	v.rangeUnit = in.optionalEnum("rangeUnit", textbookRangeUnits, false)
	v.targetDate = in.optionalString("targetDate", stringRule{checks: []stringCheck{isoDateCheck}}, true)
	v.subject = in.optionalEnum("subject", subjectValues, true)
	return in, v
}

type textbookProgress struct {
	totalAmount                    optional[int64]
	rangeUnit, targetDate, subject optional[string]
}

// record は持ち主に渡す形にする。目標日は日付だけを持つので、UTC の 0 時にする。
func (p textbookProgress) record() textbook.Progress {
	targetDate := opt.Field[time.Time]{Present: p.targetDate.present}
	if v := p.targetDate.ptr(); v != nil {
		day := dateFromYMD(*v)
		targetDate.Value = &day
	}
	return textbook.Progress{
		TotalAmount: p.totalAmount.field(), RangeUnit: p.rangeUnit.field(), Subject: p.subject.field(), TargetDate: targetDate,
	}
}

// create は POST /api/textbooks。
func (h *textbookHandlers) create(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	input, issue := readTextbookInput(body.value())
	if issue != nil {
		issue.write(w)
		return
	}

	var created textbook.Textbook
	var err error
	if input.fromMaster {
		created, err = textbook.CreateFromMaster(r.Context(), h.store.db, s.UserID, input.masterID, nowMillis())
	} else {
		created, err = textbook.Create(r.Context(), h.store.db, s.UserID,
			textbook.New{Name: input.name, RangeUnit: input.rangeUnit.ptr(), Subject: input.subject.ptr()}, nowMillis())
	}
	switch {
	case errors.Is(err, textbook.ErrMasterNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, textbook.ErrMasterNoMetric):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, textbook.ErrDuplicate):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		internalError(w, r, fmt.Errorf("textbooks create: %w", err))
	default:
		writeJSON(w, http.StatusCreated, apischema.TextbookRow(created))
	}
}

// updateProgress は PATCH /api/textbooks/{id}。入力チェックは自分の参考書かを確かめるより先（Node と同じ）。
func (h *textbookHandlers) updateProgress(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := pathID(w, r, "id")
	if !ok {
		return
	}
	in, progress := readTextbookProgress(body.value())
	if in.reject(w) {
		return
	}
	updated, err := textbook.UpdateProgress(r.Context(), h.store.db, s.UserID, id, progress.record(), nowMillis())
	switch {
	case errors.Is(err, textbook.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		internalError(w, r, fmt.Errorf("textbooks update: %w", err))
	default:
		writeJSON(w, http.StatusOK, apischema.TextbookRow(updated))
	}
}
