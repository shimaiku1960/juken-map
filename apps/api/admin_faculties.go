package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// 管理者ページのマスター編集のうち、学部とタグ（/api/admin/faculties・/api/admin/tags）。
// 共通の部品と全体の決まりは admin_masters.go。

// ---- 入口 ----

// listTags は GET /api/admin/tags。
func (h *adminMasterHandlers) listTags(w http.ResponseWriter, r *http.Request, _ *session) {
	tags, err := h.store.listTags(r.Context())
	if err != nil {
		internalError(w, r, fmt.Errorf("admin tags: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

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

// ---- 入力 ----

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

// ---- DB ----

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
