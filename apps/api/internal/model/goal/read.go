package goal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 志望校の読み取り（JUK-73）。志望校ページの一覧と、トップの第一志望が使う。
// 日時は Date を JSON にしたときと同じ ISO 文字列。

// University は学部の大学。
type University struct {
	CreatedAt  string
	ID         int64
	Name       string
	Prefecture string
	Type       string
}

// Faculty は志望校の学部（大学つき）。
type Faculty struct {
	CreatedAt    string
	ExamDate     string
	ID           int64
	Name         string
	University   University
	UniversityID int64
}

// Tag は学部のタグ。
type Tag struct {
	CreatedAt string
	ID        int64
	Name      string
}

// WithFaculty は学部・大学つきの志望校（第一志望・登録した直後に返す形）。
type WithFaculty struct {
	Goal
	Faculty Faculty
}

// Listed は一覧の1件。学部のタグも付く。タグが無い学部でも空のスライス（nil ではない）。
type Listed struct {
	WithFaculty
	Tags []Tag
}

// 志望校 → 学部 → 大学は「多対1」の連なりなので、JOIN しても行は増えない。
// 学部 → タグだけが1対多（中間テーブル _FacultyToTag、A = Faculty.id, B = Tag.id）。
const withFacultyColumns = `
  g.id, g.createdAt, g.userId, g.facultyId, g.isFirstChoice, g.note, g.status,
  f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
  f.createdAt AS f_createdAt, f.universityId AS f_universityId,
  u.id AS u_id, u.name AS u_name, u.prefecture AS u_prefecture,
  u.type AS u_type, u.createdAt AS u_createdAt`

const fromGoalWithFaculty = `
  FROM FinalGoal AS g
  JOIN Faculty AS f ON f.id = g.facultyId
  JOIN University AS u ON u.id = f.universityId`

// withFacultyDest は withFacultyColumns の順に Scan の受け皿を並べる。日時は文字列で受けるので、
// 読み終えたら fixDates で ISO にする。
func withFacultyDest(g *WithFaculty) []any {
	f, u := &g.Faculty, &g.Faculty.University
	return []any{
		&g.ID, &g.CreatedAt, &g.UserID, &g.FacultyID, &g.IsFirstChoice, &g.Note, &g.Status,
		&f.ID, &f.Name, &f.ExamDate, &f.CreatedAt, &f.UniversityID,
		&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.CreatedAt,
	}
}

func fixDates(g *WithFaculty) {
	g.CreatedAt = database.ISOFromDatetime(g.CreatedAt)
	g.Faculty.ExamDate = database.ISOFromDatetime(g.Faculty.ExamDate)
	g.Faculty.CreatedAt = database.ISOFromDatetime(g.Faculty.CreatedAt)
	g.Faculty.University.CreatedAt = database.ISOFromDatetime(g.Faculty.University.CreatedAt)
}

// List は志望校ページ用。学部・大学に加え、学部のタグまで引く。
// 作った順に並べ、同じ日時どうしは id で、タグは id で並べる。
func List(ctx context.Context, db *sql.DB, userID string) ([]Listed, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT"+withFacultyColumns+`,
		        t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt`+
			fromGoalWithFaculty+`
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 WHERE g.userId = ?
		 ORDER BY g.createdAt ASC, g.id ASC, t.id ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]Listed, 0)
	for rows.Next() {
		var (
			row      WithFaculty
			tagID    *int64
			tagName  *string
			tagAdded *string
		)
		if err := rows.Scan(append(withFacultyDest(&row), &tagID, &tagName, &tagAdded)...); err != nil {
			return nil, err
		}
		// ORDER BY で同じ志望校の行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(goals) == 0 || goals[len(goals)-1].ID != row.ID {
			fixDates(&row)
			goals = append(goals, Listed{WithFaculty: row, Tags: make([]Tag, 0)})
		}
		// タグが1つも無い学部は、タグの列が NULL の行が1行だけ来る
		if tagID != nil {
			last := &goals[len(goals)-1]
			last.Tags = append(last.Tags, Tag{ID: *tagID, Name: *tagName, CreatedAt: database.ISOFromDatetime(*tagAdded)})
		}
	}
	return goals, rows.Err()
}

// FindFirstChoice はトップの「第一志望」表示専用。タグは画面で使わないので引かない。無ければ nil。
func FindFirstChoice(ctx context.Context, db *sql.DB, userID string) (*WithFaculty, error) {
	// 第一志望は1ユーザー1校（Update が保つ）。DB の制約ではないので、
	// 万一2校あっても結果が揺れないよう id で並べて1件にする。
	var g WithFaculty
	err := db.QueryRowContext(ctx,
		"SELECT"+withFacultyColumns+fromGoalWithFaculty+`
		 WHERE g.userId = ? AND g.status = 'decided' AND g.isFirstChoice = TRUE
		 ORDER BY g.id ASC
		 LIMIT 1`,
		userID,
	).Scan(withFacultyDest(&g)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	fixDates(&g)
	return &g, nil
}

// findWithFaculty は登録した志望校を学部・大学つきで読む。
func findWithFaculty(ctx context.Context, db *sql.DB, id int64) (WithFaculty, error) {
	var g WithFaculty
	if err := db.QueryRowContext(ctx,
		"SELECT"+withFacultyColumns+fromGoalWithFaculty+" WHERE g.id = ?", id,
	).Scan(withFacultyDest(&g)...); err != nil {
		return WithFaculty{}, fmt.Errorf("FinalGoal %d が見つかりません: %w", id, err)
	}
	fixDates(&g)
	return g, nil
}
