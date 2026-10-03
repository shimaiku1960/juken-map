package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// 参考書の書き込み（JUK-75）。Node の次の部分にあたる。
//   - routes/textbooks.ts の POST /api/textbooks と PATCH /api/textbooks/:id
//   - services/textbook-service.ts の createTextbook・createTextbookFromMaster・updateTextbookProgress・findOwnedTextbook
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

var (
	errDuplicateTextbook = errors.New("この参考書はすでに登録されています")
	errMasterNotFound    = errors.New("参考書マスターが見つかりません")
	errMasterNoMetric    = errors.New("参考書の総量データが登録されていません")
)

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

// findTextbook は参考書を1件読む。userID が空でなければ、その人のものだけ（Node の findOwnedTextbook）。無ければ nil。
func (st *textbookStore) findTextbook(ctx context.Context, id int64, userID string) (*TextbookRow, error) {
	query := "SELECT " + textbookRowColumns + " FROM Textbook WHERE id = ?"
	args := []any{id}
	if userID != "" {
		query += " AND userId = ? LIMIT 1"
		args = append(args, userID)
	}
	t, err := scanTextbook(st.db.QueryRowContext(ctx, query, args...).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// newTextbook は登録する参考書の値。null の項目は nil。
type newTextbook struct {
	name                  string
	masterID, totalAmount *int64
	rangeUnit, subject    *string
}

// createTextbook は参考書を登録する。同名の重複は DB の一意制約（userId, name）が弾くので、
// それを errDuplicateTextbook にする。
func (st *textbookStore) createTextbook(ctx context.Context, userID string, t newTextbook) (*TextbookRow, error) {
	now := nowMillis()
	res, err := st.db.ExecContext(ctx,
		`INSERT INTO Textbook
		   (userId, name, masterId, totalAmount, rangeUnit, subject, createdAt, updatedAt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, t.name, t.masterID, t.totalAmount, t.rangeUnit, t.subject, now, now)
	if isMySQLError(err, mysqlDuplicateEntry) {
		return nil, errDuplicateTextbook
	}
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return st.mustFindTextbook(ctx, id)
}

func (st *textbookStore) mustFindTextbook(ctx context.Context, id int64) (*TextbookRow, error) {
	t, err := st.findTextbook(ctx, id, "")
	if err == nil && t == nil {
		err = fmt.Errorf("Textbook %d が見つかりません", id)
	}
	return t, err
}

// textbookFromMaster は参考書マスターから登録する値を作る（Node の createTextbookFromMaster）。
// 総量はマスターの既定（isDefault）の候補を使い、既定が無ければ先頭（id が最小）の候補を使う。
func (st *textbookStore) textbookFromMaster(ctx context.Context, masterID int64) (newTextbook, error) {
	rows, err := st.db.QueryContext(ctx,
		`SELECT tm.name, m.unit, m.totalAmount, m.isDefault
		 FROM TextbookMaster AS tm
		 LEFT JOIN TextbookMasterMetric AS m ON m.masterId = tm.id
		 WHERE tm.id = ?
		 ORDER BY m.id ASC`,
		masterID)
	if err != nil {
		return newTextbook{}, err
	}
	defer rows.Close()

	var (
		name   string
		found  bool
		first  *newTextbook
		chosen *newTextbook
	)
	for rows.Next() {
		var (
			unit        *string
			totalAmount *int64
			isDefault   *bool
		)
		if err := rows.Scan(&name, &unit, &totalAmount, &isDefault); err != nil {
			return newTextbook{}, err
		}
		found = true
		// LEFT JOIN の相手（総量の候補）が居なければ、候補の列が NULL の行が1行だけ来る
		if unit == nil {
			continue
		}
		candidate := &newTextbook{masterID: &masterID, totalAmount: totalAmount, rangeUnit: unit}
		if first == nil {
			first = candidate
		}
		if chosen == nil && *isDefault {
			chosen = candidate
		}
	}
	if err := rows.Err(); err != nil {
		return newTextbook{}, err
	}
	switch {
	case !found:
		return newTextbook{}, errMasterNotFound
	case chosen == nil && first == nil:
		return newTextbook{}, errMasterNoMetric
	case chosen == nil:
		chosen = first
	}
	chosen.name = name
	return *chosen, nil
}

// updateProgress は逆算設定のうち、送られてきた項目だけを書き換える（Node の updateTextbookProgress）。
// 更新日時は何も送られていなくても書き換える。
func (st *textbookStore) updateProgress(ctx context.Context, id int64, p textbookProgress) (*TextbookRow, error) {
	// 列名はこのコードに書いた固定の名前だけで、利用者の入力は値として ? で渡す。
	var columns []string
	var args []any
	if p.totalAmount.present {
		columns, args = append(columns, "totalAmount = ?"), append(args, *p.totalAmount.value)
	}
	if p.rangeUnit.present {
		columns, args = append(columns, "rangeUnit = ?"), append(args, *p.rangeUnit.value)
	}
	if p.targetDate.present {
		// 目標日は日付だけを持つので、UTC の 0 時として保存する。
		var targetDate any
		if v := p.targetDate.ptr(); v != nil {
			targetDate = dateFromYMD(*v)
		}
		columns, args = append(columns, "targetDate = ?"), append(args, targetDate)
	}
	if p.subject.present {
		columns, args = append(columns, "subject = ?"), append(args, p.subject.ptr())
	}
	columns, args = append(columns, "updatedAt = ?"), append(args, nowMillis())

	// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（columns）。値は args で ? として渡す
	if _, err := st.db.ExecContext(ctx,
		"UPDATE Textbook SET "+strings.Join(columns, ", ")+" WHERE id = ?", append(args, id)...); err != nil {
		return nil, err
	}
	return st.mustFindTextbook(ctx, id)
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

	t := newTextbook{name: input.name, rangeUnit: input.rangeUnit.ptr(), subject: input.subject.ptr()}
	if input.fromMaster {
		var err error
		t, err = h.store.textbookFromMaster(r.Context(), input.masterID)
		switch {
		case errors.Is(err, errMasterNotFound):
			writeError(w, http.StatusNotFound, err.Error())
			return
		case errors.Is(err, errMasterNoMetric):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			internalError(w, r, fmt.Errorf("textbooks master: %w", err))
			return
		}
	}
	created, err := h.store.createTextbook(r.Context(), s.UserID, t)
	if errors.Is(err, errDuplicateTextbook) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		internalError(w, r, fmt.Errorf("textbooks create: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, created)
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
	owned, err := h.store.findTextbook(r.Context(), id, s.UserID)
	if err != nil {
		internalError(w, r, fmt.Errorf("textbooks find: %w", err))
		return
	}
	if owned == nil {
		writeError(w, http.StatusNotFound, "参考書が見つかりません")
		return
	}
	updated, err := h.store.updateProgress(r.Context(), id, progress)
	if err != nil {
		internalError(w, r, fmt.Errorf("textbooks update: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
