package chaos

import (
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	chaosmodel "github.com/shimaiku1960/juken-map/apps/api/internal/model/chaos"
)

// 応答の形（ビュー）。

// experimentsView は実験の記録を画面に返す形にする。
func experimentsView(records []chaosmodel.Record, now time.Time) []apischema.ChaosExperiment {
	list := make([]apischema.ChaosExperiment, len(records))
	for i, rec := range records {
		list[i] = experimentView(rec, now)
	}
	return list
}

// experimentView は実験の記録1つを画面に返す形にする。now の時点で実行中か終わったかを決める。
func experimentView(rec chaosmodel.Record, now time.Time) apischema.ChaosExperiment {
	e := apischema.ChaosExperiment{
		ID:         rec.ID,
		Kind:       apischema.ChaosRecordKind(rec.Kind),
		Route:      rec.Route,
		Rate:       rec.Rate,
		DelayMs:    rec.DelayMs,
		StatusCode: rec.StatusCode,
		StartsAt:   iso(rec.StartsAt),
		EndsAt:     iso(rec.EndsAt),
		Status:     apischema.ChaosStatusEnded,
	}
	switch {
	case rec.StoppedAt != nil:
		stoppedAt := iso(*rec.StoppedAt)
		stoppedBy := rec.StoppedBy
		e.StoppedAt, e.StoppedBy = &stoppedAt, &stoppedBy
		e.Status = apischema.ChaosStatusStopped
	case now.Before(rec.EndsAt):
		e.Status = apischema.ChaosStatusRunning
	}
	return e
}

func iso(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}
