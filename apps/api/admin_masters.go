package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/shimaiku1960/juken-map/apps/api/internal/write/university"
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
// 大学・学部の書き込みは持ち主の internal/write/university にあり（JUK-154）、store はその操作を呼んで結果を masterOutcome に直す。
//
// 大学・学部を変えたら、大学を探す画面の一覧のキャッシュ（universities.go）を捨てる。成功したときだけ。
// 変更はすべて構造化ログ「admin master change」に「誰が・何を・前→後」で残す（Node と同じ項目。Grafana の Loki で追える）。
//
// ファイルは対象ごとに分け、それぞれに入口・入力・DB をまとめる（JUK-135）。
//   - admin_universities.go：大学（/api/admin/universities）
//   - admin_faculties.go：学部とタグ（/api/admin/faculties・/api/admin/tags）
//   - admin_textbook_masters.go：参考書マスター（/api/admin/textbook-masters）
//
// このファイルには、3つが共通で使うもの（上限・断ったときの返し方・変更の記録・store の定義・入力と DB の部品）を置く。

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

// ---- ここから下は本物の DB ----

type sqlAdminMasterStore struct {
	db *sql.DB
	// universitiesChanged は大学・学部を変えて確定したあとに呼ぶ（大学を探す画面のキャッシュを捨てる）。
	universitiesChanged func()
	// textbookMastersChanged は参考書マスターを変えて確定したあとに呼ぶ（GET /api/textbook-masters のキャッシュを捨てる）。
	textbookMastersChanged func()
}

// universityOutcome は大学・学部の持ち主（internal/write/university）の操作の結果を masterOutcome にする。
// 断った理由は failure に、想定外の失敗は error にする。成功したら（確定したあとに）大学を探す画面のキャッシュを捨てる。
// トランザクションの中で捨てると、確定前に別のリクエストが古い一覧を読み直して置き直せる。
func universityOutcome[T any](st *sqlAdminMasterStore, v T, err error) (masterOutcome[T], error) {
	var inUse *university.InUseError
	switch {
	case err == nil:
		st.universitiesChanged()
		return masterOutcome[T]{value: v}, nil
	case errors.Is(err, university.ErrNotFound):
		return masterOutcome[T]{failure: masterNotFound}, nil
	case errors.Is(err, university.ErrDuplicate):
		return masterOutcome[T]{failure: masterDuplicate}, nil
	case errors.Is(err, university.ErrInvalidTags):
		return masterOutcome[T]{failure: masterInvalidTags}, nil
	case errors.As(err, &inUse):
		return masterOutcome[T]{failure: masterInUse, count: inUse.Count}, nil
	}
	return masterOutcome[T]{}, err
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
