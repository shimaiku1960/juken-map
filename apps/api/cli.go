package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/shimaiku1960/juken-map/apps/api/internal/database"
	"github.com/shimaiku1960/juken-map/apps/api/internal/feature/ops"
	"github.com/shimaiku1960/juken-map/apps/api/internal/migrate"
)

// 引数を付けて起動したときのコマンド（JUK-122）。サーバーとしては起動せず、DB に繋いで1つの操作をして終わる。
// 本番は RDS に外から繋げないので、EC2 で動いている Go のコンテナの中で実行する（distroless でシェルが無いので
// 実行ファイルを直接呼ぶ）。コンテナの DATABASE_URL をそのまま使う。
//
//	sudo docker exec juken-map-go /api incident sessions <メールアドレス>
//	sudo docker exec juken-map-go /api grant-admin --list
//
// 手元では pnpm incident・pnpm admin:grant（scripts/go-cli.sh）が .env を読んで同じものを呼ぶ。手順は docs/incident-response.md。
// 変える操作は、実行の内容を OpsAuditLog に残す（internal/write/account の OpsAudit、JUK-138）。`incident log` で見る。
// migrate（internal/migrate）だけは本番でもデプロイが1回きりのコンテナで流し、手元では pnpm db:migrate が呼ぶ。

// cliTimeout は1回の操作にかけてよい時間。DB が詰まっていても、手順の途中で固まらないように。
const cliTimeout = 30 * time.Second

// runCommand は args（os.Args[1:]）の操作をして、終了コードを返す。
func runCommand(args []string, stdout, stderr io.Writer) int {
	// migrate（internal/migrate）は繋ぐユーザー・接続の設定・かけてよい時間がほかと違うので、DB を開く前に分ける。
	if args[0] == "migrate" {
		return migrate.Run(stdout, stderr)
	}
	db, err := database.Open(os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	return ops.Run(ctx, db, args, stdout, stderr)
}
