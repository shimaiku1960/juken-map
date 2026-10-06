// Package university は大学・学部のマスター（University・Faculty と、学部のタグ _FacultyToTag）への書き込みの持ち主。
// 管理画面のマスター編集（JUK-78）の作成・書き換え・削除を操作として出す。持ち主の一覧と決まりは
// docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 削除は「誰にも使われていない行」に限る。学部は志望校（FinalGoal）が参照していれば消せず（DB も ON DELETE RESTRICT で拒む）、
// 大学は学部が CASCADE で一緒に消えるので、配下の学部がどれか志望校に使われていれば止める。事前に数えて断り、
// 数えたあとに志望校が増えた場合も、DB の外部キーが拒んだエラーを同じ InUseError に読み替える。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。大学を探す画面のキャッシュを捨てるのも、
// 変更をログに残すのも入口（操作が成功して確定したあと）。
package university

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 操作が断る理由。入口が利用者向けの文言（404・409・400）に直す。
var (
	// ErrNotFound は、大学・学部（学部の作成では大学）が無いこと。
	ErrNotFound = errors.New("university: not found")
	// ErrDuplicate は、同じ名前の大学（DB の一意制約）か、同じ大学に同じ名前の学部があること。
	ErrDuplicate = errors.New("university: duplicate")
	// ErrInvalidTags は、送られたタグに存在しないものが混ざっていること。
	ErrInvalidTags = errors.New("university: invalid tags")
)

// InUseError は、志望校に使われているので消せないこと。Count は使っている志望校の件数（文言に入れる）。
type InUseError struct{ Count int }

func (e *InUseError) Error() string { return fmt.Sprintf("university: used by %d goals", e.Count) }

// University は管理画面の大学の行。項目の名前・型・並びを apischema の AdminUniversity とそろえ、入口が型の変換だけで返せるようにしている。
type University struct {
	FacultyCount int
	// GoalCount は配下の学部を志望校にしている件数。
	GoalCount  int
	ID         int64
	Name       string
	Prefecture string
	Type       string
}

// Faculty は学部の行（タグは id だけ）。監査ログと応答に使う。項目の並びは apischema の AdminFacultySnapshot とそろえている。
type Faculty struct {
	// ExamDate は YYYY-MM-DD。
	ExamDate     string
	ID           int64
	Name         string
	TagIds       []int64
	UniversityID int64
}

// Change は書き換えの前と後。入口は監査ログに両方を残し、応答は後を返す。
type Change[T any] struct {
	Before, After T
}

// UniversityInput は大学の作成・書き換えの値。
type UniversityInput struct {
	Name, Prefecture, Type string
}

// FacultyInput は学部の作成・書き換えの値。ExamDate はその日の 00:00 UTC。UniversityID は作成のときだけ使う（書き換えでは大学を移せない）。
type FacultyInput struct {
	Name         string
	ExamDate     time.Time
	TagIDs       []int64
	UniversityID int64
}

const universityColumns = `u.id, u.name, u.prefecture, u.type,
  (SELECT COUNT(*) FROM Faculty f WHERE f.universityId = u.id) AS facultyCount,
  (SELECT COUNT(*) FROM FinalGoal g JOIN Faculty f ON f.id = g.facultyId WHERE f.universityId = u.id) AS goalCount`

// CreateUniversity は大学を作る。同じ名前の大学があれば ErrDuplicate。
func CreateUniversity(ctx context.Context, db *sql.DB, in UniversityInput, now time.Time) (University, error) {
	res, err := db.ExecContext(ctx,
		"INSERT INTO University (name, prefecture, type, createdAt) VALUES (?, ?, ?, ?)", in.Name, in.Prefecture, in.Type, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return University{}, ErrDuplicate
	}
	if err != nil {
		return University{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return University{}, err
	}
	return findUniversity(ctx, db, id, "")
}

// UpdateUniversity は大学の名前・都道府県・種類を書き換え、前と後を返す。無ければ ErrNotFound、名前が重なれば ErrDuplicate。
func UpdateUniversity(ctx context.Context, db *sql.DB, id int64, in UniversityInput) (Change[University], error) {
	var change Change[University]
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if change.Before, err = findUniversity(ctx, tx, id, " FOR UPDATE"); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE University SET name = ?, prefecture = ?, type = ? WHERE id = ?", in.Name, in.Prefecture, in.Type, id)
		if database.IsMySQLError(err, database.DuplicateEntry) {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		change.After, err = findUniversity(ctx, tx, id, "")
		return err
	})
	return change, err
}

// DeleteUniversity は大学を（学部ごと）消し、消した行を返す。配下の学部が志望校に使われていれば *InUseError。
func DeleteUniversity(ctx context.Context, db *sql.DB, id int64) (University, error) {
	var deleted University
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if deleted, err = findUniversity(ctx, tx, id, " FOR UPDATE"); err != nil {
			return err
		}
		if deleted.GoalCount > 0 {
			return &InUseError{Count: deleted.GoalCount}
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM University WHERE id = ?", id)
		if database.IsMySQLError(err, database.RowIsReferenced) {
			return &InUseError{Count: deleted.GoalCount}
		}
		return err
	})
	return deleted, err
}

