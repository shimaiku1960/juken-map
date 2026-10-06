package main

import (
	"regexp"
	"strings"
)

// 監視ツール（ログの Loki・トレースの Tempo）へ出す文字列から、メールアドレスを伏せる（06 G4）。
// コードで書く値（userId など）にはメールアドレスを入れていないが、誤りの文には紛れ込む。
// 例：MySQL の重複エラー（1062）は「Duplicate entry 'a@example.com' for key ...」と値をそのまま返し、
// otelsql はそれをスパンに記録する。出どころごとに外すのではなく、出る直前にまとめて伏せる。
//
// URL のクエリに入ってエンコードされたもの（a%40example.com）も伏せる。画面の Faro と同じ（faro.ts）。
var emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+(?:@|%40)[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

func redactEmails(s string) string {
	if !strings.Contains(s, "@") && !strings.Contains(s, "%40") {
		return s
	}
	return emailPattern.ReplaceAllString(s, "[REDACTED]")
}
