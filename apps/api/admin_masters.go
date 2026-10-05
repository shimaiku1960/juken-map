package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// 管理者ページのマスター編集（/admin/masters、JUK-78）。Node の routes/admin-masters.ts と
// services/master-service.ts にあたる。どれも rt.admin（管理者＋2段階認証を通したセッションだけ）で登録する。
//
// 削除は「誰にも使われていない行」に限る。
//   - 学部：志望校（FinalGoal）が参照していれば消せない（DB も ON DELETE RESTRICT で拒む）
//   - 大学：学部は CASCADE で一緒に消えるので、配下の学部がどれか志望校に使われていれば止める
//   - 参考書：利用者の参考書（Textbook.masterId）は SET NULL で黙って紐づきが外れるので、1冊でもあれば止める
//
// 事前に数えて断るのが基本で、数えたあとに志望校が増えた場合も、DB の外部キーが拒んだエラーを
// 同じ「使われている」に読み替える（二重の守り）。
//
// 大学・学部を変えたら、大学を探す画面の一覧のキャッシュ（universities.go）を捨てる。成功したときだけ。
// 変更はすべて構造化ログ「admin master change」に「誰が・何を・前→後」で残す（Node と同じ項目。Grafana の Loki で追える）。

const (
	adminUniversitiesPageSize = 50
	// adminUniversitiesMaxPage は Node の listQuerySchema の page の上限。
	adminUniversitiesMaxPage = 1_000
	// adminMasterQueryMax は一覧の検索語（q）の上限。前後の空白を削ってから数える。
	adminMasterQueryMax = 100
	// adminTextbookMastersLimit は参考書マスターの一覧の上限。件数が少ないので、ページに分けず先頭から返す。
	adminTextbookMastersLimit = 200
	adminFacultyTagsMax       = 20
	// textbookMetricsMax は総量の候補の上限。単位（RANGE_UNIT_VALUES）の数で、単位は重ねられないのでこれ以上は無い。
	textbookMetricsMax = 6
)

