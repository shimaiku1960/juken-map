# api-go

Node（`apps/api`）の業務 API を1本ずつ Go へ移すためのサーバー（JUK-70）。
今は `GET /api/dashboard`（JUK-69）と、学習記録・予定の一覧（`GET /api/study-logs`・`/api/study-logs/daily`・
`/api/study-plans`）、志望校（`GET /api/goals`・`/api/goals/first-choice`）、参考書（`GET /api/textbooks`・`/api/textbook-masters`）、
通知設定（`GET /api/notification-preferences`）、大学（`GET /api/universities`・`/api/universities/{id}`）を
持つ（JUK-73）。書き込みは、通知設定（`PUT /api/notification-preferences`）・プロフィール（`PUT /api/profile`）・
学習記録（`POST /api/study-logs`・`PATCH`/`DELETE /api/study-logs/{id}`）を移している（JUK-75）。本番では nginx がこれらのパスだけを Go へ振り分け、
それ以外は今までどおり Node が返す（JUK-72、下の「本番」）。

ログインの発行・管理画面・外部連携は Node に残す。Go は Node（Better Auth）が発行した
セッション Cookie を、同じ DB と同じ `BETTER_AUTH_SECRET` で確かめるだけ。

## 動かし方

```sh
cd apps/api-go
set -a; source ../../.env; set +a   # DATABASE_URL と BETTER_AUTH_SECRET を読む
go run .                             # PORT を指定しなければ 8080
go test ./...
```

ログは本番と同じ1行1つの JSON。人が読みたいときは、Node と同じ pino-pretty に通す。

```sh
go run . | pnpm exec pino-pretty
```

ログインは Node 側で行い、そのセッション Cookie をそのまま使う。

```sh
curl -H "Cookie: better-auth.session_token=..." localhost:8080/api/dashboard
```

| 環境変数 | 既定 | 意味 |
| --- | --- | --- |
| `PORT` | 8080 | 待ち受けるポート |
| `METRICS_PORT` | なし | 指定したときだけ、このポートで `/metrics` を出す |
| `OVERLOAD_MAX_IN_FLIGHT` | 160 | 同時に処理する件数の上限。超えたら 503 |
| `LOG_LEVEL` | info | pino と同じ名前（debug・info・warn・error） |
| `NODE_ENV` | なし | `production` のとき reqId を UUID のまま出す（開発は8文字） |
| `DATABASE_URL` | なし | 必須。Node と同じ形（`mysql://…`） |
| `BETTER_AUTH_SECRET` | なし | 必須。Node と同じ値（Cookie の署名を確かめる） |
| `DAILY_NOTIFICATION_SECRET` | なし | 毎日の通知の入口の共有トークン。空なら、その入口は必ず 401 |
| `RESEND_API_KEY` | なし | 毎日の通知のメールを送る Resend のキー |
| `RESEND_BASE_URL` | `https://api.resend.com` | Resend の送り先。手元の比較で偽のサーバーへ向けるときだけ変える（Node の SDK と同じ名前） |
| `LINE_CHANNEL_ACCESS_TOKEN` | なし | 毎日の通知を LINE で送るトークン |
| `LINE_API_BASE` | `https://api.line.me/v2/bot` | LINE の送り先。テスト用 |
| `SIMULATION_ENABLED` | なし | `on` のときだけシミュレーションの API（`/api/sim/*`）を登録する。それ以外は 404 |
| `SIMULATION_SECRET` | なし | シミュレーションの API の共有トークン（cron とは別）。空なら必ず 401 |

## 本番

```
nginx ─┬─ /api/dashboard・/api/health/go             ─▶ juken-map-go（127.0.0.1:8080 か 8081）
       ├─ /api/study-plans・/api/goals・/first-choice・
       │  /api/textbooks・/api/textbook-masters・
       │  /api/universities・/api/universities/{id}
       │    GET・HEAD                                 ─▶ juken-map-go
       │    それ以外（POST など）                      ─▶ juken-map（Node）
       ├─ /api/study-logs・/daily・/api/study-logs/{id}・
       │  /api/notification-preferences・/api/profile（全メソッド） ─▶ juken-map-go
       ├─ POST /api/analytics/registration・POST /api/csp-report・
       │  POST /api/cron/daily-study-notifications     ─▶ juken-map-go
       ├─ GET /api/sim/state・POST /api/sim/users・
       │  PATCH /api/sim/users/{seq}                   ─▶ juken-map-go（ほかのメソッドは Node）
       └─ それ以外                                     ─▶ juken-map（3000 か 3001、Node）
```

