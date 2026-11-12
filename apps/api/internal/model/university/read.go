package university

import (
	"context"
	"database/sql"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/validation"
)

// 大学・学部の読み取り。管理画面（一覧・詳細・タグ）と、利用者の画面（大学を探す・大学詳細）が使う。

const (
	// pageSize は管理画面の一覧の1ページの件数。
	pageSize = 50
	// maxPage は管理画面の一覧の page の上限。
	maxPage = 1_000
)

// Page は管理画面の一覧の1ページ。Total は絞り込んだ全体の件数。
type Page struct {
	Universities []University
	Total        int
	Page         int
	PageSize     int
}

// Tag は学部のタグ。
type Tag struct {
	ID   int64
	Name string
}

// Detail は管理画面の大学詳細（学部とタグつき）。
type Detail struct {
	University University
	Faculties  []DetailFaculty
}

// DetailFaculty は管理画面の大学詳細の学部。ExamDate は YYYY-MM-DD、GoalCount は志望校にしている件数。
type DetailFaculty struct {
	ID        int64
	Name      string
	ExamDate  string
	GoalCount int
	Tags      []Tag
}

// List は管理画面の大学の一覧（名前順、50件ずつ）。query の q（名前の部分一致）で絞り、page でページを選ぶ。
// q・page の形が違えば *validation.Issue を返す。
func List(ctx context.Context, db *sql.DB, query map[string][]string) (Page, error) {
	q, issue := validation.SearchQuery(query, "q", searchMax)
	if issue != nil {
		return Page{}, issue
	}
	page, issue := validation.PageQuery(query, maxPage)
	if issue != nil {
		return Page{}, issue
	}
	list := Page{Universities: []University{}, Page: page, PageSize: pageSize}
	where, params := "", []any{}
	if q != "" {
		where, params = "WHERE u.name LIKE ?", append(params, "%"+database.EscapeLike(q)+"%")
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM University u "+where, params...).Scan(&list.Total); err != nil {
		return Page{}, err
	}
	// #nosec G202 -- universityColumns は固定の列の並び、where は固定の条件と ? だけ。値は params で渡す
	rows, err := db.QueryContext(ctx,
		"SELECT "+universityColumns+" FROM University u "+where+" ORDER BY u.name ASC, u.id ASC LIMIT ? OFFSET ?",
		append(params, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var u University
		if err := rows.Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.FacultyCount, &u.GoalCount); err != nil {
			return Page{}, err
		}
		list.Universities = append(list.Universities, u)
	}
	return list, rows.Err()
}

// FindDetail は管理画面の大学詳細。大学が無ければ ErrNotFound。
func FindDetail(ctx context.Context, db *sql.DB, id int64) (Detail, error) {
	university, err := findUniversity(ctx, db, id, "")
	if err != nil {
		return Detail{}, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT f.id, f.name, f.examDate,
		        (SELECT COUNT(*) FROM FinalGoal g WHERE g.facultyId = f.id) AS goalCount,
		        t.id AS tagId, t.name AS tagName
		 FROM Faculty f
		 LEFT JOIN _FacultyToTag ft ON ft.A = f.id
		 LEFT JOIN Tag t ON t.id = ft.B
		 WHERE f.universityId = ?
		 ORDER BY f.id ASC, t.id ASC`, id)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()

	detail := Detail{University: university, Faculties: []DetailFaculty{}}
	for rows.Next() {
		var (
			f        DetailFaculty
			examDate string
			tagID    *int64
			tagName  *string
		)
		if err := rows.Scan(&f.ID, &f.Name, &examDate, &f.GoalCount, &tagID, &tagName); err != nil {
			return Detail{}, err
		}
		// 行は（学部 × タグ）の数だけ並ぶ。同じ学部の行は隣り合うので、直前と比べて束ねる。
		if n := len(detail.Faculties); n == 0 || detail.Faculties[n-1].ID != f.ID {
			f.ExamDate = examDate[:10] // DATETIME の日付の部分
			f.Tags = []Tag{}
			detail.Faculties = append(detail.Faculties, f)
		}
		if tagID != nil && tagName != nil {
			last := &detail.Faculties[len(detail.Faculties)-1]
			last.Tags = append(last.Tags, Tag{ID: *tagID, Name: *tagName})
		}
	}
	return detail, rows.Err()
}

// Tags はタグの一覧（id 順）。管理画面で学部に付けるタグを選ぶのに使う。
func Tags(ctx context.Context, db *sql.DB) ([]Tag, error) {
	rows, err := db.QueryContext(ctx, "SELECT id, name FROM Tag ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}