// prefectures は47都道府県。Node の src/shared/prefectures.ts の PREFECTURES と同じ並び。
var prefectures = []string{
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

// masterFailure は、行を書き換えずに断った理由。
type masterFailure int

const (
	masterOK masterFailure = iota
	masterNotFound
	masterDuplicate
	masterInUse
	masterInvalidTags
)

// masterOutcome は書き込みの結果。failure が masterOK なら value に結果が入る。
// count は masterInUse のときの「使われている件数」（文言に入れる）。
type masterOutcome[T any] struct {
	failure masterFailure
	value   T
	count   int
}

// masterChange は書き換えの前と後。監査ログに両方を残し、応答は後を返す。
type masterChange[T any] struct {
	before, after T
}

// masterMessages は断ったときの文言。Node の MESSAGES と同じ。inUse は件数を %d で入れる。
type masterMessages struct {
	notFound, duplicate, inUse string
}

var (
	universityMessages = masterMessages{
		notFound:  "大学が見つかりません",
		duplicate: "同じ名前の大学がすでにあります",
		inUse:     "この大学の学部が志望校に%d件使われているため削除できません",
	}
	facultyMessages = masterMessages{
		notFound:  "学部（または大学）が見つかりません",
		duplicate: "この大学に同じ名前の学部がすでにあります",
		inUse:     "この学部が志望校に%d件使われているため削除できません",
	}
	textbookMasterMessages = masterMessages{
		notFound:  "参考書が見つかりません",
		duplicate: "同じ ISBN の参考書がすでにあります",
		inUse:     "この参考書は利用者の%d冊に使われているため削除できません",
	}
)

// rejectMasterFailure は断った結果なら Node の sendFailure と同じ status・文言を送って true を返す。
func rejectMasterFailure(w http.ResponseWriter, m masterMessages, failure masterFailure, count int) bool {
	switch failure {
	case masterOK:
		return false
	case masterNotFound:
		writeError(w, http.StatusNotFound, m.notFound)
	case masterDuplicate:
		writeError(w, http.StatusConflict, m.duplicate)
	case masterInUse:
		writeError(w, http.StatusConflict, fmt.Sprintf(m.inUse, count))
	case masterInvalidTags:
		writeError(w, http.StatusBadRequest, "存在しないタグが含まれています")
	}
	return true
}

// logMasterChange は、誰が・何を・前→後に変えたかを残す（Node の logChange と同じ項目・同じ文言・info）。
// detail は "before", 値, "after", 値 のように並べる。
func logMasterChange(ctx context.Context, adminID, action, table string, id int64, detail ...any) {
	attrs := append([]any{"adminId", adminID, "action", action, "table", table, "id", id}, detail...)
	slog.InfoContext(ctx, "admin master change", attrs...)
}

// adminMasterStore は DB の読み書き。テストでは偽物を渡す（Go の CI には DB が無い）。
// 書き込みは、断ったときは failure を、想定外の失敗は error を返す。
type adminMasterStore interface {
	listUniversities(ctx context.Context, q string, page int) (AdminUniversityList, error)
	// universityDetail は大学と学部。大学が無ければ nil。
	universityDetail(ctx context.Context, id int64) (*AdminUniversityDetail, error)
	listTags(ctx context.Context) ([]AdminTag, error)
	createUniversity(ctx context.Context, in universityInput) (masterOutcome[AdminUniversity], error)
	updateUniversity(ctx context.Context, id int64, in universityInput) (masterOutcome[masterChange[AdminUniversity]], error)
	deleteUniversity(ctx context.Context, id int64) (masterOutcome[AdminUniversity], error)
	createFaculty(ctx context.Context, in facultyInput) (masterOutcome[AdminFacultySnapshot], error)
	updateFaculty(ctx context.Context, id int64, in facultyInput) (masterOutcome[masterChange[AdminFacultySnapshot]], error)
	deleteFaculty(ctx context.Context, id int64) (masterOutcome[AdminFacultySnapshot], error)
	listTextbookMasters(ctx context.Context, q string) ([]AdminTextbookMaster, error)
	createTextbookMaster(ctx context.Context, in textbookMasterInput) (masterOutcome[AdminTextbookMaster], error)
	updateTextbookMaster(ctx context.Context, id int64, in textbookMasterInput) (masterOutcome[masterChange[AdminTextbookMaster]], error)
	deleteTextbookMaster(ctx context.Context, id int64) (masterOutcome[AdminTextbookMaster], error)
}

type adminMasterHandlers struct {
	store adminMasterStore
}

// ---- 大学 ----

// listUniversities は GET /api/admin/universities。
func (h *adminMasterHandlers) listUniversities(w http.ResponseWriter, r *http.Request, _ *session) {
	query := parseQuery(r.URL.RawQuery)
	q, issue := readMasterSearchQuery(query)
	if issue == nil {
		var page int
		page, issue = readPageQuery(query, adminUniversitiesMaxPage)
		if issue == nil {
			list, err := h.store.listUniversities(r.Context(), q, page)
			if err != nil {
				internalError(w, r, fmt.Errorf("admin universities: %w", err))
				return
			}
			writeJSON(w, http.StatusOK, list)
			return
		}
	}
	issue.write(w)
}

// universityDetail は GET /api/admin/universities/{id}。
func (h *adminMasterHandlers) universityDetail(w http.ResponseWriter, r *http.Request, _ *session) {
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	detail, err := h.store.universityDetail(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin university detail: %w", err))
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, universityMessages.notFound)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

// listTags は GET /api/admin/tags。
func (h *adminMasterHandlers) listTags(w http.ResponseWriter, r *http.Request, _ *session) {
	tags, err := h.store.listTags(r.Context())
	if err != nil {
		internalError(w, r, fmt.Errorf("admin tags: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

// createUniversity は POST /api/admin/universities。
func (h *adminMasterHandlers) createUniversity(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	input, in := readUniversityInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.createUniversity(r.Context(), input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin create university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "University", outcome.value.ID, "after", outcome.value)
	writeJSON(w, http.StatusCreated, outcome.value)
}

// updateUniversity は PATCH /api/admin/universities/{id}。
func (h *adminMasterHandlers) updateUniversity(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	// Node と同じく path を先に、本文を後に確かめる。
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readUniversityInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.updateUniversity(r.Context(), id, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin update university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "University", id, "before", outcome.value.before, "after", outcome.value.after)
	writeJSON(w, http.StatusOK, outcome.value.after)
}

// deleteUniversity は DELETE /api/admin/universities/{id}。
func (h *adminMasterHandlers) deleteUniversity(w http.ResponseWriter, r *http.Request, s *session) {
	// 本文は使わないが、Node（Fastify）はハンドラより先に本文を読むので、受け付けない形なら同じく 415・413 にする。
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteUniversity(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin delete university: %w", err))
		return
	}
	if rejectMasterFailure(w, universityMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "University", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 学部 ----

// createFaculty は POST /api/admin/faculties。
func (h *adminMasterHandlers) createFaculty(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	input, in := readFacultyInput(body.value(), true)
	if in.reject(w) {
		return
	}
	outcome, err := h.store.createFaculty(r.Context(), input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin create faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "Faculty", outcome.value.ID, "after", outcome.value)
	writeJSON(w, http.StatusCreated, outcome.value)
}

// updateFaculty は PATCH /api/admin/faculties/{id}。
func (h *adminMasterHandlers) updateFaculty(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readFacultyInput(body.value(), false)
	if in.reject(w) {
		return
	}
	outcome, err := h.store.updateFaculty(r.Context(), id, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin update faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "Faculty", id, "before", outcome.value.before, "after", outcome.value.after)
	writeJSON(w, http.StatusOK, outcome.value.after)
}

// deleteFaculty は DELETE /api/admin/faculties/{id}。
func (h *adminMasterHandlers) deleteFaculty(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteFaculty(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin delete faculty: %w", err))
		return
	}
	if rejectMasterFailure(w, facultyMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "Faculty", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 参考書 ----

// listTextbookMasters は GET /api/admin/textbook-masters。
func (h *adminMasterHandlers) listTextbookMasters(w http.ResponseWriter, r *http.Request, _ *session) {
	q, issue := readMasterSearchQuery(parseQuery(r.URL.RawQuery))
	if issue != nil {
		issue.write(w)
		return
	}
	masters, err := h.store.listTextbookMasters(r.Context(), q)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin textbook masters: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, masters)
}

// createTextbookMaster は POST /api/admin/textbook-masters。
func (h *adminMasterHandlers) createTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	input, in := readTextbookMasterInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.createTextbookMaster(r.Context(), input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin create textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "create", "TextbookMaster", outcome.value.ID, "after", outcome.value)
	writeJSON(w, http.StatusCreated, outcome.value)
}

// updateTextbookMaster は PATCH /api/admin/textbook-masters/{id}。
func (h *adminMasterHandlers) updateTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	body, ok := readBody(w, r, defaultBodyLimit)
	if !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	input, in := readTextbookMasterInput(body.value())
	if in.reject(w) {
		return
	}
	outcome, err := h.store.updateTextbookMaster(r.Context(), id, input)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin update textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "update", "TextbookMaster", id, "before", outcome.value.before, "after", outcome.value.after)
	writeJSON(w, http.StatusOK, outcome.value.after)
}

// deleteTextbookMaster は DELETE /api/admin/textbook-masters/{id}。
func (h *adminMasterHandlers) deleteTextbookMaster(w http.ResponseWriter, r *http.Request, s *session) {
	if _, ok := readBody(w, r, defaultBodyLimit); !ok {
		return
	}
	id, ok := masterID(w, r)
	if !ok {
		return
	}
	outcome, err := h.store.deleteTextbookMaster(r.Context(), id)
	if err != nil {
		internalError(w, r, fmt.Errorf("admin delete textbook master: %w", err))
		return
	}
	if rejectMasterFailure(w, textbookMasterMessages, outcome.failure, outcome.count) {
		return
	}
	logMasterChange(r.Context(), s.UserID, "delete", "TextbookMaster", id, "before", outcome.value)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 入力 ----
// 規則の正は src/shared/validations/master.ts の Zod のスキーマ（validate.go の冒頭を参照）。

// masterID は path の {id} を読む。判定は pathID と同じだが、Node のマスター編集は readIdParam を通さず
// idParamsSchema を sendValidationError で返すので、本文は ValidationError の形になる。
func masterID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	if !idPattern.MatchString(raw) {
		(&validationIssue{code: "invalid_format", field: "id", message: "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/"}).write(w)
		return 0, false
	}
	// 形は上で確かめたので、ここで失敗することはない。
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id, true
}

// readMasterSearchQuery は一覧の q（z.string().trim().max(100).optional()）を読む。
// Node は空の q を「絞らない」として扱う（q || undefined）ので、"" はそのまま返す。
func readMasterSearchQuery(query map[string][]string) (string, *validationIssue) {
	values, ok := query["q"]
	if !ok {
		return "", nil
	}
	// Fastify は同じキーが2つ以上あると値を配列にする。
	if len(values) != 1 {
		return "", invalidType("q", "string", []any{})
	}
	q := jsTrim(values[0])
	if codePointLength(q) > adminMasterQueryMax {
		return "", &validationIssue{code: "too_big", field: "q", message: "Too big: expected string to have <=100 characters"}
	}
	return q, nil
}

// masterNameRule は Node の name(label)：z.string().trim().min(1).max(100)。
func masterNameRule(label string) stringRule {
	return stringRule{
		trimFirst: true,
		min:       1, minMessage: label + "を入力してください",
		max: 100, maxMessage: label + "は100文字以内で入力してください",
	}
}

type universityInput struct {
	name, prefecture, typ string
}

// readUniversityInput は universityInputSchema。
func readUniversityInput(body any) (universityInput, *objectInput) {
	in := readObject(body)
	v := universityInput{
		name:       in.string("name", masterNameRule("大学名")),
		prefecture: in.string("prefecture", stringRule{checks: []stringCheck{oneOf(prefectures, "invalid_prefecture", "都道府県を選んでください")}}),
		typ:        in.enum("type", func(s string) bool { return UniversityInputType(s).Valid() }, "種別を選んでください"),
	}
	return v, in
}

type facultyInput struct {
	name     string
	examDate time.Time
	tagIDs   []int64
	// universityID は作成のときだけ（書き換えでは大学を移せない）。
	universityID int64
}

// readFacultyInput は facultyInputSchema（withUniversity なら createFacultySchema）。
// createFacultySchema は facultyInputSchema を extend したものなので、universityId は最後に確かめる。
func readFacultyInput(body any, withUniversity bool) (facultyInput, *objectInput) {
	in := readObject(body)
	var v facultyInput
	v.name = in.string("name", masterNameRule("学部名"))
	examDate := in.string("examDate", stringRule{checks: []stringCheck{
		{ok: ymdPattern.MatchString, code: "invalid_format", message: "受験日を選んでください"},
		{ok: func(s string) bool { _, ok := examDateOf(s); return ok }, code: "invalid_exam_date", message: "受験日を選んでください"},
	}})
	v.tagIDs = readTagIDs(in, "tagIds")
	if withUniversity {
		v.universityID = int64(in.number("universityId", numberRule{int: true, positive: true}))
	}
	if in.issue == nil {
		v.examDate, _ = examDateOf(examDate)
	}
	return v, in
}

// examDateOf は受験日（YYYY-MM-DD）を、Node が保存する new Date("YYYY-MM-DD")（UTC の0時）と同じ日時にする。
// JavaScript の new Date は月が 01〜12・日が 01〜31 なら受け付け、その月に無い日は繰り上げる
// （2027-02-30 は 3月2日）。time.Date も同じく繰り上げる。形（ymdPattern）は呼ぶ前に確かめておくこと。
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
func readTagIDs(in *objectInput, key string) []int64 {
	items := in.array(key, "")
	if in.issue != nil {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for i, item := range items {
		f, issue := checkNumber(in.field(key)+"."+strconv.Itoa(i), item, numberRule{int: true, positive: true})
		if issue != nil {
			in.issue = issue
			return nil
		}
		ids = append(ids, int64(f))
	}
	if len(ids) > adminFacultyTagsMax {
		in.addIssue("too_big", key, "タグは20個までです")
		return nil
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			in.addIssue("duplicate_tags", key, "同じタグが重なっています")
			return nil
		}
		seen[id] = true
	}
	return ids
}

type textbookMasterInput struct {
	name               string
	publisher, edition *string
	isbn               string
	metrics            []AdminTextbookMasterMetric
}

// readTextbookMasterInput は textbookMasterInputSchema。
func readTextbookMasterInput(body any) (textbookMasterInput, *objectInput) {
	in := readObject(body)
	var v textbookMasterInput
	v.name = in.string("name", stringRule{
		trimFirst: true,
		min:       1, minMessage: "参考書名を入力してください",
		max: 150, maxMessage: "参考書名は150文字以内で入力してください",
	})
	v.publisher = readOptionalText(in, "publisher", 100)
	v.edition = readOptionalText(in, "edition", 50)
	// isbn は z.string() を読んでから整え（transform）、整えた後の形を確かめる（refine）。
	v.isbn = normalizeISBN(in.string("isbn", stringRule{}))
	if in.issue == nil && !isbnPattern.MatchString(v.isbn) {
		in.addIssue("invalid_isbn", "isbn", "ISBN は10桁か13桁で入力してください")
	}
	v.metrics = readMetrics(in)
	return v, in
}

// readOptionalText は Node の optionalText(max)：z.string().trim().max(max).nullable().optional() で、
// 空（キーが無い・null・削ったら空）なら null。
func readOptionalText(in *objectInput, key string, max int) *string {
	o := in.optionalString(key, stringRule{trimFirst: true, max: max, maxMessage: fmt.Sprintf("%d文字以内で入力してください", max)}, true)
	if o.value == nil || *o.value == "" {
		return nil
	}
	return o.value
}

// isbnPattern は ISBN-13（数字13桁）か ISBN-10（数字9桁＋数字か X）。
var isbnPattern = regexp.MustCompile(`^(\d{13}|\d{9}[\dX])$`)

// normalizeISBN は Node と同じく、ハイフンと空白（JavaScript の \s）を取り除いて大文字にする。
// 既存の ISBN は数字だけで入っている。大文字にするのは ISBN-10 の最後の x のため
// （JavaScript の toUpperCase と Go の ToUpper は一部の文字で結果が違うが、どちらも isbnPattern に合わない）。
func normalizeISBN(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '-' || isJSWhitespace(r) {
			return -1
		}
		return r
	}, s)
	return strings.ToUpper(s)
}

// readMetrics は総量の候補：z.array(metric).min(1).max(6).refine(単位が重ならない).refine(既定がちょうど1つ)。
// 要素 → min・max → refine の順に確かめる。
func readMetrics(in *objectInput) []AdminTextbookMasterMetric {
	items := in.array("metrics", "総量を1つ以上入力してください")
	if in.issue != nil {
		return nil
	}
	metrics := make([]AdminTextbookMasterMetric, 0, len(items))
	for i, item := range items {
		m := readObjectAt(item, in.field("metrics")+"."+strconv.Itoa(i))
		metric := AdminTextbookMasterMetric{
			Unit: m.string("unit", stringRule{checks: []stringCheck{{
				ok:   func(s string) bool { return TextbookMasterInputMetricsUnit(s).Valid() },
				code: "invalid_range_unit", message: "単位を選んでください",
			}}}),
			// z.number({ error: "総量を入力してください" }).int(…).min(1, …).max(100000, …)。
			// 整数に限ると min(1) は positive と同じ（0.5 は int で先に弾かれる）。
			TotalAmount: int(m.number("totalAmount", numberRule{
				typeMessage: "総量を入力してください",
				int:         true, intMessage: "総量は整数で入力してください",
				positive: true, positiveMessage: "総量は1以上で入力してください",
				max: 100_000, maxMessage: "総量は100000以下で入力してください",
			})),
			IsDefault: m.boolean("isDefault"),
		}
		if m.issue != nil {
			in.take(m)
			return nil
		}
		metrics = append(metrics, metric)
	}
	if len(metrics) > textbookMetricsMax {
		in.addIssue("too_big", "metrics", "Too big: expected array to have <=6 items")
		return nil
	}
	units := make(map[string]bool, len(metrics))
	defaults := 0
	for _, m := range metrics {
		if units[m.Unit] {
			in.addIssue("duplicate_units", "metrics", "同じ単位が重なっています")
			return nil
		}
		units[m.Unit] = true
		if m.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		in.addIssue("default_unit_required", "metrics", "既定の単位を1つ選んでください")
		return nil
	}
	return metrics
}

// ---- ここから下は本物の DB ----

// sqlRunner は *sql.DB と *sql.Tx の両方で使う読み書き。
type sqlRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// isMySQLError は MySQL のエラー番号で見分ける。
//   - 1062 ER_DUP_ENTRY：一意制約（大学名・ISBN）に当たった
//   - 1451 ER_ROW_IS_REFERENCED_2：外部キーに参照されていて消せない
func isMySQLError(err error, number uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == number
}

const (
	mysqlDuplicateEntry  = 1062
	mysqlRowIsReferenced = 1451
)

// placeholders は IN (…)・VALUES に並べる ? を n 個つなぐ。
func placeholders(n int, one string) string {
	return strings.TrimSuffix(strings.Repeat(one+", ", n), ", ")
}

type sqlAdminMasterStore struct {
	db *sql.DB
	// universitiesChanged は大学・学部を変えて確定したあとに呼ぶ（大学を探す画面のキャッシュを捨てる）。
	universitiesChanged func()
}

const adminUniversityColumns = `u.id, u.name, u.prefecture, u.type,
  (SELECT COUNT(*) FROM Faculty f WHERE f.universityId = u.id) AS facultyCount,
  (SELECT COUNT(*) FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId WHERE f.universityId = u.id) AS goalCount`

func scanAdminUniversity(scan func(...any) error) (AdminUniversity, error) {
	var u AdminUniversity
	err := scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.FacultyCount, &u.GoalCount)
	return u, err
}

func (st *sqlAdminMasterStore) listUniversities(ctx context.Context, q string, page int) (AdminUniversityList, error) {
	list := AdminUniversityList{Universities: []AdminUniversity{}, Page: page, PageSize: adminUniversitiesPageSize}
	where, params := "", []any{}
	if q != "" {
		where, params = "WHERE u.name LIKE ?", append(params, "%"+escapeLike(q)+"%")
	}
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM University u "+where, params...).Scan(&list.Total); err != nil {
		return list, err
	}
	// #nosec G202 -- adminUniversityColumns は固定の列の並び、where は固定の条件と ? だけ。値は params で渡す
	rows, err := st.db.QueryContext(ctx,
		"SELECT "+adminUniversityColumns+" FROM University u "+where+" ORDER BY u.name ASC, u.id ASC LIMIT ? OFFSET ?",
		append(params, adminUniversitiesPageSize, (page-1)*adminUniversitiesPageSize)...)
	if err != nil {
		return list, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanAdminUniversity(rows.Scan)
		if err != nil {
			return list, err
		}
		list.Universities = append(list.Universities, u)
	}
	return list, rows.Err()
}

// findAdminUniversity は大学を1件引く。無ければ nil。
func findAdminUniversity(ctx context.Context, db sqlRunner, id int64) (*AdminUniversity, error) {
	u, err := scanAdminUniversity(db.QueryRowContext(ctx, "SELECT "+adminUniversityColumns+" FROM University u WHERE u.id = ?", id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (st *sqlAdminMasterStore) universityDetail(ctx context.Context, id int64) (*AdminUniversityDetail, error) {
	university, err := findAdminUniversity(ctx, st.db, id)
	if err != nil || university == nil {
		return nil, err
	}
	rows, err := st.db.QueryContext(ctx,
		`SELECT f.id, f.name, f.examDate,
		        (SELECT COUNT(*) FROM FinalGoal g WHERE g.facultyId = f.id) AS goalCount,
		        t.id AS tagId, t.name AS tagName
		 FROM Faculty f
		 LEFT JOIN _FacultyToTag ft ON ft.A = f.id
		 LEFT JOIN Tag t ON t.id = ft.B
		 WHERE f.universityId = ?
		 ORDER BY f.id ASC, t.id ASC`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	detail := &AdminUniversityDetail{University: *university, Faculties: []AdminFaculty{}}
	for rows.Next() {
		var (
			f        AdminFaculty
			examDate string
			tagID    *int64
			tagName  *string
		)
		if err := rows.Scan(&f.ID, &f.Name, &examDate, &f.GoalCount, &tagID, &tagName); err != nil {
			return nil, err
		}
		// 行は（学部 × タグ）の数だけ並ぶ。同じ学部の行は隣り合うので、直前と比べて束ねる。
		if n := len(detail.Faculties); n == 0 || detail.Faculties[n-1].ID != f.ID {
			f.ExamDate = examDate[:10] // DATETIME の日付の部分（Node の toISOString().slice(0, 10)）
			f.Tags = []AdminTag{}
			detail.Faculties = append(detail.Faculties, f)
		}
		if tagID != nil && tagName != nil {
			last := &detail.Faculties[len(detail.Faculties)-1]
			last.Tags = append(last.Tags, AdminTag{ID: *tagID, Name: *tagName})
		}
	}
	return detail, rows.Err()
}

func (st *sqlAdminMasterStore) listTags(ctx context.Context) ([]AdminTag, error) {
	rows, err := st.db.QueryContext(ctx, "SELECT id, name FROM Tag ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []AdminTag{}
	for rows.Next() {
		var t AdminTag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

func (st *sqlAdminMasterStore) createUniversity(ctx context.Context, in universityInput) (masterOutcome[AdminUniversity], error) {
	res, err := st.db.ExecContext(ctx,
		"INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)", in.name, in.prefecture, in.typ, nowMillis())
	if isMySQLError(err, mysqlDuplicateEntry) {
		return masterOutcome[AdminUniversity]{failure: masterDuplicate}, nil
	}
	if err != nil {
		return masterOutcome[AdminUniversity]{}, err
	}
	st.universitiesChanged()
	id, err := res.LastInsertId()
	if err != nil {
		return masterOutcome[AdminUniversity]{}, err
	}
	return foundOutcome(findAdminUniversity(ctx, st.db, id))
}

// foundOutcome は、作った・書き換えた直後に読み直した行を結果にする。直後に別の操作で消えていれば失敗にする。
func foundOutcome[T any](v *T, err error) (masterOutcome[T], error) {
	switch {
	case err != nil:
		return masterOutcome[T]{}, err
	case v == nil:
		return masterOutcome[T]{}, errors.New("書き込んだ行を読み直せなかった")
	}
	return masterOutcome[T]{value: *v}, nil
}

func (st *sqlAdminMasterStore) updateUniversity(ctx context.Context, id int64, in universityInput) (masterOutcome[masterChange[AdminUniversity]], error) {
	type outcome = masterOutcome[masterChange[AdminUniversity]]
	before, err := findAdminUniversity(ctx, st.db, id)
	if err != nil || before == nil {
		return outcome{failure: masterNotFound}, err
	}
	_, err = st.db.ExecContext(ctx, "UPDATE University SET name = ?, prefecture = ?, type = ? WHERE id = ?", in.name, in.prefecture, in.typ, id)
	if isMySQLError(err, mysqlDuplicateEntry) {
		return outcome{failure: masterDuplicate}, nil
	}
	if err != nil {
		return outcome{}, err
	}
	st.universitiesChanged()
	after, err := foundOutcome(findAdminUniversity(ctx, st.db, id))
	return outcome{value: masterChange[AdminUniversity]{before: *before, after: after.value}}, err
}

func (st *sqlAdminMasterStore) deleteUniversity(ctx context.Context, id int64) (masterOutcome[AdminUniversity], error) {
	university, err := findAdminUniversity(ctx, st.db, id)
	if err != nil || university == nil {
		return masterOutcome[AdminUniversity]{failure: masterNotFound}, err
	}
	inUse := masterOutcome[AdminUniversity]{failure: masterInUse, count: university.GoalCount}
	if university.GoalCount > 0 {
		return inUse, nil
	}
	_, err = st.db.ExecContext(ctx, "DELETE FROM University WHERE id = ?", id)
	if isMySQLError(err, mysqlRowIsReferenced) {
		return inUse, nil
	}
	if err != nil {
		return masterOutcome[AdminUniversity]{}, err
	}
	st.universitiesChanged()
	return masterOutcome[AdminUniversity]{value: *university}, nil
}

// findFacultySnapshot は学部を、監査ログと応答の形（タグは id だけ）で引く。無ければ nil。
func findFacultySnapshot(ctx context.Context, db sqlRunner, id int64) (*AdminFacultySnapshot, error) {
	var f AdminFacultySnapshot
	var examDate string
	err := db.QueryRowContext(ctx, "SELECT id, universityId, name, examDate FROM Faculty WHERE id = ?", id).
		Scan(&f.ID, &f.UniversityID, &f.Name, &examDate)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f.ExamDate = examDate[:10]
	rows, err := db.QueryContext(ctx, "SELECT B AS tagId FROM _FacultyToTag WHERE A = ? ORDER BY B ASC", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	f.TagIds = []int64{}
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, err
		}
		f.TagIds = append(f.TagIds, tagID)
	}
	return &f, rows.Err()
}

// hasFacultyNamed は、その大学に同じ名前の学部があるか（exceptID の学部を除く）。
// Faculty には (universityId, name) の一意制約が無い（seed が名前で照合している）ので、ここで重複を断る。
func hasFacultyNamed(ctx context.Context, tx *sql.Tx, universityID int64, name string, exceptID int64) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM Faculty WHERE universityId = ? AND name = ? AND id <> ? LIMIT 1",
		universityID, name, exceptID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// allTagsExist は、送られたタグがすべて Tag にあるか。
func allTagsExist(ctx context.Context, tx *sql.Tx, tagIDs []int64) (bool, error) {
	if len(tagIDs) == 0 {
		return true, nil
	}
	args := make([]any, len(tagIDs))
	for i, id := range tagIDs {
		args[i] = id
	}
	var count int
	err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM Tag WHERE id IN ("+placeholders(len(tagIDs), "?")+")", args...).Scan(&count)
	return count == len(tagIDs), err
}

// replaceTags は学部のタグを送られたものに置き換える（中間テーブルの A = Faculty.id, B = Tag.id）。
func replaceTags(ctx context.Context, tx *sql.Tx, facultyID int64, tagIDs []int64) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM _FacultyToTag WHERE A = ?", facultyID); err != nil {
		return err
	}
	if len(tagIDs) == 0 {
		return nil
	}
	args := make([]any, 0, len(tagIDs)*2)
	for _, tagID := range tagIDs {
		args = append(args, facultyID, tagID)
	}
	// #nosec G202 -- 埋め込むのは件数ぶん並べた (?, ?) だけ。値は args で渡す
	_, err := tx.ExecContext(ctx, "INSERT INTO _FacultyToTag (A, B) VALUES "+placeholders(len(tagIDs), "(?, ?)"), args...)
	return err
}

// checkFacultyInput は学部の作成・書き換えで、名前の重なりとタグの存在を確かめる。断るなら理由を返す。
func checkFacultyInput(ctx context.Context, tx *sql.Tx, universityID int64, in facultyInput, exceptID int64) (masterFailure, error) {
	taken, err := hasFacultyNamed(ctx, tx, universityID, in.name, exceptID)
	if err != nil || taken {
		return masterDuplicate, err
	}
	exist, err := allTagsExist(ctx, tx, in.tagIDs)
	if err != nil || !exist {
		return masterInvalidTags, err
	}
	return masterOK, nil
}

// トランザクションの中でキャッシュを捨てると、確定前に別のリクエストが古い一覧を読み直して置き直せる。
// 学部の作成・書き換えは、確定（commit）してから捨てる。

func (st *sqlAdminMasterStore) createFaculty(ctx context.Context, in facultyInput) (masterOutcome[AdminFacultySnapshot], error) {
	var outcome masterOutcome[AdminFacultySnapshot]
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		// 大学を押さえてから学部を足す（確かめている間に大学が消されないように）。
		var universityID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM University WHERE id = ? FOR UPDATE", in.universityID).Scan(&universityID)
		if errors.Is(err, sql.ErrNoRows) {
			outcome.failure = masterNotFound
			return nil
		}
		if err != nil {
			return err
		}
		if outcome.failure, err = checkFacultyInput(ctx, tx, in.universityID, in, 0); err != nil || outcome.failure != masterOK {
			return err
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
			in.name, in.examDate, in.universityID, nowMillis())
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := replaceTags(ctx, tx, id, in.tagIDs); err != nil {
			return err
		}
		outcome, err = foundOutcome(findFacultySnapshot(ctx, tx, id))
		return err
	})
	if err == nil && outcome.failure == masterOK {
		st.universitiesChanged()
	}
	return outcome, err
}

func (st *sqlAdminMasterStore) updateFaculty(ctx context.Context, id int64, in facultyInput) (masterOutcome[masterChange[AdminFacultySnapshot]], error) {
	var outcome masterOutcome[masterChange[AdminFacultySnapshot]]
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		before, err := findFacultySnapshot(ctx, tx, id)
		if err != nil || before == nil {
			outcome.failure = masterNotFound
			return err
		}
		if outcome.failure, err = checkFacultyInput(ctx, tx, before.UniversityID, in, id); err != nil || outcome.failure != masterOK {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE Faculty SET name = ?, examDate = ? WHERE id = ?", in.name, in.examDate, id); err != nil {
			return err
		}
		if err := replaceTags(ctx, tx, id, in.tagIDs); err != nil {
			return err
		}
		after, err := foundOutcome(findFacultySnapshot(ctx, tx, id))
		outcome.value = masterChange[AdminFacultySnapshot]{before: *before, after: after.value}
		return err
	})
	if err == nil && outcome.failure == masterOK {
		st.universitiesChanged()
	}
	return outcome, err
}

func (st *sqlAdminMasterStore) deleteFaculty(ctx context.Context, id int64) (masterOutcome[AdminFacultySnapshot], error) {
	faculty, err := findFacultySnapshot(ctx, st.db, id)
	if err != nil || faculty == nil {
		return masterOutcome[AdminFacultySnapshot]{failure: masterNotFound}, err
	}
	var goalCount int
	if err := st.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM FinalGoal WHERE facultyId = ?", id).Scan(&goalCount); err != nil {
		return masterOutcome[AdminFacultySnapshot]{}, err
	}
	inUse := masterOutcome[AdminFacultySnapshot]{failure: masterInUse, count: goalCount}
	if goalCount > 0 {
		return inUse, nil
	}
	// 中間テーブルの行は外部キーの CASCADE で一緒に消える。
	_, err = st.db.ExecContext(ctx, "DELETE FROM Faculty WHERE id = ?", id)
	if isMySQLError(err, mysqlRowIsReferenced) {
		return inUse, nil
	}
	if err != nil {
		return masterOutcome[AdminFacultySnapshot]{}, err
	}
	st.universitiesChanged()
	return masterOutcome[AdminFacultySnapshot]{value: *faculty}, nil
}

// selectAdminTextbookMasters は参考書マスターを、総量の候補と利用者の参考書の数をつけて引く（先頭200件まで）。
func selectAdminTextbookMasters(ctx context.Context, db sqlRunner, where string, args ...any) ([]AdminTextbookMaster, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT tm.id, tm.name, tm.publisher, tm.edition, tm.isbn,
		        (SELECT COUNT(*) FROM Textbook t WHERE t.masterId = tm.id) AS textbookCount,
		        m.unit, m.totalAmount, m.isDefault
		 FROM (SELECT * FROM TextbookMaster tm `+where+` ORDER BY tm.id ASC LIMIT `+strconv.Itoa(adminTextbookMastersLimit)+`) AS tm
		 LEFT JOIN TextbookMasterMetric m ON m.masterId = tm.id
		 ORDER BY tm.id ASC, m.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	masters := []AdminTextbookMaster{}
	for rows.Next() {
		var (
			tm          AdminTextbookMaster
			unit        *string
			totalAmount *int
			isDefault   *bool
		)
		if err := rows.Scan(&tm.ID, &tm.Name, &tm.Publisher, &tm.Edition, &tm.Isbn, &tm.TextbookCount, &unit, &totalAmount, &isDefault); err != nil {
			return nil, err
		}
		// 行は（参考書 × 総量の候補）の数だけ並ぶ。同じ参考書の行は隣り合うので、直前と比べて束ねる。
		if n := len(masters); n == 0 || masters[n-1].ID != tm.ID {
			tm.Metrics = []AdminTextbookMasterMetric{}
			masters = append(masters, tm)
		}
		if unit != nil {
			last := &masters[len(masters)-1]
			last.Metrics = append(last.Metrics, AdminTextbookMasterMetric{Unit: *unit, TotalAmount: *totalAmount, IsDefault: *isDefault})
		}
	}
	return masters, rows.Err()
}

func findAdminTextbookMaster(ctx context.Context, db sqlRunner, id int64) (*AdminTextbookMaster, error) {
	masters, err := selectAdminTextbookMasters(ctx, db, "WHERE tm.id = ?", id)
	if err != nil || len(masters) == 0 {
		return nil, err
	}
	return &masters[0], nil
}

func (st *sqlAdminMasterStore) listTextbookMasters(ctx context.Context, q string) ([]AdminTextbookMaster, error) {
	if q == "" {
		return selectAdminTextbookMasters(ctx, st.db, "")
	}
	pattern := "%" + escapeLike(q) + "%"
	return selectAdminTextbookMasters(ctx, st.db, "WHERE tm.name LIKE ? OR tm.publisher LIKE ? OR tm.isbn LIKE ?", pattern, pattern, pattern)
}

// replaceMetrics は総量の候補を送られたものに置き換える（入力チェックで1つ以上ある）。
func replaceMetrics(ctx context.Context, tx *sql.Tx, masterID int64, metrics []AdminTextbookMasterMetric) error {
	if _, err := tx.ExecContext(ctx, "DELETE FROM TextbookMasterMetric WHERE masterId = ?", masterID); err != nil {
		return err
	}
	now := nowMillis()
	args := make([]any, 0, len(metrics)*6)
	for _, m := range metrics {
		args = append(args, masterID, m.Unit, m.TotalAmount, m.IsDefault, now, now)
	}
	// #nosec G202 -- 埋め込むのは件数ぶん並べた (?, …) だけ。値は args で渡す
	_, err := tx.ExecContext(ctx,
		`INSERT INTO TextbookMasterMetric (masterId, unit, totalAmount, isDefault, createdAt, updatedAt)
		 VALUES `+placeholders(len(metrics), "(?, ?, ?, ?, ?, ?)"), args...)
	return err
}

func (st *sqlAdminMasterStore) createTextbookMaster(ctx context.Context, in textbookMasterInput) (masterOutcome[AdminTextbookMaster], error) {
	var outcome masterOutcome[AdminTextbookMaster]
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		now := nowMillis()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO TextbookMaster (name, publisher, edition, isbn, createdAt, updatedAt) VALUES (?, ?, ?, ?, ?, ?)`,
			in.name, in.publisher, in.edition, in.isbn, now, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := replaceMetrics(ctx, tx, id, in.metrics); err != nil {
			return err
		}
		outcome, err = foundOutcome(findAdminTextbookMaster(ctx, tx, id))
		return err
	})
	if isMySQLError(err, mysqlDuplicateEntry) {
		return masterOutcome[AdminTextbookMaster]{failure: masterDuplicate}, nil
	}
	return outcome, err
}

// updateTextbookMaster は参考書マスターを書き換える。利用者がすでに登録した参考書（Textbook）は総量を
// 自分の行に写し取っているので、ここで総量を変えても既存の利用者の参考書は変わらない（これから登録する人から効く）。
func (st *sqlAdminMasterStore) updateTextbookMaster(ctx context.Context, id int64, in textbookMasterInput) (masterOutcome[masterChange[AdminTextbookMaster]], error) {
	var outcome masterOutcome[masterChange[AdminTextbookMaster]]
	err := inTx(ctx, st.db, func(tx *sql.Tx) error {
		before, err := findAdminTextbookMaster(ctx, tx, id)
		if err != nil || before == nil {
			outcome.failure = masterNotFound
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"UPDATE TextbookMaster SET name = ?, publisher = ?, edition = ?, isbn = ?, updatedAt = ? WHERE id = ?",
			in.name, in.publisher, in.edition, in.isbn, nowMillis(), id); err != nil {
			return err
		}
		if err := replaceMetrics(ctx, tx, id, in.metrics); err != nil {
			return err
		}
		after, err := foundOutcome(findAdminTextbookMaster(ctx, tx, id))
		outcome.value = masterChange[AdminTextbookMaster]{before: *before, after: after.value}
		return err
	})
	if isMySQLError(err, mysqlDuplicateEntry) {
		return masterOutcome[masterChange[AdminTextbookMaster]]{failure: masterDuplicate}, nil
	}
	return outcome, err
}

func (st *sqlAdminMasterStore) deleteTextbookMaster(ctx context.Context, id int64) (masterOutcome[AdminTextbookMaster], error) {
	master, err := findAdminTextbookMaster(ctx, st.db, id)
	if err != nil || master == nil {
		return masterOutcome[AdminTextbookMaster]{failure: masterNotFound}, err
	}
	if master.TextbookCount > 0 {
		return masterOutcome[AdminTextbookMaster]{failure: masterInUse, count: master.TextbookCount}, nil
	}
	// 総量の候補は外部キーの CASCADE で一緒に消える。
	if _, err := st.db.ExecContext(ctx, "DELETE FROM TextbookMaster WHERE id = ?", id); err != nil {
		return masterOutcome[AdminTextbookMaster]{}, err
	}
	return masterOutcome[AdminTextbookMaster]{value: *master}, nil
}
