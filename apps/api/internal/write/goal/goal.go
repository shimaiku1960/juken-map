// Package goal は志望校（FinalGoal）への書き込みの持ち主。登録・学部の差し替え・第一志望やメモの変更・削除を
// 操作として出す。持ち主の一覧と決まりは docs/architecture.md「バックエンドの構成」（JUK-148・JUK-150）。
//
// 入力の形の確かめ（Zod と同じ 400）は入口が行い、ここには確かめ済みの値が来る。
package goal

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/opt"
)

// 操作が断る理由。
var (
	// ErrNotFound は、志望校が無いか、他人のものであること。
	ErrNotFound = errors.New("志望校が見つかりません")
	// ErrDuplicate は、同じ学部をもう志望校にしていること（DB の一意制約 userId, facultyId）。Error() は利用者に見せる文言。
	ErrDuplicate = errors.New("この学部はすでに登録されています")
)

// Goal は志望校の行（学部は付けない）。日時は Date#toISOString と同じ形。
// 項目の名前・型・並びを画面に返す形（apischema の GoalFields）とそろえ、入口が型の変換だけで返せるようにしている。
type Goal struct {
	CreatedAt     string
	FacultyID     int64
	ID            int64
	IsFirstChoice bool
	Note          *string
	Status        string
	UserID        string
}

// Patch は第一志望・メモ・ステータスの変更。Present の項目だけを書き換える。
type Patch struct {
	IsFirstChoice opt.Field[bool]
	Note, Status  opt.Field[string]
}

// Create は志望校を登録し、その ID を返す。status が nil なら "decided"。同じ学部がもうあれば ErrDuplicate。
// 無い学部は外部キーで弾かれ、Node と同じくそのままエラー（500）になる。
func Create(ctx context.Context, db *sql.DB, userID string, facultyID int64, status *string, now time.Time) (int64, error) {
	s := "decided"
	if status != nil {
		s = *status
	}
	res, err := db.ExecContext(ctx,
		"INSERT INTO FinalGoal (userId, facultyId, status, createdAt) VALUES (?, ?, ?, ?)",
		userID, facultyID, s, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return 0, ErrDuplicate
	}
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ReplaceFaculty は志望校の学部を差し替え、差し替えた後の行を返す（Node の updateGoal）。facultyID が nil なら何も変えない。
// 同じ学部の志望校が既にあると一意制約、無い学部だと外部キーで弾かれ、Node と同じくそのままエラー（500）になる。
func ReplaceFaculty(ctx context.Context, db *sql.DB, userID string, id int64, facultyID *int64) (Goal, error) {
	var updated Goal
	err := database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := find(ctx, tx, id, userID, " FOR UPDATE"); err != nil {
			return err
		}
		if facultyID != nil {
			if _, err := tx.ExecContext(ctx,
				"UPDATE FinalGoal SET facultyId = ? WHERE id = ? AND userId = ?", *facultyID, id, userID); err != nil {
				return err
			}
		}
		var err error
		updated, err = find(ctx, tx, id, userID, "")
		return err
	})
	return updated, err
}

// ApplyPatch は第一志望・メモ・ステータスのうち、送られてきたものだけを書き換える（Node の applyGoalPatch）。
//
// 第一志望は1ユーザー1校までなので、第一志望にするときは「全部外す→1件立てる」を同じトランザクションで行う。
// 分けて実行すると、途中で失敗したときに第一志望が0校の状態が残る。
func ApplyPatch(ctx context.Context, db *sql.DB, userID string, id int64, p Patch) error {
	return database.InTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := find(ctx, tx, id, userID, " FOR UPDATE"); err != nil {
			return err
		}
		var sets []string
		var args []any
		set := func(column string, present bool, value any) {
			if present {
				sets = append(sets, column+" = ?")
				args = append(args, value)
			}
		}
		set("isFirstChoice", p.IsFirstChoice.Present, p.IsFirstChoice.Value)
		set("note", p.Note.Present, p.Note.Value)
		set("status", p.Status.Present, p.Status.Value)
		if len(sets) == 0 {
			return nil
		}
		if v := p.IsFirstChoice.Value; v != nil && *v {
			if _, err := tx.ExecContext(ctx, "UPDATE FinalGoal SET isFirstChoice = FALSE WHERE userId = ?", userID); err != nil {
				return err
			}
		}
		// #nosec G202 -- 列名はこの関数に書いた固定の名前だけ（set の1つ目）。値は args で ? として渡す
		_, err := tx.ExecContext(ctx,
			"UPDATE FinalGoal SET "+strings.Join(sets, ", ")+" WHERE id = ? AND userId = ?",
			append(args, id, userID)...)
		return err
	})
}

// Delete は userID の人の志望校を消す。無いか他人のものなら ErrNotFound。
func Delete(ctx context.Context, db *sql.DB, userID string, id int64) error {
	res, err := db.ExecContext(ctx, "DELETE FROM FinalGoal WHERE id = ? AND userId = ?", id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// find は userID の人の志望校を1件読む（Node の findOwnedGoal）。無いか他人のものなら ErrNotFound。
// suffix には、変える前に行を押さえる " FOR UPDATE" を渡せる。
func find(ctx context.Context, q database.QueryRower, id int64, userID, suffix string) (Goal, error) {
	var g Goal
	err := q.QueryRowContext(ctx,
		`SELECT id, createdAt, userId, facultyId, isFirstChoice, note, status
		 FROM FinalGoal WHERE id = ? AND userId = ? LIMIT 1`+suffix, id, userID,
	).Scan(&g.ID, &g.CreatedAt, &g.UserID, &g.FacultyID, &g.IsFirstChoice, &g.Note, &g.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Goal{}, ErrNotFound
	}
	if err != nil {
		return Goal{}, err
	}
	g.CreatedAt = database.ISOFromDatetime(g.CreatedAt)
	return g, nil
}
