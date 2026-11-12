// Package study は学習記録・予定とダッシュボードの入口（/api/study-logs・/api/study-plans・GET /api/dashboard）。
// MVC のコントローラーにあたり、HTTP だけを受け持つ（JSON の読み書き・セッション・状態コード）。
// 入力の決まり・業務の決まり・読み書きの SQL はモデルの internal/model/studyrecord にある（JUK-219）。
// ダッシュボードは一覧の API と同じ読み取りを使うので、別の feature に分けずここに置く。
package study

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/studyrecord"
)

// モデルの結果を応答の形（apischema、契約は openapi/openapi.yaml）へ直す。MVC のビューにあたる。
// 項目の並びまで同じ型は型変換（apischema.StudyLogRow(log) など）で直し、入れ子のあるものだけここで組み立てる。

func textbookView(tb *studyrecord.Textbook) *apischema.Textbook {
	if tb == nil {
		return nil
	}
	v := apischema.Textbook(*tb)
	return &v
}

func logsView(logs []studyrecord.ListedLog) []apischema.StudyLog {
	out := make([]apischema.StudyLog, len(logs))
	for i, l := range logs {
		out[i] = apischema.StudyLog{
			ID: l.ID, Date: l.Date, Subject: l.Subject, Minutes: l.Minutes, TextbookID: l.TextbookID,
			RangeStart: l.RangeStart, RangeEnd: l.RangeEnd, RangeUnit: l.RangeUnit, Memo: l.Memo,
			StudyPlanID: l.StudyPlanID, Textbook: textbookView(l.Textbook),
		}
	}
	return out
}

func plansView(plans []studyrecord.ListedPlan) []apischema.StudyPlan {
	out := make([]apischema.StudyPlan, len(plans))
	for i, p := range plans {
		out[i] = apischema.StudyPlan{
			ID: p.ID, Date: p.Date, Content: p.Content, Subject: p.Subject, Done: p.Done, TextbookID: p.TextbookID,
			RangeStart: p.RangeStart, RangeEnd: p.RangeEnd, RangeUnit: p.RangeUnit, StudyLogID: p.StudyLogID,
			Textbook: textbookView(p.Textbook),
		}
	}
	return out
}

func dailyView(daily []studyrecord.DailyMinutes) []apischema.DailyStudyMinutes {
	out := make([]apischema.DailyStudyMinutes, len(daily))
	for i, d := range daily {
		out[i] = apischema.DailyStudyMinutes(d)
	}
	return out
}

func logWithTextbookView(l studyrecord.LogWithTextbook) apischema.StudyLogWithTextbook {
	v := apischema.StudyLogWithTextbook{
		CreatedAt: l.CreatedAt, Date: l.Date, ID: l.ID, Memo: l.Memo, Minutes: l.Minutes,
		RangeEnd: l.RangeEnd, RangeStart: l.RangeStart, RangeUnit: l.RangeUnit, StudyPlanID: l.StudyPlanID,
		Subject: l.Subject, TextbookID: l.TextbookID, UpdatedAt: l.UpdatedAt, UserID: l.UserID,
	}
	if l.Textbook != nil {
		tb := apischema.TextbookRow(*l.Textbook)
		v.Textbook = &tb
	}
	return v
}
