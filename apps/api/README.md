# apps/api（Go のサーバー）

業務の API を受けるサーバー（JUK-70）。Node の API（当時の `apps/api`。JUK-131 で Go がこの名前を引き継いだ）を1本ずつ移し、移し終えたものは
Node から消した（JUK-84）。ダッシュボード・学習記録と予定・志望校・参考書・大学・通知設定・プロフィール・
毎日の通知・LINE 連携・管理画面・登録の計測・CSP の報告・シミュレーションを、書き込みも含めて Go が返す。
本番では nginx がこれらのパスを Go へ振り分ける（JUK-72、下の「本番」）。

ログイン（`/api/auth/*`）も Better Auth（Node）から移し、Go で自作した（JUK-115、`internal/feature/auth/`）。
判定の基準は dev-standards の `targets/10_authentication.md`（認証 基準）で、コメントの B1・C3 などはその項目。
画面（SPA・SSG の HTML・静的ファイル）・sitemap・ブログの中継（`/api/blog`）・`/line/settings` も Node から移した
（JUK-111、`internal/spa/`・`internal/feature/blog/`）。本番で動くアプリのコンテナは Go だけ（JUK-109 で Node のコンテナを外した）。

どのファイルに何があるかは、下の「[ファイルの分け方](#ファイルの分け方)」にまとめた。

## 動かし方

ふだんの開発では、リポジトリのルートの `pnpm dev` が Go も起動する（`dev:go`、4100番）。
画面からは nginx（`dev:proxy`）が本番と同じ振り分けで Go へ送る（JUK-96、`infra/nginx/README.md`）。
Go だけを動かすときは次のとおり。

```sh
cd apps/api
set -a; source ../../.env; set +a   # DATABASE_URL と BETTER_AUTH_SECRET などを読む
go run ./cmd/api                     # PORT を指定しなければ 8080
go test ./...
```

CI の go のジョブと同じ静的な確かめは次のとおり。lint の設定は `.golangci.yml`（JUK-136）で、
版は CI（`.github/workflows/ci.yml`）とそろえる。

```sh
gofmt -l .                                     # 何か出たら gofmt -w . で整形する
go vet ./... && go vet -tags dbtest ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./...   # 初回はビルドに1分ほどかかる
```

ログは本番と同じ1行1つの JSON。人が読みたいときは、pino と同じ形なので pino-pretty に通す。

```sh
go run ./cmd/api | pnpm exec pino-pretty
```

ログインした Cookie（`__Host-jm_session`）をそのまま付ければ、ほかの API も叩ける。

```sh
curl -c jar -H "Content-Type: application/json" -d '{"email":"…","password":"…"}' localhost:8080/api/auth/sign-in
curl -b jar localhost:8080/api/dashboard
```

| 環境変数 | 既定 | 意味 |
| --- | --- | --- |
| `PORT` | 8080 | 待ち受けるポート |
| `METRICS_PORT` | なし | 指定したときだけ、このポートで `/metrics` を出す |
| `OVERLOAD_MAX_IN_FLIGHT` | 160 | 同時に処理する件数の上限。超えたら 503 |
| `LOG_FILE` | なし | 指定したときだけ、同じ JSON のログをこのファイルにも書く（手元の Alloy → Loki 用。相対パスは起動したディレクトリから） |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | なし | 指定したときだけ、トレースをこの送り先（OTLP/HTTP、`/v1/traces` を足す）へ送る。手元は Tempo、本番は Alloy（JUK-126） |
| `LOG_LEVEL` | info | pino と同じ名前（debug・info・warn・error） |
| `NODE_ENV` | なし | `production` のとき reqId を UUID のまま出す（開発は8文字） |
| `DATABASE_URL` | なし | 必須。`mysql://…` の形（`db/` の道具と同じ） |
| `BETTER_AUTH_SECRET` | なし | 必須。2段階認証の秘密を暗号化する鍵（版 v0）をここから導く（`AUTH_TOTP_KEYS` が無いとき）。名前は Better Auth の名残 |
| `AUTH_TOTP_KEYS` | なし | 2段階認証の秘密を暗号化する鍵。`版:base64の32バイト` をカンマで並べ、先頭が今の版。鍵を作り直すときは新しい版を先頭に足す（古い版も残す） |
| `AUTH_HASH_CONCURRENCY` | 2 | パスワードのハッシュ（Argon2id、1回 19MiB）を同時に計算する数 |
| `AUTH_GOOGLE_ID`・`AUTH_GOOGLE_SECRET` | なし | Google ログイン。どちらか空なら Google ログインは無い（404） |
| `AUTH_GITHUB_ID`・`AUTH_GITHUB_SECRET` | なし | GitHub ログイン。どちらか空なら GitHub ログインは無い（404） |
| `ADMIN_NOTIFICATION_EMAIL` | なし | 新しい利用者を知らせる宛先。空ならログに残すだけ |
| `DAILY_NOTIFICATION_SECRET` | なし | 毎日の通知の入口の共有トークン。空なら、その入口は必ず 401 |
| `RESEND_API_KEY` | なし | メール（毎日の通知・確認・再設定・本人への知らせ）を送る Resend のキー |
| `RESEND_BASE_URL` | `https://api.resend.com` | Resend の送り先。手元の比較で偽のサーバーへ向けるときだけ変える（Resend の SDK と同じ名前） |
| `LINE_CHANNEL_ACCESS_TOKEN` | なし | 毎日の通知を LINE で送るトークン |
| `LINE_API_BASE` | `https://api.line.me/v2/bot` | LINE の送り先（Messaging API）。テスト用 |
| `LINE_CHANNEL_SECRET` | なし | LINE の Webhook の署名を確かめる。空なら Webhook は必ず 401 |
| `LINE_LOGIN_CHANNEL_ID` | なし | LINE Login（プロフィールからの連携）のチャネル ID。空なら連携を始められない |
| `LINE_LOGIN_CHANNEL_SECRET` | なし | LINE Login のチャネルシークレット |
| `LINE_LOGIN_API_BASE` | `https://api.line.me` | LINE Login の API（トークン・ID トークンの確認・友だち状態）。テスト用 |
| `WEB_ORIGIN` | `https://juken-map.com` | 画面のオリジン。メールのリンク、Google・GitHub・LINE Login の戻り先、終わったあとのリダイレクト先。手元は Vite の URL |
| `SIMULATION_ENABLED` | なし | `on` のときだけシミュレーションの API（`/api/sim/*`）を登録する。それ以外は 404 |
| `SIMULATION_SECRET` | なし | シミュレーションの API の共有トークン（cron とは別）。空なら必ず 401 |
| `CHAOS_ENABLED` | なし | `on` のときだけ障害注入（`internal/fault`）を組み込み、実験を始める API（`/api/chaos/*`）を登録する。それ以外は何も差し込まず 404。消してデプロイすれば全部止まる |
| `CHAOS_SECRET` | なし | 障害注入の API の共有トークン（cron・sim とは別）。空なら必ず 401 |
| `MICROCMS_WEBHOOK_SECRET` | なし | microCMS の Webhook の署名を確かめる（JUK-112）。空なら Webhook は必ず 401 |
| `GITHUB_DEPLOY_TOKEN` | なし | microCMS の Webhook で deploy.yml を動かす GitHub のトークン（fine-grained、このリポジトリの Actions: Read and write だけ）。空なら Webhook は 502 |
| `GITHUB_API_BASE` | `https://api.github.com` | GitHub の API の根元。テスト用 |
| `MIGRATION_DATABASE_URL` | なし | `migrate` コマンド（`internal/migrate/`）が繋ぐ、テーブル定義を変えられるユーザー。無ければ `DATABASE_URL` |
| `MIGRATIONS_DIR` | なし（イメージでは `/migrations`、手元は `pnpm db:migrate` が `db/migrations` を渡す） | `migrate` が当てる SQL の置き場 |
| `WEB_DIST_DIR` | なし（イメージでは `/web`） | 画面のビルド成果物（apps/web の dist）。`index.html` が無ければ画面を配らない（開発は Vite が配る） |
| `GA_MEASUREMENT_ID` | なし | GA4 の測定 ID。画面の HTML に計測のタグを差し込み、CSP でそのインラインスクリプトだけをハッシュで許す |
| `FARO_COLLECTOR_URL` | なし | 画面のエラーの送り先（Grafana Faro）。HTML の meta で画面へ渡し、CSP の connect-src にオリジンを足す |
| `MICROCMS_SERVICE_DOMAIN`・`MICROCMS_API_KEY` | なし | ブログの中継（`/api/blog`）。どちらか空なら 502 |

## 本番

```
nginx ─┬─ /api/dashboard・/api/health/go             ─▶ juken-map-go（127.0.0.1:8080 か 8081）
       ├─ /api/auth/*・/api/line/*・/api/admin/*・/api/sim/*（全メソッド） ─▶ juken-map-go
       ├─ /api/study-logs・/daily・/{id}・
       │  /api/study-plans・/{id}・/{id}/complete・
       │  /api/goals・/{id}・/first-choice・
       │  /api/textbooks・/{id}・/api/textbook-masters・
       │  /api/universities・/{id}・
       │  /api/notification-preferences・/api/profile（全メソッド） ─▶ juken-map-go
       ├─ POST /api/analytics/registration・POST /api/csp-report・
       │  POST /api/cron/daily-study-notifications・
       │  POST /api/webhooks/microcms                  ─▶ juken-map-go
       └─ それ以外（location ~ ^/：画面・/api/blog・/api/health・/line/settings） ─▶ juken-map-go
```

- イメージはこのディレクトリの `Dockerfile` で作り、ECR の `juken-map-go` に置く（`deploy.yml`）。
  画面のビルド成果物は、同じコミットのタグの画面のイメージ（ECR の `juken-map`。ルートの `Dockerfile`、実行はしない）から写す。
  マイグレーションも、このイメージの1回きりのコンテナ（`/api migrate`、JUK-125）で当てる（`.github/scripts/deploy-ec2.sh`）。
  スモークテストが通ったときだけ、nginx を1回の reload で切り替える
- nginx の振り分けは `infra/nginx/juken-map-go-routes.conf`。最後の `location ~ ^/` が残りを全部 Go へ送り、
  その前の location はパスごとの設定（JSON の gzip・ヘッダー）のためにある
- 応答の圧縮は nginx が行う（上のファイルの `gzip`）。Go 自身は圧縮しない。
  例外は大学の一覧で、全員に同じ 80KB なので、Go が1回だけ gzip にした形を持って返す
- 大学の一覧は Go のメモリに10分持つ。管理画面で大学・学部を編集したら（`internal/feature/admin/universities.go`・`internal/feature/admin/faculties.go`）その場で捨てるので、
  編集はすぐ大学を探す画面に出る。10分の期限は、DB を直接書き換えたとき（seed など）の保険
- RDS へは TLS で繋ぐ（`internal/database`。ホスト名が `.rds.amazonaws.com` のときだけ）。証明書は
  `rds-ca-ap-northeast-1.pem` を実行ファイルに埋め込む
- ログとメトリクスは Node のころと同じ `job="juken-map-api"` で Grafana Cloud に入り、
  `runtime="go"` で分けられる（`observability/alloy/production.alloy`）
- 外形監視は `https://juken-map.com/api/health/go`（Go の `/api/health` を返す）

## ルートを足す

入口の種類ごとに登録の関数が分かれている（`internal/httpx/router.go`）。種類を選ばずに登録する方法は無い。

```go
rt.Public("GET /api/health", healthHandler(db))          // 誰でも
rt.User("GET /api/dashboard", dashboard.serve)           // ログイン必須。デモの書き込みは 403
rt.Admin("GET /api/admin/users", adminUsers.serve)       // 管理者だけ
rt.Job("POST /api/cron/…", secret, cron.handle)           // タイマー（infra/systemd/）などが共有トークンで呼ぶ
```

`user` と `admin` のハンドラは `func(w, r, s *httpx.Session)` で、ログイン中の利用者を引数で受け取る。
path の ID は `pathID(w, r, "id")` で読む（数字でなければ 400）。想定外の失敗は
`internalError(w, r, err)` に渡す（500 を返し、原因はログにだけ残す）。

利用者の持ち物（志望校・参考書・実績・予定）を読む・変える関数は、引数に `userID` を取り、
SQL を `WHERE id = ? AND userId = ?` にする。ハンドラーも先に持ち主を確かめる（他人のものは 404）が、
それを書き忘れても他人の行が変わらないようにするため（JUK-54）。新しく足す関数もこの形にし、
`internal/app/ownership_store_db_test.go` に1件足す。

ルートを足したら、次の3か所に足す。

1. `internal/app/routes.go` の `registerRoutes` と、`internal/app/main_test.go` の一覧（入口の種類）
2. user のルートなら、`internal/app` の `ownership_db_test.go`（他人の ID、A3）と `forbidden_fields_db_test.go`（禁止項目、A4）の表。
   書かなければ CI のこの2本が落ちる（下の「DB に流すテスト」）
3. 本番では、書かなくても `infra/nginx/juken-map-go-routes.conf` の最後の `location ~ ^/` で Go に届く。
   JSON の gzip のようにパスごとに変えたい設定があれば、そのファイルに location を足す

## 書き込みのルート

本文は `httpx.ReadBody`（`internal/httpx/body.go`）で読み、入力は `httpx.ReadObject`（`internal/httpx/validate.go`）で確かめる。

```go
body, ok := readBody(w, r, defaultBodyLimit)   // 415・413 はここで返す。壊れた JSON は「本文なし」になる
if !ok {
	return
}
in := readObject(body.value())   // value() は本文の値（本文なしと null を区別する）
input := ProfileInput{Nickname: in.string("nickname", nicknameRule)}
if in.reject(w) {            // 最初の1件を {error, code, field} の 400 で返す
	return
}
```

- **入力チェックの規則の正は Zod**（`src/shared/validations/`）で、画面のフォームが使う。
  Go は同じ規則を手で書く。契約（`openapi/openapi.yaml`）には形（型・必須・長さ）だけを書き、Go の型はそこから作る。
  項目をまたぐ規則や「今日より未来は不可」はスキーマに書けないため、規則は2か所に持つと決めた（JUK-75）
- Zod の規則を変えたら、Go も直して `*_writes_test.go` にケースを足す。移すあいだは Node と応答を比べるテスト
  （`parity.sh`）でずれを見つけていたが、比べる相手の Node の API を消したので一緒に消した（JUK-84）
- Zod の issue は「スキーマに書いた項目の順、項目の中では書いたチェックの順」に積まれ、API は最初の1件だけを返す（Node のころから）。
  `ReadObject` の読み取りも書いた順に確かめ、最初の1件で止まる。文字列の長さは Zod 4.5 と同じくコードポイントで数え、
  trim は JavaScript の `String#trim` と同じ文字を削る
- Cookie で認証する書き込みは、別のサイトから送られたら 403（`internal/httpx/router.go` の `sameOrigin`、標準の
  `http.CrossOriginProtection`）。Node の自前 API には無く、Go で足した
- DB に書く時刻は `dates.NowMillis()`（`internal/dates`。ミリ秒で切り捨て）。そのまま渡すと MySQL が DATETIME(3) へ丸め、切り捨てで書いてきた既存の行とずれる
- 任意の項目は `optional[T]`（`in.optionalString`・`in.optionalInt`）で読み、「キーが無い」と null を区別する。
  キーが無ければ null を書き、`differs()` で今の値と比べて「変わったか」を見る（Node の `data.x ?? null`・`data.x !== current` と同じ規則）
- 数は `json.Number` のまま受け、`httpx.CheckNumber` で Zod の `number().int().positive().max()` と同じ順・同じ文言で確かめる
  （範囲外の数は ±Infinity、安全な整数の外は too_big・too_small）
- 入れ子のオブジェクトは `readObjectAt(element, "items.0")` で読み、`in.take(item)` で外側の issue にする。
  field は Zod と同じ `items.0.content` の形になる。配列は `in.array`（`.min(1)` も確かめる）

クエリ文字列は `r.URL.Query()` ではなく `httpx.ParseQuery`（`internal/httpx/query.go`）で読む。Go の標準は `;` を含む組や
壊れた `%` を黙って捨てるが、Fastify のころは値として受け取っていたので、画面との約束をそちらに合わせてある。
不正な入力の 400 は、Zod の issue と同じ形（`httpx.ValidationIssue`）で返す。

## DB に流すテスト（A3・A4）

```sh
pnpm test:go-db                      # リポジトリのルートから。先に pnpm db:start
```

セキュリティ基準 06 の A3（他人の持ち物の ID を断る）と A4（禁止項目を混ぜても書き換わらない）を、
本物の MySQL に流して確かめる（JUK-97。Node の `ownership.test.ts`・`forbidden-fields.test.ts` を移したもの）。
LINE 連携の SQL（`internal/feature/line/line_db_test.go` の `TestLineStoreDB`）も同じ仕組みで流す。
`registerRoutes` の user のルートを全件、表と突き合わせるので、ルートを足して表に書かなければ落ちる。

- `dbtest` タグのテスト（`*_db_test.go` のうち `//go:build dbtest` のもの）。ふだんの `go test` では動かない
- DB は `db/` のテストと同じ `juken_map_test`。本番と同じマイグレーションが当たり、本番と同じ DML だけの権限で繋ぐ。
  `pnpm --filter @juken-map/db test-db:prepare`（`db/test-db/`）が用意する（`pnpm test:go-db` は先にこれを呼ぶ）
- セッションは Cookie「test」の値を利用者 ID として読む（本物のログインの Cookie の確かめは `internal/feature/auth/` のテスト）
- CI は MySQL のある check のジョブで回す（go のジョブには DB が無い）
- 守りを外すと落ちることは確かめた（参考書の持ち主の確認を外すと A3 が2本、プロフィールの更新で role を書くと A4 が1本落ちる）

## Node と応答を比べる（消した）

移すあいだは、Node と Go に同じリクエストを送ってステータス・本文・ヘッダーを比べていた（`parity.sh`、JUK-71）。
比べる相手の Node の API をすべて消したので、`parity_test.go`・`parity_writes_test.go`・管理画面の比較
（`admin_*_db_test.go`）・`servers.sh` と一緒に消した（JUK-84）。中身は `git log --diff-filter=D -- apps/api` で探せる。

## 毎日の通知の送り方

Go は5本同時に送るが、メールは Resend の上限（チーム全体で毎秒10リクエスト。登録確認のメールなどと分け合う）を
超えないよう毎秒5通に抑える。そのため、メールだけなら Go の速さは毎秒5通で頭打ちになる
（移す前に Node と送り比べた結果は 100人・1通300ms で Node 31.2秒、Go 20.2秒。比べたスクリプト
`compare-notifications.sh` は、Node の通知の入口を消したとき（JUK-84）に一緒に消した）。

## Node と CPU を比べる（消した）

1リクエストあたりの CPU 時間を Node と比べる `compare-cpu.sh` も、比べる相手が無くなったので JUK-84 で消した。

## ファイルの分け方

Go ではフォルダ1つが1つのパッケージで、ファイルの分け方はコンパイル結果に関係しない。
下の分け方は、読む人が探しやすいようにしているだけ。テスト（`*_test.go`）は、確かめる相手の
ファイル名に `_test` か `_db_test`（本物の MySQL、`pnpm test:go-db`）を付けた名前にしている。

移す前の Node のどのファイルにあたるかの対応表は、改名（JUK-131）の前の README にある
（`git show 4c8c109:apps/api-go/README.md`）。Node のファイルは `git log --diff-filter=D -- apps/api/src` で探す。

### 入口（起動・ルート・断り方）

| ファイル | 中身 |
| --- | --- |
| `cmd/api/main.go` | 起動の入口。`internal/app` の `Main` を呼ぶだけ（JUK-156） |
| `internal/app/main.go` | 設定の読み込み、各機能の組み立て、起動と停止 |
| `internal/app/routes.go` | ルートの登録（`registerRoutes`）。一覧と入口の種類は `main_test.go` の `TestRegisteredRoutes` が確かめる（JUK-161） |
| `internal/app/server.go` | ミドルウェアの順番（`newServerHandler`）と、1リクエストの時間の上限（`requestTimeout`） |
| `internal/app/middleware.go` | reqId、アクセスログ、panic の 500、セキュリティヘッダー、時間の上限 |
| `internal/app/overload.go` | 同時処理数の上限を超えたら 503 |
| `Dockerfile` | 本番のイメージ（distroless の static に実行ファイル1つ） |

### ログイン（`internal/feature/auth/`、JUK-115）

| ファイル | 中身 |
| --- | --- |
| `internal/feature/auth/handlers.go` | 入口の一覧、登録・ログイン・ログアウト・セッションの取得 |
| `internal/feature/auth/session.go`・`internal/feature/auth/token.go` | セッション（期限・取り消し・Cookie、各ルートでの照会）とトークンの作り方。行の作成・消去は `internal/write/account` |
| `internal/feature/auth/password.go` | パスワードのハッシュ（Argon2id）と規則（15文字以上・よくあるものの拒否。一覧は `internal/feature/auth/common_passwords.txt`） |
| `internal/feature/auth/recovery.go` | メールの確認・確認メールの再送・再設定・パスワードの変更 |
| `internal/feature/auth/mfa.go`・`internal/feature/auth/totp.go` | 2段階認証（TOTP・予備コード・秘密の暗号化） |
| `internal/feature/auth/oauth.go` | Google・GitHub ログイン（PKCE・nonce・アカウントの結びつけ） |
| `internal/feature/auth/throttle.go`・`internal/feature/auth/email.go` | 回数制限と、上限つきのメール送信（数え方と上限は `internal/write/authguard`） |
| `internal/feature/auth/delete_account.go` | 本人の退会（確かめ直してから、利用者とぶら下がるデータをすべて消す。JUK-123） |
| `internal/app/expired_cleanup.go` | 期限の切れたセッション・トークン・ログインの途中の値・LINE 連携の途中の値を、起動時と1時間ごとに500行ずつ消す（06 G1、JUK-140）。どの表を消すかは持ち主（`internal/write/account`・`authguard`・`notification` の `expired.go`）が決め、このジョブはそれを順に呼ぶ（JUK-154） |

テストは `internal/feature/auth/auth_test.go`（DB なし）と `internal/feature/auth/auth_db_test.go`・`internal/feature/auth/delete_account_db_test.go`・`internal/app/expired_cleanup_db_test.go`（本物の MySQL）。

### 学習記録・志望校・参考書（利用者の画面の API）

読み取りは `<名前>.go`、書き込みは `<名前>_writes.go` に分けている。

| ファイル | 中身 |
| --- | --- |
| `internal/feature/study/` | 学習記録・予定の一覧とダッシュボード（3本の SQL を同時に流して組み立てる）／学習記録の記録・書き換え・削除、学習予定の作成（まとめて）・書き換え・削除・完了の入口（書き込みは `internal/write/studyrecord`）。一覧の期間（`?from=&to=`）の読み方もここ |
| `internal/feature/goals/` | 志望校の一覧と第一志望／志望校の追加・書き換え（PUT・PATCH）・削除の入口（書き込みは `internal/write/goal`） |
| `internal/feature/textbooks/` | 参考書の一覧と参考書マスター／参考書の追加（マスターからも）・進み具合の書き換えの入口（書き込みは `internal/write/textbook`） |
| `internal/feature/universities/` | 大学の一覧（メモリに持ち、マスター編集で捨てる。ETag と 304、gzip 済みを返す）と大学詳細 |
| `internal/feature/auth/profile.go` | プロフィールの更新 |
| `internal/feature/notifications/notification_preferences.go` | 通知設定の読み取り（保存していなければ全部 false）と保存（保存は `internal/write/notification`） |

### 管理画面（`/api/admin/*`）

| ファイル | 中身 |
| --- | --- |
| `internal/feature/admin/users.go` | 利用者の管理（概要・一覧・停止・停止解除・削除、監査ログ） |
| `internal/feature/admin/masters.go` | マスター編集の共通部分（使われている行は消さない決まり、断ったときの返し方、変更の記録、store の定義） |
| `internal/feature/admin/universities.go` | マスター編集の大学（入口・入力・一覧の SQL。書き込みは `internal/write/university` を呼び、変えたら大学一覧のキャッシュを捨てる） |
| `internal/feature/admin/faculties.go` | マスター編集の学部とタグ（入口・入力・タグの一覧。書き込みは `internal/write/university`） |
| `internal/feature/admin/textbook_masters.go` | マスター編集の参考書（入口・入力・一覧の SQL。書き込みは `internal/write/textbookmaster`） |

### 画面の配信（JUK-111）

| ファイル | 中身 |
| --- | --- |
| `internal/spa/spa.go` | 画面の配信（dist を起動時にメモリへ読み、gzip を作り置く。SSG の HTML、知らないパスの 404、sitemap） |
| `internal/spa/seo.go` | ページごとの meta・GA4・Faro の差し込み、画面の CSP、sitemap の中身、SPA のルートの一覧 |
| `internal/feature/blog/blog.go` | ブログの記事の中継（microCMS、3秒で打ち切り、障害は 502） |

### 外部サービスとの連携

| ファイル | 中身 |
| --- | --- |
| `internal/feature/line/` | LINE 連携の入口（連携の確認・解除、トークからの Account Link、プロフィールからの LINE Login、Webhook）。書き込みは `internal/write/notification` |
| `internal/feature/line/api.go` | LINE の API を呼ぶ部分（Messaging API・LINE Login、Webhook の署名の確かめ）。テストでは偽物に差し替える |
| `internal/feature/blog/microcms_webhook.go` | microCMS の記事の公開・更新・削除を受け、署名を確かめてから GitHub の API でデプロイを動かす（JUK-112） |

### 毎日の通知

送り方の速さは上の「[毎日の通知の送り方](#毎日の通知の送り方)」。

| ファイル | 中身 |
| --- | --- |
| `internal/feature/notifications/notifications.go` | 毎日の通知の送信（同時に5本、メールは毎秒5通まで）。DB と送信先は差し替えられる。送った印は `internal/write/notification` |
| `internal/feature/notifications/daily_notification.go` | 通知の文面と、日本時間の「今日」の範囲 |

### 計測・シミュレーション

| ファイル | 中身 |
| --- | --- |
| `internal/feature/analytics/` | 本登録の完了を GA4 の sign_up として1回だけ数えるための問い合わせ（JUK-80） |
| `internal/feature/cspreport/` | ブラウザが送る CSP の違反の報告をログに残す（認証なしの口なので件数と大きさに上限） |
| `internal/feature/chaos/` | 障害注入の実験の入口（JUK-173）。始める・一覧・まとめて止めるは機械の入口（`/api/chaos/*`、`CHAOS_ENABLED=on` と `CHAOS_SECRET`）、管理画面（`/api/admin/chaos`）は終わった実験の記録と緊急停止だけで、実行中の実験は返さず、止めたかどうかも返さない（JUK-178）。書き込みは `internal/write/chaos` |
| `internal/feature/sim/` | シミュレーション（`sim/`）専用の API。`SIMULATION_ENABLED=on` と `SIMULATION_SECRET` が要り、シミュレーション用のアドレスだけに触る。書き込みは `internal/write/simulation` |

### コマンド（引数を付けて起動したとき）

| ファイル | 中身 |
| --- | --- |
| `internal/app/cli.go` | 引数を付けて起動したときの振り分け：`migrate` は `internal/migrate`、`incident`（乗っ取りのときの操作）・`grant-admin`（管理者の付け外し）は `internal/feature/ops` へ渡す。本番は `docker exec juken-map-go /api ...`、手元は `pnpm incident`・`pnpm admin:grant`（JUK-122） |
| `internal/feature/ops/` | `incident`・`grant-admin` の引数の読み取り（`commands.go`）と、使う読み取り（`incident.go`）と、`internal/write/account` の操作（セッションの取り消し・停止・2段階認証の解除・役割の付け外し。変えたことは `OpsAuditLog` に残り、`incident log` で見る。JUK-138）の呼び出し。手順は `docs/incident-response.md` |
| `internal/migrate/` | `migrate`（まだ当てていないマイグレーションを名前順に流す。JUK-125）。本番はデプロイが起動前に流し、手元は `pnpm db:migrate` |
| `cmd/devtool/`・`internal/devtool/` | 開発でしか使わない道具（JUK-143）。`hash-password`・`email-token`・`sessions` で、seed・E2E・負荷試験の利用者のパスワードのハッシュ、メールのリンクのトークン、ログイン済みのセッションを、`internal/feature/auth` の `devtool.go` の関数（ログインと同じ作り方）で作る。手元は `scripts/go-devtool.sh`。Dockerfile は `cmd/api` だけを作るので、本番のイメージには入らない |

### 共通の部品

| ファイル | 中身 |
| --- | --- |
| `internal/app/http.go` | ヘルスチェック |
| `internal/dates/` | 東京の「今日」、月初・月末、日付のずらし、`YYYY-MM-DD` の読み方（Node の `new Date` と同じ繰り越し）、DB に書く時刻（ミリ秒で切り捨て）と ISO 文字列。機能をまたいで使う（JUK-156） |
| `internal/site/` | 本番の画面のオリジン（`site.URL`）。通知の本文・SEO・LINE の連携先が使う（JUK-156） |
| `internal/httpx/` | HTTP の入口の共通部品（JUK-155）。入口の種類ごとの拒否（`router.go`。未ログイン・停止中・管理者・デモ・別のサイトからの書き込み）、利用者単位の回数制限（`user_rate_limit.go`。読み取り・書き込みの2種類、メモリのトークンバケット。超えたら 429 と `Retry-After`、06 E2）、エラー応答の形・404・path の ID（`errors.go`）、JSON と 400 の書き出し（`response.go`）、リクエスト本文の読み方（`body.go`。Content-Type・上限・壊れた JSON・不正な UTF-8、415・413）、書き込みの入力チェック（`validate.go`。Zod の最初の issue と同じ 400）、接続元の IP（`client_ip.go`）、クエリ文字列の読み方（`query.go`。Fastify のころと同じ規則）、全員に同じ応答を JSON・gzip・ETag でメモリに持つキャッシュ（`json_snapshot.go`。大学一覧・参考書マスター、JUK-50）、外部サービスの呼び出しの上限（`external.go`）、受け付けるなら gzip で返す（`gzip.go`。画面・sitemap・ブログの中継）。セッションの読み方は関数で受け取り、`internal/write` には依存しない |
| `internal/telemetry/` | 計測の土台（JUK-155）。pino と同じ形の JSON ログ（`logger.go`。reqId・trace_id を足し、`LOG_FILE` にも書く）、Prometheus のメトリクス（`metrics.go`。名前・ラベルは Node のころと同じ）、OpenTelemetry のトレース（`tracing.go`。リクエスト・SQL・外部 API の呼び出し。URL のパスとクエリは入れない、JUK-126）、監視に出す文字列のメールアドレスを伏せる（`redact.go`）、リクエストごとの情報（`request_info.go`。reqId・シミュレーションの印・ルート） |
| `internal/fault/` | 障害注入（JUK-173）。実行中の実験を5秒ごとに `ChaosExperiment` から読み、対象のルートのリクエストにだけ遅延・5xx（ルーター）、DB の失敗（`database.Open` に渡す接続の包み）、外部 API のタイムアウト（外部 API のクライアントの土台）を起こす。管理画面・ログイン・障害注入の入口とリクエストの外の処理には起こさない。調べる練習で答えにならないよう、起こしたことはログ・スパン・数値・画面のどこにも出さず、誤りの文言も本物と同じにする（JUK-178）。何をいつ起こしたかは `ChaosExperiment` の行が答え合わせになる |
| `internal/database/` | 接続プール、RDS への TLS（`rds-ca-ap-northeast-1.pem`）、トランザクション（`InTx`）、MySQL のエラー番号、DATETIME の文字列を ISO にする。書き込みの持ち主と読み取りの両方が使う（JUK-152） |
| `internal/opt/` | 持ち主の操作に渡す「送られなかった」と null を区別する値（`opt.Field`）。入口の `httpx.Optional` を `.Field()` で変換する。持ち主ではないので `internal/write` の外に置く（JUK-160） |
| `internal/write/account/` | アカウントへの書き込みの持ち主（`user` の行・ログインの状態・運用の記録）。利用停止・解除（`suspend.go`）、セッション（`session.go`）、登録とメールの確認（`registration.go`）、パスワード・メールのトークン・2段階認証の途中の状態（`credential.go`）、TOTP と予備コード（`totp.go`）、外部ログインの連携・削除・ニックネーム・計測の印（`user.go`）、権限（`role.go`）。運用のコマンドから呼ぶ操作は、記録（`OpsAuditLog`）を同じトランザクションで書く（JUK-151・JUK-154、構成は `docs/architecture.md`「バックエンドの構成」） |
| `internal/write/studyrecord/` | 学習記録（実績・予定・初回記録の日時）への書き込みの持ち主。実績の記録・変更・削除と、予定の作成・変更・削除・完了。参考書の持ち主と範囲の確かめ、予定の完了と実績の作成を1つのトランザクションで行う（JUK-153） |
| `internal/write/textbook/` | 利用者の参考書への書き込みの持ち主。名前・参考書マスターからの登録と、逆算設定の変更（JUK-154） |
| `internal/write/goal/` | 志望校への書き込みの持ち主。登録・学部の差し替え・第一志望やメモの変更・削除。第一志望の付け替えを1つのトランザクションで行う（JUK-154） |
| `internal/write/university/` | 大学・学部のマスター（学部のタグを含む）への書き込みの持ち主。管理画面の作成・書き換え・削除。志望校に使われている行は消さない（JUK-154） |
| `internal/write/textbookmaster/` | 参考書マスター（総量の候補を含む）への書き込みの持ち主。管理画面の作成・書き換え・削除。利用者の参考書に使われているものは消さない（JUK-154） |
| `internal/write/notification/` | 通知（LINE の連携・通知の設定・送った印）への書き込みの持ち主。連携・解除、通知の設定の保存。LINE と連携していなければ LINE 通知を ON にしない（JUK-154） |
| `internal/write/simulation/` | 負荷のシミュレーションの利用者の印（`user.simSeq` など）への書き込みの持ち主。連番と続き方の型を付け、最後に操作した日・来なくなった日を記録する。シミュレーション用のアドレスの利用者にしか触れない（JUK-154） |
| `internal/write/chaos/` | 障害注入の実験（`ChaosExperiment`）への書き込みの持ち主。始める・まとめて止める。実行中の実験は表全体で1つまでを、1つの SQL で守る（JUK-173） |
| `internal/write/authguard/` | ログインの守り（回数の制限・メールの送信の上限・外部ログインの state）への書き込みの持ち主。試行を先に数えてから判定し、メールは数えてから記録するまでを名前付きロックで1件ずつ通す（JUK-154） |
| `internal/write/expired/` | 期限の切れた行の消し方（主キーで選んで主キーで消す）。消す表は持ち主（account・authguard・notification）が渡す。書き込みの SQL は `internal/write/` の下だけに置く決まり（JUK-157）のため、`internal/database` から移した |
| `internal/write/sql_boundary_test.go` | 書き込みの SQL（INSERT・UPDATE・DELETE・REPLACE）が `internal/write/` の下（と `internal/migrate`・`internal/dbtest`）にしか無いことを確かめるテスト（JUK-157） |
| `internal/write/import_boundary_test.go`・`internal/feature/import_boundary_test.go` | write が HTTP と画面の形（`net/http`・`httpx`・`apischema`・feature など）を import しないこと、feature 同士が import し合わないことを確かめるテスト（JUK-159） |
| `internal/apischema/` | `openapi/openapi.yaml` から作った応答・リクエストの型（`openapi.gen.go`。手で直さない。`pnpm openapi:generate`、設定は同じディレクトリの `oapi-codegen.yaml`）。入口と持ち主の両方が使う（JUK-155） |

### 本物の DB に流す横断のテスト（dbtest タグ）

DB テストは名前に `DB` を入れる。`pnpm test:go-db`（CI も同じ）は `-run DB` で、どのパッケージのものも拾う。DB 全体を数えるテストがあるので、パッケージは `-p 1` で1つずつ流す。

| ファイル | 中身 |
| --- | --- |
| `internal/app` の `ownership_db_test.go`・`ownership_store_db_test.go`・`forbidden_fields_db_test.go`・`dbtest_support_test.go` | 他人の ID（A3。入口からと、ストアを直接呼んで）と禁止項目（A4）を確かめる |
| `internal/dbtest/` | DB テストの補助。テスト用 DB への接続（名前が `_test` で終わる DB にだけ繋ぐ）と、テスト用の利用者・データの作り方。どのパッケージの DB テストからも使う（JUK-158）。ルーターを組んで叩く `dbTestApp` は `registerRoutes` を使うので `internal/app/dbtest_support_test.go` に置く |
| `internal/httpx/httpxtest/` | 入口と入力チェックを使うテストの補助。Cookie で選ぶ偽のセッション（`FakeSessions`・`Sessions`）、JSON の比べ方、入力チェックの 400 の本文。feature のテストが共通で使う（JUK-156）。本番のコードからは import しない |
| `internal/app/list_limits_db_test.go` | 学習記録・予定の一覧が 1000 件で切り詰められることを確かめる（06 E2） |

## Node のころから変えていないこと

画面・監視・運用の手順がこの形に頼っているので、変えるときはそちらも直す。


- 応答の形。エラーは `{"error"}`（ルートが断ったとき）と `{"error","code","reqId"}`（想定外・混雑・404）の2つ
- 拒否の判定と文言。401 Unauthorized、停止中・管理者でない・デモの書き込みは 403
- ログの形（pino：`level` は数字、`time` は UNIX ミリ秒、`req.url` はパスだけ）。Alloy と Grafana がこの形を読む
- メトリクスの名前・ラベル・区切り。`route` は `/api/study-logs/:id` の形
- `X-Request-Id` ヘッダー、セキュリティヘッダー（CSP だけ API 向けに `default-src 'none'`）
- 同時処理数の上限 160、断るときは 503 と `Retry-After: 1`
- SQL（同じ列・同じ条件・同じ並び）、接続プールの上限 15、`?` への埋め込みはドライバ側（1クエリ1往復）

## Node のころから変えたこと

- 応答の圧縮（Node は br・gzip を自分でしていた。Go は持たず、本番では nginx が gzip にする）
- OpenTelemetry のトレース（`internal/telemetry/tracing.go`）
- 1リクエスト10秒の上限（`internal/app/server.go` の `requestTimeout`。DB の照会と接続待ちもここで止まる）