- イメージはこのディレクトリの `Dockerfile` で作り、ECR の `juken-map-go` に置く（`deploy.yml`）。
  Node と同じコミットのタグで、同じデプロイ（`.github/scripts/deploy-ec2.sh`）の中で入れ替える。
  Node と Go の両方のスモークテストが通ったときだけ、nginx を1回の reload で両方とも切り替える
- 振り分けるパスは `infra/nginx/juken-map-go-routes.conf`。ルートを移したらここに足す。
  Node に戻す手順は `infra/nginx/README.md` の「Go の振り分け」
- 応答の圧縮は nginx が行う（上のファイルの `gzip`）。Go 自身は圧縮しない。
  例外は大学の一覧で、全員に同じ 80KB なので、Go が1回だけ gzip にした形を持って返す
- 大学の一覧は Go のメモリに1分持つ。管理画面（Node）で大学・学部・タグを編集しても Go のキャッシュは
  捨てられないので、大学を探す画面に出るまで最大1分かかる。管理 API を Go へ移したら（JUK-78）、編集のときに捨てる
- RDS へは TLS で繋ぐ（`db.go`。ホスト名が `.rds.amazonaws.com` のときだけ）。証明書は
  `rds-ca-ap-northeast-1.pem` を実行ファイルに埋め込む
- ログとメトリクスは Node と同じ `job="juken-map-api"` で Grafana Cloud に入り、
  `runtime="go"` で分けられる（`observability/alloy/production.alloy`）
- 外形監視は `https://juken-map.com/api/health/go`（Go の `/api/health` を返す）

## ルートを足す

入口の種類ごとに登録の関数が分かれている（`router.go`）。種類を選ばずに登録する方法は無い。

```go
rt.public("GET /api/health", healthHandler(db))          // 誰でも
rt.user("GET /api/dashboard", dashboard.serve)           // ログイン必須。デモの書き込みは 403
rt.admin("GET /api/admin/users", adminUsers.serve)       // 管理者だけ
rt.job("POST /api/cron/…", secret, cron.handle)           // タイマー（infra/systemd/）などが共有トークンで呼ぶ
```

`user` と `admin` のハンドラは `func(w, r, s *session)` で、ログイン中の利用者を引数で受け取る。
path の ID は `pathID(w, r, "id")` で読む（数字でなければ 400）。想定外の失敗は
`internalError(w, r, err)` に渡す（500 を返し、原因はログにだけ残す）。

移したら、次の3か所に足す。

1. `main.go` の `registerRoutes` と、`main_test.go` の一覧（入口の種類を Node と揃える）
2. `parity_test.go` の `parityCases`（Node と応答が同じかを確かめる。不正な入力のケースも）
3. 本番に出すときは `infra/nginx/juken-map-go-routes.conf`。同じパスに書き込みが残っているなら、
   `/api/study-plans` と同じく GET・HEAD 以外を `@node` へ回す

## 書き込みのルート

本文は `readBody`（`body.go`）で読み、入力は `readObject`（`validate.go`）で確かめる。

```go
body, ok := readBody(w, r, defaultBodyLimit)   // 415・413 はここで返す。壊れた JSON は「本文なし」になる（Node と同じ）
if !ok {
	return
}
in := readObject(body.value())   // value() は Node の request.body と同じ値（本文なしと null を区別する）
input := ProfileInput{Nickname: in.string("nickname", nicknameRule)}
if in.reject(w) {            // 最初の1件を {error, code, field} の 400 で返す
	return
}
```

- **入力チェックの規則の正は Zod**（`src/shared/validations/`）で、画面のフォームと Node が使う。
  Go は同じ規則を手で書く。契約（`openapi/openapi.yaml`）には形（型・必須・長さ）だけを書き、Go の型はそこから作る。
  項目をまたぐ規則や「今日より未来は不可」はスキーマに書けないため、規則は2か所に持つと決めた（JUK-75）
- ずれは応答一致テスト（`parity_writes_test.go`）に不正な入力を並べて見つける。Zod の規則を変えたら、
  Go も直してケースを足す
- Zod の issue は「スキーマに書いた項目の順、項目の中では書いたチェックの順」に積まれ、Node は最初の1件だけを返す。
  `readObject` の読み取りも書いた順に確かめ、最初の1件で止まる。文字列の長さは Zod 4.5 と同じくコードポイントで数え、
  trim は JavaScript の `String#trim` と同じ文字を削る
- Cookie で認証する書き込みは、別のサイトから送られたら 403（`router.go` の `sameOrigin`、標準の
  `http.CrossOriginProtection`）。Node の自前 API には無く、Go だけが持つ
