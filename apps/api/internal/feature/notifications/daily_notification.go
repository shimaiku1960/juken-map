package notifications

import (
	"fmt"
	"strings"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/apischema"
	"github.com/shimaiku1960/juken-map/apps/api/internal/dates"
	"github.com/shimaiku1960/juken-map/apps/api/internal/site"
)

// 毎日の通知の文面と、「日本時間の今日」の範囲（JUK-74）。
// Node の domain/dailyNotification.ts と同じ文面・同じ規則。DB も外部サービスも使わない。

type planSummary struct {
	Done         bool
	Content      *string
	Subject      *string
	TextbookName *string
}

type dailyMessage struct {
	Subject string
	HTML    string
	Text    string
}

// tokyoDay は日本時間の「今日」。date は YYYY-MM-DD、start と end はその日の 0時（日本時間）と翌日の 0時。
// 予定・実績の date は「その日の 00:00 UTC」で入っているので、[start, end) で今日の分だけを拾える。
// start は配信記録（NotificationDelivery.date）にも入れる。Node と同じ値なので、どちらが送っても
// 同じ日・同じ時間帯の2回目を UNIQUE 制約で弾ける。
type tokyoDay struct {
	date       string
	start, end time.Time
}

func tokyoDateRange(now time.Time) tokyoDay {
	y, m, d := now.In(dates.Tokyo).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, dates.Tokyo).UTC()
	return tokyoDay{date: start.In(dates.Tokyo).Format(time.DateOnly), start: start, end: start.Add(24 * time.Hour)}
}

// escapeHTML は Node の escapeHtml と同じ5文字を置き換える（html.EscapeString は ' を &#39; にするので使わない）。
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#039;")

func escapeHTML(s string) string {
	return htmlEscaper.Replace(s)
}

// planLabel は予定の見出し。参考書名 → 内容 → 科目 → 「学習予定」の順に、先にあるものを使う。
func planLabel(p planSummary) string {
	for _, v := range []*string{p.TextbookName, p.Content, p.Subject} {
		if v != nil {
			return *v
		}
	}
	return "学習予定"
}

func buildDailyNotification(slot apischema.NotificationSlot, nickname string, plans []planSummary, logMinutes []int64) dailyMessage {
	safeName := escapeHTML(nickname)

	if slot == apischema.NotificationSlotMorning {
		planText := fmt.Sprintf("今日の予定は%d件です。", len(plans))
		if len(plans) == 0 {
			planText = "今日はまだ予定がありません。まず1つだけ決めてみましょう。"
		}
		// 本文に並べるのは先頭の5件まで
		shown := plans[:min(len(plans), 5)]
		var planList, textList strings.Builder
		if len(shown) > 0 {
			planList.WriteString("<ul>")
			for _, p := range shown {
				planList.WriteString("<li>" + escapeHTML(planLabel(p)) + "</li>")
				textList.WriteString("\n・" + planLabel(p))
			}
			planList.WriteString("</ul>")
		}
		return dailyMessage{
			Subject: "【受験マップ】今日の学習予定",
			HTML: "<p>" + safeName + "さん、おはようございます。</p><p>" + planText + "</p>" + planList.String() +
				`<p><a href="` + site.URL + `/dashboard">今日の学習を始める</a></p>`,
			Text: nickname + "さん、おはようございます。\n" + planText + textList.String() +
				"\n\n今日の学習を始める\n" + site.URL + "/dashboard",
		}
	}

	var totalMinutes int64
	for _, m := range logMinutes {
		totalMinutes += m
	}
	completed := 0
	for _, p := range plans {
		if p.Done {
			completed++
		}
	}
	achievement := "今日は予定を入れない一日でした。"
	if len(plans) > 0 {
		achievement = fmt.Sprintf("予定%d件中%d件を完了しました。", len(plans), completed)
	}
	effort := "今日はまだ学習記録がありません。短い時間でも、記録から再開できます。"
	if totalMinutes > 0 {
		effort = fmt.Sprintf("今日は%d分の学習を記録しました。", totalMinutes)
	}
	return dailyMessage{
		Subject: "【受験マップ】今日の学習振り返り",
		HTML: "<p>" + safeName + "さん、今日もおつかれさまでした。</p><p>" + effort + "</p><p>" + achievement + "</p>" +
			`<p><a href="` + site.URL + `/dashboard">今日を振り返る</a></p>`,
		Text: nickname + "さん、今日もおつかれさまでした。\n" + effort + "\n" + achievement +
			"\n\n今日を振り返る\n" + site.URL + "/dashboard",
	}
}