// findUniversity は大学を1件読む。無ければ ErrNotFound。suffix には " FOR UPDATE" を渡せる。
func findUniversity(ctx context.Context, q database.QueryRower, id int64, suffix string) (University, error) {
	var u University
	err := q.QueryRowContext(ctx, "SELECT "+universityColumns+" FROM University u WHERE u.id = ?"+suffix, id).
		Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.FacultyCount, &u.GoalCount)
	if errors.Is(err, sql.ErrNoRows) {
		return University{}, ErrNotFound
	}
	return u, err
}

// CreateFaculty は学部をタグつきで作る。大学が無ければ ErrNotFound、同じ大学に同じ名前があれば ErrDuplicate、
// 無いタグがあれば ErrInvalidTags。
func CreateFaculty(ctx context.Context, db *sql.DB, in FacultyInput, now time.Time) (Faculty, error) {
	var created Faculty
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		// 大学を押さえてから学部を足す（確かめている間に大学が消されないように）。
		if _, err := findUniversity(ctx, tx, in.UniversityID, " FOR UPDATE"); err != nil {
			return err
		}
		if err := checkFaculty(ctx, tx, in.UniversityID, in, 0); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, "INSERT INTO Faculty (name, examDate, universityId, createdAt) VALUES (?, ?, ?, ?)",
			in.Name, in.ExamDate, in.UniversityID, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if err := replaceTags(ctx, tx, id, in.TagIDs); err != nil {
			return err
		}
		created, err = findFaculty(ctx, tx, id, "")
		return err
	})
	return created, err
}

// UpdateFaculty は学部の名前・受験日・タグを書き換え、前と後を返す。断る理由は CreateFaculty と同じ。
func UpdateFaculty(ctx context.Context, db *sql.DB, id int64, in FacultyInput) (Change[Faculty], error) {
	var change Change[Faculty]
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if change.Before, err = findFaculty(ctx, tx, id, " FOR UPDATE"); err != nil {
			return err
		}
		if err := checkFaculty(ctx, tx, change.Before.UniversityID, in, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE Faculty SET name = ?, examDate = ? WHERE id = ?", in.Name, in.ExamDate, id); err != nil {
			return err
		}
		if err := replaceTags(ctx, tx, id, in.TagIDs); err != nil {
			return err
		}
		change.After, err = findFaculty(ctx, tx, id, "")
		return err
	})
	return change, err
}

// DeleteFaculty は学部を消し、消した行を返す。志望校に使われていれば *InUseError。
func DeleteFaculty(ctx context.Context, db *sql.DB, id int64) (Faculty, error) {
	var deleted Faculty
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		var err error
		if deleted, err = findFaculty(ctx, tx, id, " FOR UPDATE"); err != nil {
			return err
		}
		var goalCount int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM FinalGoal WHERE facultyId = ?", id).Scan(&goalCount); err != nil {
			return err
		}
		if goalCount > 0 {
			return &InUseError{Count: goalCount}
		}
		// 中間テーブルの行は外部キーの CASCADE で一緒に消える。
		_, err = tx.ExecContext(ctx, "DELETE FROM Faculty WHERE id = ?", id)
		if database.IsMySQLError(err, database.RowIsReferenced) {
			return &InUseError{Count: goalCount}
		}
		return err
	})
	return deleted, err
}

// findFaculty は学部をタグの id つきで読む。無ければ ErrNotFound。suffix には " FOR UPDATE" を渡せる。
func findFaculty(ctx context.Context, q database.Runner, id int64, suffix string) (Faculty, error) {
	var f Faculty
	var examDate string
	err := q.QueryRowContext(ctx, "SELECT id, universityId, name, examDate FROM Faculty WHERE id = ?"+suffix, id).
		Scan(&f.ID, &f.UniversityID, &f.Name, &examDate)
	if errors.Is(err, sql.ErrNoRows) {
		return Faculty{}, ErrNotFound
	}
	if err != nil {
		return Faculty{}, err
	}
	f.ExamDate = examDate[:10]
	rows, err := q.QueryContext(ctx, "SELECT B AS tagId FROM _FacultyToTag WHERE A = ? ORDER BY B ASC", id)
	if err != nil {
		return Faculty{}, err
	}
	defer rows.Close()
	f.TagIds = []int64{}
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return Faculty{}, err
		}
		f.TagIds = append(f.TagIds, tagID)
	}
	return f, rows.Err()
}

// checkFaculty は学部の作成・書き換えで、名前の重なり（exceptID の学部を除く）とタグの存在を確かめる。
// Faculty には (universityId, name) の一意制約が無い（seed が名前で照合している）ので、ここで重複を断る。
func checkFaculty(ctx context.Context, tx *sql.Tx, universityID int64, in FacultyInput, exceptID int64) error {
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM Faculty WHERE universityId = ? AND name = ? AND id <> ? LIMIT 1",
		universityID, in.Name, exceptID).Scan(&id)
	switch {
	case err == nil:
		return ErrDuplicate
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	if len(in.TagIDs) == 0 {
		return nil
	}
	args := make([]any, len(in.TagIDs))
	for i, id := range in.TagIDs {
		args[i] = id
	}
	var count int
	// #nosec G202 -- 埋め込むのは件数ぶん並べた ? だけ。値は args で渡す
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM Tag WHERE id IN ("+database.Placeholders(len(in.TagIDs), "?")+")", args...).Scan(&count); err != nil {
		return err
	}
	if count != len(in.TagIDs) {
		return ErrInvalidTags
	}
	return nil
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
	_, err := tx.ExecContext(ctx, "INSERT INTO _FacultyToTag (A, B) VALUES "+database.Placeholders(len(tagIDs), "(?, ?)"), args...)
	return err
}
