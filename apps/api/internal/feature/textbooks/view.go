package textbooks

import (
	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/model/textbook"
)

// ビュー：モデルの結果を、openapi/openapi.yaml から生成した応答の形（apischema）に直す。
// 参考書の行は項目をそろえてあるので型の変換だけ。マスターは総量の候補を入れ子に持つので組み立てる。

func listView(textbooks []textbook.Textbook) []apischema.TextbookRow {
	out := make([]apischema.TextbookRow, 0, len(textbooks))
	for _, t := range textbooks {
		out = append(out, apischema.TextbookRow(t))
	}
	return out
}

// mastersView は総量の候補が無いマスターでも "metrics": [] を返す（画面は null を想定していない）。
func mastersView(masters []textbook.Master) []apischema.TextbookMaster {
	out := make([]apischema.TextbookMaster, 0, len(masters))
	for _, m := range masters {
		metrics := make([]apischema.TextbookMasterMetric, 0, len(m.Metrics))
		for _, mm := range m.Metrics {
			metrics = append(metrics, apischema.TextbookMasterMetric(mm))
		}
		out = append(out, apischema.TextbookMaster{
			CreatedAt: m.CreatedAt, Edition: m.Edition, ID: m.ID, ISBN: m.ISBN, Metrics: metrics,
			Name: m.Name, Publisher: m.Publisher, UpdatedAt: m.UpdatedAt,
		})
	}
	return out
}
