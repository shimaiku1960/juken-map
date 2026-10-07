// juken-map のサーバー。引数なしで起動すると API・ログイン・画面を配り、引数があれば運用のコマンド
// （incident・grant-admin・migrate）として1つの操作をして終わる。中身は internal/app にある（JUK-156）。
package main

import (
	"os"

	"github.com/shimaiku1960/juken-map/apps/api/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