- DB に書く時刻は `nowMillis()`（ミリ秒で切り捨て）。そのまま渡すと MySQL が DATETIME(3) へ丸め、Node とずれる
- 任意の項目は `optional[T]`（`in.optionalString`・`in.optionalInt`）で読み、「キーが無い」と null を区別する。
  Node は `data.x ?? null` で書き、`data.x !== current` で「変わったか」を見るので、`ptr()` と `differs()` で同じにする
- 数は `json.Number` のまま受け、`checkNumber` で Zod の `number().int().positive().max()` と同じ順・同じ文言で確かめる
  （範囲外の数は ±Infinity、安全な整数の外は too_big・too_small）
- 成功する書き込みの応答一致は、同じ手順を Node と Go で順に流して各段を比べる（`TestParityStudyLogScenario`）。
  手順は自分で作った行を自分で消し、書き換えた行は最後に戻す

クエリ文字列は `r.URL.Query()` ではなく `parseQuery`（`query.go`）で読む。Go の標準は `;` を含む組や
壊れた `%` を黙って捨てるが、Node（Fastify）は値として受け取るので、そのままでは応答がずれる。
不正な入力の 400 は、Node の Zod と同じ形（`validationIssue`）で返す。

## Node と応答を比べる

```sh
bash apps/api-go/parity.sh                 # worktree のルートから。合成ユーザー50人ぶん
```

Node と Go を立ち上げ、同じリクエストを送って、ステータス・本文・ヘッダーを比べる。
食い違うと、どこが違うかを `$.logs[3].textbook.name: … → …` の形で出す。
両方のサーバーと合成データ（`pnpm db:seed:synthetic`）が要るので CI では動かさない。

## 毎日の通知を Node と比べる

```sh
bash apps/api-go/compare-notifications.sh    # worktree のルートから。合成ユーザー100人
N=30 LATENCY_MS=400 bash apps/api-go/compare-notifications.sh
```

偽の Resend（1通ごとに LATENCY_MS 待って 200 を返す）を立て、Node と Go に朝の通知を1回ずつ送らせる。
宛先・件名・本文が同じかと、送り終わるまでの秒数を比べる。本物のメールは送らない。
⚠️ 流している間、手元の DB の通知設定を書き換える（終わったら戻す）。DB は全 worktree で共有なので注意。

Go は5本同時に送るが、メールは Resend の上限（チーム全体で毎秒10リクエスト。登録確認のメールなどと分け合う）を
超えないよう毎秒5通に抑える。そのため、メールだけなら Go の速さは毎秒5通で頭打ちになる
（100人・1通300ms で Node 31.2秒、Go 20.2秒）。

## Node と CPU を比べる

```sh
bash apps/api-go/compare-cpu.sh            # worktree のルートから。N=500 件 × 交互3回
API_PATH='/api/study-plans?from=2026-09-01&to=2026-10-31' bash apps/api-go/compare-cpu.sh
```

負荷試験（k6）は使わない。手元で 300 RPS をかけると Mac 全体が詰まり、ほかの作業が
できなくなるうえ、ほかのアプリの影響で結果も揺れる。このスクリプトは1件ずつ順番に送り、
サーバーの CPU 時間の増分を件数で割るので、ほかの作業をしながら測れる。
分かるのは「1件の重さ」で、同時に何件さばけるか（限界 RPS）は分からない。

## ファイルの分け方

