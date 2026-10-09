package textbooks

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
	"github.com/shimaiku1960/juken-map/apps/api/internal/write/textbook"
)

// 参考書の書き込み（JUK-75）。POST /api/textbooks と PATCH /api/textbooks/{id}。
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
var textbookNameRule = httpx.StringRule{
	TrimFirst: true,
	Min:       1, MinMessage: "参考書名を入力してください",
	Max: 100, MaxMessage: "100文字以内で入力してください",
}

// totalAmountRule は z.number().int("整数で入力してください").min(1, …).max(100000, …)。
// 整数の min(1) は positive と同じ判定・同じ issue になる。
var totalAmountRule = httpx.NumberRule{
	Int: true, IntMessage: "整数で入力してください",
	Positive: true, PositiveMessage: "1以上で入力してください",
	Max: 100000, MaxMessage: "100000以下で入力してください",
}

// textbookInput は createTextbookSchema を通した本文。fromMaster なら masterID だけを使う。
type textbookInput struct {
	fromMaster bool
	masterID   int64
	name       string
	subject    httpx.Optional[string]
	rangeUnit  httpx.Optional[string]
}

// readTextbookInput は createTextbookSchema（z.union の2択）。
//
// Zod 4 の union は、選択肢を順に試して最初に通ったものを使う（名前と masterId の両方を送ると名前で作る。
// 知らない項目は捨てる）。どれも通らなければ、型の誤り（invalid_type・invalid_value）を1つも含まない
// 選択肢がちょうど1つのときだけその issue を返し、それ以外は invalid_union（field は null）の1件にする。
// 長さや範囲の誤り（too_small・too_big）は型の誤りではないので、たとえば {"name":""} は名前の too_small になる。
//
// その判定には選択肢の issue を全部見る必要があるので、ここは httpx.ObjectInput（最初の1件で止まる）を使わずに読む。
func readTextbookInput(body any) (textbookInput, *httpx.ValidationIssue) {
	m, isObject := body.(map[string]any)
	if !isObject {
		return textbookInput{}, invalidUnion()
	}
	get := func(key string) (any, bool) {
		v, ok := m[key]
		if !ok {
			return httpx.JSUndefined{}, false
		}
		return v, true
	}

	// 1つ目の選択肢：名前で作る。issue は項目の順（name・subject・rangeUnit）に積む。
	var byName []*httpx.ValidationIssue
	nameValue, _ := get("name")
	name, issue := httpx.CheckString("name", nameValue, textbookNameRule)
	byName = appendIssue(byName, issue)
	subject := httpx.Optional[string]{}
	if v, ok := get("subject"); ok {
		subject.Present = true
		if v != nil {
			s, issue := httpx.CheckEnum("subject", v, subjectValues)
			byName = appendIssue(byName, issue)
			subject.Value = &s
		}
	}
	rangeUnit := httpx.Optional[string]{}
	if v, ok := get("rangeUnit"); ok {
		s, issue := httpx.CheckEnum("rangeUnit", v, textbookRangeUnits)
		byName = appendIssue(byName, issue)
		rangeUnit = httpx.Optional[string]{Present: true, Value: &s}
	}
	if len(byName) == 0 {
		return textbookInput{name: name, subject: subject, rangeUnit: rangeUnit}, nil
	}

	// 2つ目の選択肢：参考書マスターから作る。
	masterValue, _ := get("masterId")
	masterID, masterIssue := httpx.CheckNumber("masterId", masterValue, httpx.PositiveIntRule)
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

func appendIssue(issues []*httpx.ValidationIssue, issue *httpx.ValidationIssue) []*httpx.ValidationIssue {
	if issue == nil {
		return issues
	}
	return append(issues, issue)
}

// isTypeIssue は、Zod 4 がその選択肢をそこで打ち切る issue（型の誤り）か。
func isTypeIssue(issue *httpx.ValidationIssue) bool {
	return issue.Code == "invalid_type" || issue.Code == "invalid_value"
}

func anyTypeIssue(issues []*httpx.ValidationIssue) bool {
	for _, issue := range issues {
		if isTypeIssue(issue) {
			return true
		}
	}
	return false
}

func invalidUnion() *httpx.ValidationIssue {
	return &httpx.ValidationIssue{Code: "invalid_union", Message: "Invalid input"}
}

// readTextbookProgress は updateTextbookProgressSchema。送られた項目だけを書き換える。
func readTextbookProgress(body any) (*httpx.ObjectInput, textbookProgress) {
	in := httpx.ReadObject(body)
	var v textbookProgress
	v.totalAmount = in.OptionalInt("totalAmount", totalAmountRule, false)
	v.rangeUnit = in.OptionalEnum("rangeUnit", textbookRangeUnits, false)
	v.targetDate = in.OptionalString("targetDate", httpx.StringRule{Checks: []httpx.StringCheck{httpx.ISODateCheck}}, true)
	v.subject = in.OptionalEnum("subject", subjectValues, true)
	return in, v
}

type textbookProgress struct {
	totalAmount                    httpx.Optional[int64]
	rangeUnit, targetDate, subject httpx.Optional[string]
}

// record は持ち主に渡す形にする。目標日は日付だけを持つので、UTC の 0 時にする。
func (p textbookProgress) record() textbook.Progress {
	targetDate := opt.Field[time.Time]{Present: p.targetDate.Present}
	if v := p.targetDate.Ptr(); v != nil {
		day := dates.FromYMD(*v)
		targetDate.Value = &day
	}
	return textbook.Progress{
		TotalAmount: p.totalAmount.Field(), RangeUnit: p.rangeUnit.Field(), Subject: p.subject.Field(), TargetDate: targetDate,
	}
}

// Create は POST /api/textbooks。
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	input, issue := readTextbookInput(body.Value())
	if issue != nil {
		issue.Write(w)
		return
	}

	var created textbook.Textbook
	var err error
	if input.fromMaster {
		created, err = textbook.CreateFromMaster(r.Context(), h.store.db, s.UserID, input.masterID, dates.NowMillis())
	} else {
		created, err = textbook.Create(r.Context(), h.store.db, s.UserID,
			textbook.New{Name: input.name, RangeUnit: input.rangeUnit.Ptr(), Subject: input.subject.Ptr()}, dates.NowMillis())
	}
	switch {
	case errors.Is(err, textbook.ErrMasterNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, textbook.ErrMasterNoMetric):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, textbook.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case err != nil:
		httpx.InternalError(w, r, fmt.Errorf("textbooks create: %w", err))
	default:
		httpx.WriteJSON(w, http.StatusCreated, apischema.TextbookRow(created))
	}
}

// UpdateProgress は PATCH /api/textbooks/{id}。入力チェックは自分の参考書かを確かめるより先。
func (h *Handlers) UpdateProgress(w http.ResponseWriter, r *http.Request, s *httpx.Session) {
	body, ok := httpx.ReadBody(w, r, httpx.DefaultBodyLimit)
	if !ok {
		return
	}
	id, ok := httpx.PathID(w, r, "id")
	if !ok {
		return
	}
	in, progress := readTextbookProgress(body.Value())
	if in.Reject(w) {
		return
	}
	updated, err := textbook.UpdateProgress(r.Context(), h.store.db, s.UserID, id, progress.record(), dates.NowMillis())
	switch {
	case errors.Is(err, textbook.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case err != nil:
		httpx.InternalError(w, r, fmt.Errorf("textbooks update: %w", err))
	default:
		httpx.WriteJSON(w, http.StatusOK, apischema.TextbookRow(updated))
	}
}
