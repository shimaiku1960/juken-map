# api-go

`GET /api/dashboard` と同じ応答を Go で返す、**比較実験用**のサーバー（JUK-69）。
Node（`apps/api`）と同じ DB・同じ負荷で、1リクエストあたりの CPU 時間を比べるためにある。
本番には出さず、Node を置き換えるものでもない。

## 動かし方

```sh
cd apps/api-go
set -a; source ../../.env; set +a   # DATABASE_URL と BETTER_AUTH_SECRET を読む
go run .                             # PORT を指定しなければ 8080
go test ./...
```

ログインは Node 側（Better Auth）で行い、そのセッション Cookie をそのまま使う。

```sh
curl -H "Cookie: better-auth.session_token=..." localhost:8080/api/dashboard
```

## Node と比べる

```sh
bash apps/api-go/compare-cpu.sh            # worktree のルートから。N=500 件 × 交互3回
```

負荷試験（k6）は使わない。手元で 300 RPS をかけると Mac 全体が詰まり、ほかの作業が
できなくなるうえ、ほかのアプリの影響で結果も揺れる。このスクリプトは1件ずつ順番に送り、
サーバーの CPU 時間の増分を件数で割るので、ほかの作業をしながら測れる。
分かるのは「1件の重さ」で、同時に何件さばけるか（限界 RPS）は分からない。

## ファイルの分け方

| ファイル | 役割 | Node 側で近いもの |
| --- | --- | --- |
| `main.go` | 設定の読み込み、ルートの登録、起動と停止 | `server.ts` |
| `http.go` | JSON の書き出し、アクセスログ、ヘルスチェック | Fastify 本体、`observability/logger.ts` |
| `db.go` | 接続プール、DATETIME の文字列を ISO にする | `infra/db.ts` |
| `auth.go` | セッション Cookie の署名確認と、session テーブルの照会 | `context.ts` の `requireSession`（Better Auth） |
| `dates.go` | 東京の「今日」、月初・月末、日付のずらし | `src/shared/date.ts` |
| `dashboard.go` | 応答の型、3本の SQL を同時に流して組み立てる | `services/dashboard-service.ts` ほか |

Go ではフォルダ1つが1つのパッケージで、ファイルの分け方はコンパイル結果に関係しない。
上の分け方は、読む人が探しやすいようにしているだけ。

## Node と揃えていること

比べたいのは言語・ランタイムの差なので、それ以外の条件を揃えている。

- SQL は同じ列・同じ条件・同じ並び。応答に使わない列（`userId`・`createdAt` など）も同じく読む
- 接続プールの上限は 15（`connectionLimit` と同じ）
- 値の `?` への埋め込みはドライバ側で行う（`InterpolateParams`）。1クエリ1往復
- DATETIME は文字列のまま受け取り、組み替えて ISO にする（Node の `selectDateStrings`）
- 1リクエストにつき JSON のアクセスログを1行出す
- セッションは session と user を JOIN して1回で引く（Better Auth と同じ）

揃えていないこと：応答の圧縮（Go 側は持たない。比べるときは `ACCEPT_ENCODING=identity`）、
セッションの有効期限の延長（Better Auth は古くなったセッションを更新するが、Go 側は読むだけ）、
OpenTelemetry（手元の Node でも無効）。
