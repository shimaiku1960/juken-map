package university

import (
	"context"
	"database/sql"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
)

// 利用者の画面の読み取り（JUK-73）。大学を探す画面の一覧（全員に同じもの）と、大学詳細。
// 日時は Date を JSON にしたときと同じ ISO 文字列。

// ExploreUniversity は大学を探す画面の一覧の1件。画面が使うのは大学の列と「学部ごとのタグ名」だけ。
type ExploreUniversity struct {
	ID         int64
	Name       string
	Prefecture string
	Type       string
	Faculties  []ExploreFaculty
}

// ExploreFaculty は一覧の学部。タグが無い学部でも空のスライス（nil ではない）。
type ExploreFaculty struct {
	TagNames []string
}

// Profile は大学詳細（学部とタグつき）。学部は志望校の学部と違い、大学を入れ子にしない。
type Profile struct {
	ID         int64
	Name       string
	Prefecture string
	Type       string
	CreatedAt  string
	Faculties  []ProfileFaculty
}

// ProfileFaculty は大学詳細の学部。
type ProfileFaculty struct {
	ID           int64
	Name         string
	ExamDate     string
	CreatedAt    string
	UniversityID int64
	Tags         []ProfileTag
}

// ProfileTag は大学詳細の学部のタグ。
type ProfileTag struct {
	ID        int64
	Name      string
	CreatedAt string
}

// ListForExplore は大学を探す画面の一覧（名前順）。大学 → 学部 → タグを LEFT JOIN 1本で取り、入れ子へ詰め直す。
// 行は（大学 × 学部 × タグ）の数だけ並ぶ。
func ListForExplore(ctx context.Context, db *sql.DB) ([]ExploreUniversity, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, f.id AS facultyId, t.name AS tagName
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 ORDER BY u.name ASC, u.id ASC, f.id ASC, t.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	universities := make([]ExploreUniversity, 0)
	var lastFacultyID *int64
	for rows.Next() {
		var (
			u         ExploreUniversity
			facultyID *int64
			tagName   *string
		)
		if err := rows.Scan(&u.ID, &u.Name, &u.Prefecture, &u.Type, &facultyID, &tagName); err != nil {
			return nil, err
		}
		// ORDER BY で同じ大学・同じ学部の行が隣り合うので、直前の要素と比べるだけで束ねられる。
		if len(universities) == 0 || universities[len(universities)-1].ID != u.ID {
			u.Faculties = make([]ExploreFaculty, 0)
			universities = append(universities, u)
			lastFacultyID = nil
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if facultyID == nil {
			continue
		}
		last := &universities[len(universities)-1]
		if lastFacultyID == nil || *lastFacultyID != *facultyID {
			last.Faculties = append(last.Faculties, ExploreFaculty{TagNames: make([]string, 0)})
			lastFacultyID = facultyID
		}
		if tagName != nil {
			f := &last.Faculties[len(last.Faculties)-1]
			f.TagNames = append(f.TagNames, *tagName)
		}
	}
	return universities, rows.Err()
}

// FindProfile は大学詳細。学部と、絞り込みに使うタグまで一度に引く。大学が無ければ ErrNotFound。
func FindProfile(ctx context.Context, db *sql.DB, id int64) (Profile, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT u.id, u.name, u.prefecture, u.type, u.createdAt,
		        f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
		        f.createdAt AS f_createdAt, f.universityId AS f_universityId,
		        t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt
		 FROM University AS u
		 LEFT JOIN Faculty AS f ON f.universityId = u.id
		 LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
		 LEFT JOIN Tag AS t ON t.id = ft.B
		 WHERE u.id = ?
		 ORDER BY f.id ASC, t.id ASC`,
		id,
	)
	if err != nil {
		return Profile{}, err
	}
	defer rows.Close()

	var profile *Profile
	for rows.Next() {
		var (
			u                            Profile
			fID, fUniversityID, tID      *int64
			fName, fExamDate, fCreatedAt *string
			tName, tCreatedAt            *string
		)
		if err := rows.Scan(
			&u.ID, &u.Name, &u.Prefecture, &u.Type, &u.CreatedAt,
			&fID, &fName, &fExamDate, &fCreatedAt, &fUniversityID,
			&tID, &tName, &tCreatedAt,
		); err != nil {
			return Profile{}, err
		}
		if profile == nil {
			u.CreatedAt = database.ISOFromDatetime(u.CreatedAt)
			u.Faculties = make([]ProfileFaculty, 0)
			profile = &u
		}
		// 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
		if fID == nil {
			continue
		}
		if n := len(profile.Faculties); n == 0 || profile.Faculties[n-1].ID != *fID {
			profile.Faculties = append(profile.Faculties, ProfileFaculty{
				ID:           *fID,
				Name:         *fName,
				ExamDate:     database.ISOFromDatetime(*fExamDate),
				CreatedAt:    database.ISOFromDatetime(*fCreatedAt),
				UniversityID: *fUniversityID,
				Tags:         make([]ProfileTag, 0),
			})
		}
		if tID != nil {
			f := &profile.Faculties[len(profile.Faculties)-1]
			f.Tags = append(f.Tags, ProfileTag{ID: *tID, Name: *tName, CreatedAt: database.ISOFromDatetime(*tCreatedAt)})
		}
	}
	if err := rows.Err(); err != nil {
		return Profile{}, err
	}
	if profile == nil {
		return Profile{}, ErrNotFound
	}
	return *profile, nil
}
