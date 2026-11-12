package universities

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// 応答の形。モデルの値を JSON の形に直す。

// 一覧の応答の形。大学の一覧は、Go へ移したときに JSON のキーの順番（と ETag）を以前と同じにするため、生成した型
// （ExploreUniversity。キーはアルファベット順になる）を使わず手書きの型で返している。契約とのずれは universities_test.go が見る。
type exploreUniversityDTO struct {
	ID         int64               `json:"id"`
	Name       string              `json:"name"`
	Prefecture string              `json:"prefecture"`
	Type       string              `json:"type"`
	Faculties  []exploreFacultyDTO `json:"faculties"`
}

type exploreFacultyDTO struct {
	Tags []exploreTagDTO `json:"tags"`
}

type exploreTagDTO struct {
	Name string `json:"name"`
}

func exploreView(universities []university.ExploreUniversity) []exploreUniversityDTO {
	out := make([]exploreUniversityDTO, len(universities))
	for i, u := range universities {
		faculties := make([]exploreFacultyDTO, len(u.Faculties))
		for j, f := range u.Faculties {
			tags := make([]exploreTagDTO, len(f.TagNames))
			for k, name := range f.TagNames {
				tags[k] = exploreTagDTO{Name: name}
			}
			faculties[j] = exploreFacultyDTO{Tags: tags}
		}
		out[i] = exploreUniversityDTO{ID: u.ID, Name: u.Name, Prefecture: u.Prefecture, Type: u.Type, Faculties: faculties}
	}
	return out
}

func profileView(p university.Profile) apischema.UniversityDetail {
	faculties := make([]apischema.FacultyWithTags, len(p.Faculties))
	for i, f := range p.Faculties {
		tags := make([]apischema.Tag, len(f.Tags))
		for j, t := range f.Tags {
			tags[j] = apischema.Tag{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt}
		}
		faculties[i] = apischema.FacultyWithTags{
			ID: f.ID, Name: f.Name, ExamDate: f.ExamDate, CreatedAt: f.CreatedAt, UniversityID: f.UniversityID, Tags: tags,
		}
	}
	return apischema.UniversityDetail{
		ID: p.ID, Name: p.Name, Prefecture: p.Prefecture, Type: p.Type, CreatedAt: p.CreatedAt, Faculties: faculties,
	}
}
