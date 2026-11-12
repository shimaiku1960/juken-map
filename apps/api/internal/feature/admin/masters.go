package admin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/shimaiku1960/juken-map/apps/api/internal/httpx"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 管理者ページのマスター編集（/admin/masters、JUK-78）のコントローラー。どれも rt.Admin（管理者＋2段階認証を通したセッションだけ）で登録する。
//
// 入力の決まり・読み書き・「使われている行は消さない」決まりはモデル（internal/model/university・internal/model/textbookmaster）にあり（JUK-224）、
// ここは HTTP と、モデルが断った理由を status と文言に直すことだけを持つ。応答の形は view.go。
//
// 大学・学部を変えたら、大学を探す画面の一覧のキャッシュ（internal/feature/universities）を、
// 参考書マスターを変えたら GET /api/textbook-masters のキャッシュを捨てる。どちらも成功して確定したあとだけ
// （トランザクションの中で捨てると、確定前に別のリクエストが古い一覧を読み直して置き直せる）。
// 変更はすべて構造化ログ「admin master change」に「誰が・何を・前→後」で残す（Grafana の Loki で追える）。
//
// ファイルは対象ごとに分ける（JUK-135）。
//   - universities.go：大学（/api/admin/universities）
//   - faculties.go：学部とタグ（/api/admin/faculties・/api/admin/tags）
//   - textbook_masters.go：参考書マスター（/api/admin/textbook-masters）
//
// このファイルには、3つが共通で使うもの（断ったときの文言・変更の記録・path の id の読み方）を置く。

// masterMessages は断ったときの文言。inUse は件数を %d で入れる。
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

type MasterHandlers struct {
	db *sql.DB
	// universitiesChanged は大学・学部を変えて確定したあとに呼ぶ（大学を探す画面のキャッシュを捨てる）。
	universitiesChanged func()
	// textbookMastersChanged は参考書マスターを変えて確定したあとに呼ぶ（GET /api/textbook-masters のキャッシュを捨てる）。
	textbookMastersChanged func()
}

// NewMasterHandlers の universitiesChanged・textbookMastersChanged は、変更を確定したあとに
// ほかの入口のキャッシュを捨てるために呼ぶ。
func NewMasterHandlers(db *sql.DB, universitiesChanged, textbookMastersChanged func()) *MasterHandlers {
	return &MasterHandlers{db: db, universitiesChanged: universitiesChanged, textbookMastersChanged: textbookMastersChanged}
}

// logMasterChange は、誰が・何を・前→後に変えたかを残す（info）。
// detail は "before", 値, "after", 値 のように並べる。
func logMasterChange(ctx context.Context, adminID, action, table string, id int64, detail ...any) {
	attrs := append([]any{"adminId", adminID, "action", action, "table", table, "id", id}, detail...)
	slog.InfoContext(ctx, "admin master change", attrs...)
}

// masterID は path の {id} を読む。判定は httpx.PathID と同じだが、マスター編集の契約（openapi.yaml）では
// 断ったときの本文が ValidationError の形になっている。
func masterID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := r.PathValue("id")
	if !httpx.IDPattern.MatchString(raw) {
		httpx.WriteValidationIssue(w, &validation.Issue{Code: "invalid_format", Field: "id", Message: "Invalid string: must match pattern /^[1-9][0-9]{0,14}$/"})
		return 0, false
	}
	// 形は上で確かめたので、ここで失敗することはない。
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id, true
}

// writeUniversityError は、大学・学部のモデルが断った理由を status と文言（m）にして送り、送ったら true を返す。
// 想定外の失敗は 500。err が nil なら何もしない。
func writeUniversityError(w http.ResponseWriter, r *http.Request, m masterMessages, op string, err error) bool {
	var inUse *university.InUseError
	switch {
	case err == nil:
		return false
	case httpx.RejectIssue(w, err):
	case errors.Is(err, university.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, m.notFound)
	case errors.Is(err, university.ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, m.duplicate)
	case errors.Is(err, university.ErrInvalidTags):
		httpx.WriteError(w, http.StatusBadRequest, "存在しないタグが含まれています")
	case errors.As(err, &inUse):
		httpx.WriteError(w, http.StatusConflict, fmt.Sprintf(m.inUse, inUse.Count))
	default:
		httpx.InternalError(w, r, fmt.Errorf("%s: %w", op, err))
	}
	return true
}
