package admin

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbookmaster"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/university"
)

// マスター編集のビュー：モデルの値を応答（apischema）の形にする。

// textbookMastersView は参考書マスターの一覧。
func textbookMastersView(masters []textbookmaster.Master) []apischema.AdminTextbookMaster {
	views := make([]apischema.AdminTextbookMaster, len(masters))
	for i, m := range masters {
		views[i] = textbookMasterView(m)
	}
	return views
}

// textbookMasterView は参考書マスターの1件（総量の候補は項目の並びが同じなので型の変換だけ）。
func textbookMasterView(m textbookmaster.Master) apischema.AdminTextbookMaster {
	metrics := make([]apischema.AdminTextbookMasterMetric, len(m.Metrics))
	for i, metric := range m.Metrics {
		metrics[i] = apischema.AdminTextbookMasterMetric(metric)
	}
	return apischema.AdminTextbookMaster{
		Edition: m.Edition, ID: m.ID, Isbn: m.Isbn, Metrics: metrics, Name: m.Name, Publisher: m.Publisher, TextbookCount: m.TextbookCount,
	}
}

// universityView は大学の行。項目はそろえてあるので型の変換だけ。
func universityView(u university.University) apischema.AdminUniversity {
	return apischema.AdminUniversity(u)
}

// universityPageView は大学の一覧の1ページ。
func universityPageView(p university.Page) apischema.AdminUniversityList {
	universities := make([]apischema.AdminUniversity, len(p.Universities))
	for i, u := range p.Universities {
		universities[i] = universityView(u)
	}
	return apischema.AdminUniversityList{Universities: universities, Total: p.Total, Page: p.Page, PageSize: p.PageSize}
}

// universityDetailView は大学詳細（学部とタグつき）。
func universityDetailView(d university.Detail) apischema.AdminUniversityDetail {
	faculties := make([]apischema.AdminFaculty, len(d.Faculties))
	for i, f := range d.Faculties {
		faculties[i] = apischema.AdminFaculty{ID: f.ID, Name: f.Name, ExamDate: f.ExamDate, GoalCount: f.GoalCount, Tags: tagsView(f.Tags)}
	}
	return apischema.AdminUniversityDetail{University: universityView(d.University), Faculties: faculties}
}

// facultyView は学部の行（タグは id だけ）。項目はそろえてあるので型の変換だけ。
func facultyView(f university.Faculty) apischema.AdminFacultySnapshot {
	return apischema.AdminFacultySnapshot(f)
}

// tagsView はタグの一覧。
func tagsView(tags []university.Tag) []apischema.AdminTag {
	views := make([]apischema.AdminTag, len(tags))
	for i, t := range tags {
		views[i] = apischema.AdminTag{ID: t.ID, Name: t.Name}
	}
	return views
}
