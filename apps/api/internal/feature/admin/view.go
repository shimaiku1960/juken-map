package admin

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbookmaster"
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