| ファイル | 役割 | Node 側で近いもの |
| --- | --- | --- |
| `main.go` | 設定の読み込み、ルートの登録、ミドルウェアの順番、起動と停止 | `server.ts` |
| `router.go` | 入口の種類ごとの拒否（未ログイン・停止中・管理者・デモ） | `access-control.ts`・`context.ts` |
| `auth.go` | セッション Cookie の署名確認と、session テーブルの照会 | Better Auth の `getSession` |
| `middleware.go` | reqId、アクセスログ、panic の 500、セキュリティヘッダー、時間の上限 | `observability/logger.ts`・`requestContext.ts`・`security-headers.ts` |
| `overload.go` | 同時処理数の上限を超えたら 503 | `overload.ts` |
| `errors.go` | エラー応答の形、404、path の ID | `error-handling.ts`・`routes/params.ts` |
| `logger.go` | pino と同じ形の JSON ログ | `observability/logger.ts` |
| `metrics.go` | Prometheus のメトリクス（名前・ラベルは Node と同じ） | `observability/metrics.ts` |
| `http.go` | JSON の書き出し、ヘルスチェック | Fastify 本体 |
| `db.go` | 接続プール、RDS への TLS、DATETIME の文字列を ISO にする | `infra/db.ts` |
| `Dockerfile` | 本番のイメージ（distroless の static に実行ファイル1つ） | ルートの `Dockerfile` |
| `dates.go` | 東京の「今日」、月初・月末、日付のずらし | `src/shared/date.ts` |
| `dashboard.go` | ダッシュボードの応答の型、3本の SQL を同時に流して組み立てる | `services/dashboard-service.ts` |
| `study.go` | 学習記録・予定の応答の型と SQL（ダッシュボードと一覧で共有） | `study-log-service.ts`・`study-plan-service.ts` の list 系 |
| `study_handlers.go` | 学習記録・予定の一覧の API | `routes/study-logs.ts`・`study-plans.ts` の GET |
| `goals.go` | 志望校の一覧と第一志望（応答の型と SQL） | `services/goal-service.ts`・`routes/goals.ts`・`home.ts` の GET |
| `textbooks.go` | 参考書の一覧と参考書マスター（応答の型と SQL） | `services/textbook-service.ts`・`routes/textbooks.ts`・`textbook-masters.ts` の GET |
| `notification_preferences.go` | 通知設定の読み取り（保存していなければ全部 false）と保存 | `routes/notification-preferences.ts` |
| `universities.go` | 大学の一覧（メモリに1分持ち、ETag と 304、gzip 済みを返す）と大学詳細 | `services/university-service.ts`・`routes/universities.ts` |
| `notifications.go` | 毎日の通知の送信（同時に5本、メールは毎秒5通まで）。DB と送信先は差し替えられる | `routes/cron.ts`・`services/sendDailyNotifications.ts` |
| `daily_notification.go` | 通知の文面と、日本時間の「今日」の範囲 | `domain/dailyNotification.ts` |
| `query.go` | クエリ文字列の読み方、期間（`?from=&to=`）、400 の形 | fast-querystring・Zod |
| `body.go` | リクエスト本文の読み方（Content-Type・上限・壊れた JSON・不正な UTF-8）、415・413 の形 | Fastify の本文の解析・`server.ts` の JSON パーサー |
| `validate.go` | 書き込みの入力チェック（Zod の最初の issue と同じ 400） | `src/shared/validations/`・`routes/validation-error.ts` |
| `study_log_writes.go` | 学習記録の記録・書き換え・削除（参考書の範囲の確かめ、初回記録の印） | `routes/study-logs.ts` の POST・`study-log-item.ts`・`study-log-service.ts`・`domain/textbookRange.ts` |
| `profile.go` | プロフィールの更新 | `routes/profile.ts`・`services/user-service.ts` の updateProfile |
| `parity_test.go`・`parity_writes_test.go`・`parity.sh` | Node と応答を比べる（ログイン済みの書き込みは `parity_writes_test.go`。書き換えた行を最後に戻す） | — |
| `compare-notifications.sh` | 毎日の通知を Node と Go で送り比べる（偽の Resend へ） | — |
| `servers.sh` | Node と Go を並べて起動する（parity.sh・compare-cpu.sh が使う） | — |

Go ではフォルダ1つが1つのパッケージで、ファイルの分け方はコンパイル結果に関係しない。
上の分け方は、読む人が探しやすいようにしているだけ。

## Node と揃えていること

- 応答の形。エラーは `{"error"}`（ルートが断ったとき）と `{"error","code","reqId"}`（想定外・混雑・404）の2つ
- 拒否の判定と文言。401 Unauthorized、停止中・管理者でない・デモの書き込みは 403
- ログの形（pino：`level` は数字、`time` は UNIX ミリ秒、`req.url` はパスだけ）。Alloy と Grafana がこの形を読む
- メトリクスの名前・ラベル・区切り。`route` は `/api/study-logs/:id` の形
- `X-Request-Id` ヘッダー、セキュリティヘッダー（CSP だけ API 向けに `default-src 'none'`）
- 同時処理数の上限 160、断るときは 503 と `Retry-After: 1`
- SQL（同じ列・同じ条件・同じ並び）、接続プールの上限 15、`?` への埋め込みはドライバ側（1クエリ1往復）

## まだ揃えていないこと

- 応答の圧縮（Node は br・gzip。Go は持たず、本番では nginx が gzip にする。比べるときは `Accept-Encoding: identity`）
- セッションの有効期限の延長（Better Auth は古くなったセッションを更新するが、Go は読むだけ）。
  画面はほかの API（Node）も呼ぶので、そちらで延長される
- OpenTelemetry のトレース
- Node に無いもの：1リクエスト10秒の上限（DB の照会と接続待ちもここで止まる）
