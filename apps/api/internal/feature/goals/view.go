package goals

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/goal"
)

// ビュー：モデルの結果を、openapi/openapi.yaml から生成した応答の形（apischema）に直す。
// 一覧は Goal（学部のタグつき）、第一志望と登録した直後は FirstChoiceGoal（タグは無く、キーごと出さない）。

func universityView(u goal.University) apischema.University {
	return apischema.University{CreatedAt: u.CreatedAt, ID: u.ID, Name: u.Name, Prefecture: u.Prefecture, Type: u.Type}
}

func firstChoiceView(g goal.WithFaculty) apischema.FirstChoiceGoal {
	f := g.Faculty
	return apischema.FirstChoiceGoal{
		ID: g.ID, CreatedAt: g.CreatedAt, UserID: g.UserID, FacultyID: g.FacultyID,
		IsFirstChoice: g.IsFirstChoice, Note: g.Note, Status: g.Status,
		Faculty: apischema.FacultyWithUniversity{
			ID: f.ID, Name: f.Name, ExamDate: f.ExamDate, CreatedAt: f.CreatedAt,
			UniversityID: f.UniversityID, University: universityView(f.University),
		},
	}
}

// listView は一覧の形にする。タグが無い学部でも [] を返す（画面は null を想定していない）。
func listView(goals []goal.Listed) []apischema.Goal {
	out := make([]apischema.Goal, 0, len(goals))
	for _, g := range goals {
		f := g.Faculty
		tags := make([]apischema.Tag, 0, len(g.Tags))
		for _, t := range g.Tags {
			tags = append(tags, apischema.Tag{CreatedAt: t.CreatedAt, ID: t.ID, Name: t.Name})
		}
		out = append(out, apischema.Goal{
			ID: g.ID, CreatedAt: g.CreatedAt, UserID: g.UserID, FacultyID: g.FacultyID,
			IsFirstChoice: g.IsFirstChoice, Note: g.Note, Status: g.Status,
			Faculty: apischema.FacultyWithUniversityAndTags{
				ID: f.ID, Name: f.Name, ExamDate: f.ExamDate, CreatedAt: f.CreatedAt,
				UniversityID: f.UniversityID, University: universityView(f.University), Tags: tags,
			},
		})
	}
	return out
}
