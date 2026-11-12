// Package goal は志望校（FinalGoal）のモデル。入力の決まり・読み取り・書き込みを持ち、HTTP を知らない。
// 線引きは docs/architecture.md「モデル・コントローラー・ビュー」（JUK-219・JUK-222）、
// 持ち主の決まりは同じ文書の「持ち主の一覧」（JUK-148・JUK-150）。
package goal

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 操作が断る理由。入力の誤りは別に *validation.Issue で返す。
var (
	// ErrNotFound は、志望校が無いか、他人のものであること。
	ErrNotFound = errors.New("志望校が見つかりません")
	// ErrDuplicate は、同じ学部をもう志望校にしていること（DB の一意制約 userId, facultyId）。Error() は利用者に見せる文言。
	ErrDuplicate = errors.New("この学部はすでに登録されています")
)

// Goal は志望校の行（学部は付けない）。日時は Date#toISOString と同じ形。
// 項目の名前・型・並びを画面に返す形（apischema の GoalFields）とそろえ、ビューが型の変換だけで返せるようにしている。
type Goal struct {
	CreatedAt     string
	FacultyID     int64
	ID            int64
	IsFirstChoice bool
	Note          *string
	Status        string
	UserID        string
}

// Create は本文（goalSchema）を確かめて志望校を登録し、学部・大学つきで返す（画面が学部名・大学名を出すため）。
// status を省くと "decided"。同じ学部がもうあれば ErrDuplicate。
// 無い学部は外部キーで弾かれ、そのままエラー（500）になる。
func Create(ctx context.Context, db *sql.DB, userID string, body any, now time.Time) (WithFaculty, error) {
	in, err := decodeCreate(body)
	if err != nil {
		return WithFaculty{}, err
	}
	status := "decided"
	if in.status != nil {
		status = *in.status
	}
	res, err := db.ExecContext(ctx,
		"INSERT INTO FinalGoal (userId, facultyId, status, createdAt) VALUES (?, ?, ?, ?)",
		userID, in.facultyID, status, now)
	if database.IsMySQLError(err, database.DuplicateEntry) {
		return WithFaculty{}, ErrDuplicate
	}
	if err != nil {
		return WithFaculty{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return WithFaculty{}, err
	}
	return findWithFaculty(ctx, db, id)
}

// Replace は本文（updateGoalSchema）を確かめて志望校の学部を差し替え、差し替えた後の行を返す。
// 入力チェックは自分の志望校かを確かめるより先。学部を省くと何も変えない。
// 同じ学部の志望校が既にあると一意制約、無い学部だと外部キーで弾かれ、そのままエラー（500）になる。
func Replace(ctx context.Context, db *sql.DB, userID string, id int64, body any) (Goal, error) {
	facultyID, err := decodeReplace(body)
	if err != nil {
		return Goal{}, err
	}
	var updated Goal
	err = database.InTx(ctx, db, func(tx *sql.Tx) error {
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

// Update は本文（patchGoalSchema）を確かめ、第一志望・メモ・ステータスのうち送られてきたものだけを書き換える。
// 入力チェックは自分の志望校かを確かめるより先。
//
// 第一志望は1ユーザー1校までなので、第一志望にするときは「全部外す→1件立てる」を同じトランザクションで行う。
// 分けて実行すると、途中で失敗したときに第一志望が0校の状態が残る。
func Update(ctx context.Context, db *sql.DB, userID string, id int64, body any) error {
	p, err := decodePatch(body)
	if err != nil {
		return err
	}
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
		set("isFirstChoice", p.isFirstChoice.Present, p.isFirstChoice.Value)
		set("note", p.note.Present, p.note.Value)
		set("status", p.status.Present, p.status.Value)
		if len(sets) == 0 {
			return nil
		}
		if v := p.isFirstChoice.Value; v != nil && *v {
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

// find は userID の人の志望校を1件読む。無いか他人のものなら ErrNotFound。
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
