# 開発ガイド

手元で受験マップを動かし、変更を確かめるための手順です。アプリの概要は[README](../README.md)、本番の運用は[運用ガイド](operations.md)にあります。

## 前提条件

- Node.js 24
- pnpm 12.3.4（`package.json`で固定）
- Go（`apps/api/go.mod`の版。業務APIはGoが返すため、開発とE2Eでも起動する）
- Docker Desktopなど、Docker Composeを実行できる環境（MySQLと、Goへ振り分けるnginxを動かす）

OAuthログイン、メール送信、ブログまで確認する場合は、Google・GitHub OAuth、Resend、microCMSの資格情報も必要です。

## 初回起動

1. リポジトリをクローンします。

   ```bash
   git clone https://github.com/shimaiku1960/juken-map.git
   cd juken-map
   ```

2. 依存関係をインストールします。

   ```bash
   npm install --global pnpm@12.3.4
   pnpm install --frozen-lockfile
   pnpm run hooks:install
   ```

   `hooks:install`は、依存ファイルをコミットするときだけ、ステージ済みの内容でlockfileを確認するGitフックを有効にします。

3. 環境変数ファイルを作成します。

   ```bash
   cp .env.example .env
   ```

4. `.env` の各値を開発環境に合わせて変更します。

   [注意] `.env` には秘密情報が含まれます。Gitへコミットしないでください。

5. MySQLを起動します。

   ```bash
   pnpm run db:start
   ```

6. マイグレーション（テーブル定義）を適用します。

   ```bash
   pnpm run db:migrate
   ```

7. 大学マスターとデモデータを投入します。

   ```bash
   pnpm run db:seed
   ```

8. 開発サーバーを起動します。MySQLの起動とマイグレーション確認後、Go・nginx・画面が並列で起動します。

   ```bash
   pnpm dev
   ```

   APIと画面のログは、実行したターミナルに実行元の名前付きで表示されます。
   APIのログは1行1件のJSONで出ます（`apps/api/internal/telemetry/logger.go`）。
   Viteが`/api`を同一オリジンのままnginx（4200番、Docker）へ送り、nginxが本番と同じ振り分けファイル
   （`infra/nginx/juken-map-go-routes.conf`）でGo（4100番）へ送ります。本番と同じ形で
   動かすためです。ブラウザで開くのは5173番です。Goは自動で再起動しないので、Goを書き換えたら
   `pnpm dev`を起動し直します。

9. [http://localhost:5173](http://localhost:5173)を開きます。

## 2回目以降の起動

```bash
pnpm dev
```

`Ctrl+C`でGo・nginx・画面をまとめて停止できます。MySQLコンテナはバックグラウンドで継続するため、停止する場合は`pnpm run db:stop`を実行します。

[注意] `.env`を変更したら、`pnpm dev`を起動し直してください。Goは起動時に一度だけ`.env`を読みます
（`scripts/dev-go.sh`）。

## 作業ごとに worktree を分ける

複数のターミナルやエージェントが同じチェックアウトで作業すると、ブランチやコミットが
混ざります。作業ごとに git worktree を作り、その中で開発します。

```bash
pnpm wt:new fix/JUK-40-foo      # ../juken-map-worktrees/fix-JUK-40-foo に作る
cd ../juken-map-worktrees/fix-JUK-40-foo
pnpm dev
pnpm wt:remove fix/JUK-40-foo   # マージ後に片付ける
```

`wt:new`は、`.env`などgitignore済みのファイルを本体へのリンクにし、空いているポート
（N番目なら画面5173+N、Go 4100+N、nginx 4200+N、E2E 3010+N。決め方は
`scripts/local-ports.sh`）を`.env.worktree`に書き、`pnpm install`まで
行います。MySQLは`docker-compose.yml`のプロジェクト名を固定しているので、どのworktreeからも
同じコンテナを使います。Google・GitHubログインはコールバックURLのポートが合わないため、
worktreeではメールとパスワードでログインします。

## ローカルDBの中身を見る

表形式で眺めるときは、TablePlusなどのGUIクライアントで`127.0.0.1:3306`（DB名`juken_map`）へ接続します。ユーザー名とパスワードは[docker-compose.yml](../docker-compose.yml)の`MYSQL_USER`／`MYSQL_PASSWORD`です。

SQLを直接打つときは、MySQLの対話画面を開きます。接続情報は`docker-compose.yml`から読むため、入力は不要です。

```bash
pnpm run db:shell                        # 対話画面を開く（exitで終了）
pnpm run db:shell -e "SHOW TABLES"       # SQLを1本だけ実行する
```

## 多人数が使っている状態を再現する

可観測性のグラフや負荷試験は、1人分の練習データの上では意味のある数字になりません。
利用者が大勢いる状態を手元のDBで再現する seed を用意しています。

```bash
pnpm run db:seed:synthetic                       # 300人 × 6か月分
USERS=2000 MONTHS=12 pnpm run db:seed:synthetic  # 2000人 × 12か月分（学習実績17万件、約9秒）
pnpm run db:seed:synthetic:clear                 # 合成データだけ消す（実データは残る）
```

行数を増やすだけでなく、**使い方の偏り**を作ります。続ける人・だんだん減って止まる人・
たまに使う人・登録しただけの人を混ぜ、平日と休日の差、科目の得意不得意、
予定を立てたのに手を付けなかった日も作ります。全員が毎日きちんと記録する前提のデータだと、
索引もキャッシュも実際より効いてしまうためです。

何を確かめたいかで欲しい集団は変わるので、`SCENARIO`で切り替えます。実行すると、
設定値ではなく**できあがった集団**（継続率と1人あたりの件数）を表示します。

| SCENARIO | どんな状態か | 直近7日に記録がある人 | 1人あたりの実績 |
|---|---|---|---|
| `default` | 続く人と辞める人が現実的な比率 | 28% | 47件 |
| `churn` | 大半が辞めていく | 14% | 26件 |
| `engaged` | よく使われている（書き込みが多い） | 45% | 73件 |
| `growth` | 最近になって登録が増えた（過去が薄い） | 49% | 31件 |

```bash
SCENARIO=churn pnpm run db:seed:synthetic     # 継続率が低いときの通知バッチや集計を見る
SCENARIO=engaged ACTIVITY=1.5 pnpm run db:seed:synthetic
```

個別の調整は`ACTIVITY`（記録の頻度の倍率）、`RECENCY`（登録日が今日に寄る度合い）、
`COHORT_MIX`（4種類の割合）で上書きできます。同じ`SEED`なら何度実行しても同じデータになり、
実行のたびに入れ直します。

合成した利用者は`@synthetic.juken-map.invalid`のメールアドレスを持ち、全員同じパスワードで
ログインできるので、k6の負荷試験からそのまま使えます。削除はこの印が付いた利用者だけを対象に
するため、既存のデータは残ります。手元のDB以外へは流れません
（意図的に流すときだけ`ALLOW_REMOTE=on`）。

## メトリクス・ログ・トレースをGrafanaで見る

APIのリクエスト数・エラー率・レスポンスタイム・CPU・メモリ（Prometheus）、APIのログ（Loki）、1回のリクエストの内訳（Tempo）を、手元のGrafanaで確認できます（本番はGrafana Cloudへ送ります。[運用ガイド](operations.md#本番の可観測性grafana-cloud)を参照）。

1. `.env`に次の3行を足してから`pnpm dev`で起動する
   ```bash
   METRICS_PORT="9464"                                # APIが別ポートで/metricsを出す
   LOG_FILE="logs/api.log"                            # APIが本番と同じJSONのログをファイルにも書く
   OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318" # APIがトレースをTempoへ送る
   ```
2. `pnpm run obs:start`でPrometheus・Grafana・Loki・Alloy・Tempo・Mailpitを起動する
3. `http://localhost:3001`（`admin` / `admin`）の「juken-map API」ダッシュボードを開く

ログは、APIが`logs/api.log`に書いたものをAlloyが読んでLokiへ送ります（`http://localhost:12345`でAlloyの処理の流れを見られます）。ダッシュボードで時間の範囲を絞ると、その時間のエラーのログと5xxを返したリクエストが下に並びます。1つのリクエストの行をまとめて見るときは、Exploreで`{job="juken-map-api"} |= "<reqId>"`と検索します。

トレースは、ダッシュボードの一番下の表でTrace IDを押すと、1回のリクエストの内訳が開く作りです。ログの行の`trace_id`からも同じトレースを開けます。

5xxの割合が1%を超えた状態が1分続くとアラートのメールが送られ、`http://localhost:8025`（Mailpit）で受け取れます。設定は[observability/](../observability/)にあります。
## 完了の確認

次の状態になれば、ローカルセットアップは完了です。

- `http://localhost:5173` でトップページが表示される
- ログイン画面からデモユーザーでログインできる
- ダッシュボードに学習予定や志望校が表示される

## 環境変数

設定項目は [.env.example](../.env.example) を参照してください。

| 変数 | 用途 |
|---|---|
| `DATABASE_URL` | MySQLへの接続 |
| `BETTER_AUTH_SECRET` | 2段階認証の秘密を暗号化する鍵の元（名前は Better Auth の名残。`AUTH_TOTP_KEYS` があればそちら） |
| `AUTH_GOOGLE_ID` / `AUTH_GOOGLE_SECRET` | Google OAuth |
| `AUTH_GITHUB_ID` / `AUTH_GITHUB_SECRET` | GitHub OAuth |
| `RESEND_API_KEY` | メール確認・パスワード再設定・各種メール通知 |
| `ADMIN_NOTIFICATION_EMAIL` | 新規ユーザー登録の通知先メールアドレス |
| `DAILY_NOTIFICATION_SECRET` | 朝・夜の学習通知APIを保護する秘密値。本番では EC2 の systemd timer（[infra/systemd/](../infra/systemd/)）が朝7時・夜21時に呼ぶ。GitHub Actionsにも同じ値を置き、手動の送り直しに使う |
| `CHAOS_ENABLED` / `CHAOS_SECRET` | `on`のときだけ障害注入（遅延・5xx・DB の失敗・外部 API のタイムアウト）を有効にし、始める API（`/api/chaos/*`）をその秘密値で保護する（任意。詳細は[apps/api/README.md](../apps/api/README.md)） |
| `SIMULATION_ENABLED` / `SIMULATION_SECRET` | `on`のときだけシミュレーション用API（`/api/sim/*`）を有効にし、その秘密値で保護する（任意。詳細は[sim/README.md](../sim/README.md)） |
| `RESEND_READ_API_KEY` | シミュレーターだけが使う、送ったメールを読めるResendのキー（任意。アプリ本体には設定しない） |
| `LINE_CHANNEL_SECRET` | LINE Messaging APIのWebhook署名検証 |
| `LINE_CHANNEL_ACCESS_TOKEN` | LINE公式アカウントからの通知送信・アカウント連携 |
| `LINE_LOGIN_CHANNEL_ID` | プロフィールから直接LINE連携するLINE LoginチャネルID |
| `LINE_LOGIN_CHANNEL_SECRET` | LINE Loginの認可コード交換 |
| `MICROCMS_API_KEY` / `MICROCMS_SERVICE_DOMAIN` | ブログ記事の取得。本番のビルドでも記事を SSG するため、GitHub Secrets にも置く |
| `MICROCMS_WEBHOOK_SECRET` / `GITHUB_DEPLOY_TOKEN` | microCMS で記事を変えたら、Go が Webhook を受けて deploy.yml を動かし、記事を作り直す。本番は Secrets Manager に置き、Go にだけ渡す |
| `METRICS_PORT` | 設定したときだけ、そのポートでPrometheus用の`/metrics`を出す（任意） |
| `LOG_FILE` | 手元の開発で、JSONのログをこのファイル（リポジトリのルートからの相対パス）にも書く。Loki用（任意） |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | 手元の開発で、設定したときだけOpenTelemetryのトレースをこの送り先（OTLP/HTTP）へ送る。Tempo用（任意）。本番はデプロイが Alloy の宛先を渡す |

## 開発コマンド

| コマンド | 説明 |
|---|---|
| `pnpm dev` | MySQLとマイグレーションを準備し、Go・nginx・Viteを並列で起動する |
| `pnpm run dev:go` | Go（API・ログイン）を4100番で起動する |
| `pnpm run dev:proxy` | nginx（Docker）を4200番で起動し、本番と同じ振り分けでGoへ送る |
| `pnpm run dev:web` | Vite（画面）を5173番で起動する。`/api`はnginxへ送る |
| `pnpm run lint` | ESLintを実行する |
| `pnpm run test` | ルート（`src/`）のVitestを実行する |
| `pnpm run e2e` | PlaywrightのE2Eテストを実行する（本番と同じくnginxの後ろにGoを立てる） |
| `pnpm run check` | Lint、型チェック、3種のVitest、SPAビルドをまとめて実行する |
| `pnpm run db:start` | MySQLコンテナを起動する |
| `pnpm run db:stop` | MySQLコンテナを停止する |
| `pnpm run db:logs` | MySQLコンテナのログを表示する |
| `pnpm run db:migrate` | まだ当てていないマイグレーション（`db/migrations/*/migration.sql`）をDBへ当てる |
| `pnpm run db:seed` | 大学マスターとデモユーザーをローカルDBへ投入する（何度流しても同じ状態になる） |
| `pnpm run admin:grant <メール>` | そのユーザーを管理者（`/admin`を開ける）にする。メール確認済みのユーザーだけ。`--revoke`で戻す。`--list`で管理者の一覧（2段階認証・パスワードの有無つき） |
| `pnpm run incident <操作> [メール]` | 乗っ取りが起きたときの操作（セッションを見る・消す、止める、管理者全員のセッションを消す、2段階認証を戻す）。手順は`docs/incident-response.md` |
| `pnpm run db:shell` | ローカルDB（`juken_map`）のMySQL対話画面を開く（`exit`で終了）。`-e "SQL"`を付けると1本だけ実行する |
| `pnpm run obs:start` | Prometheus（9090番）・Grafana（3001番）・Loki（3100番）・Alloy（12345番）・Tempo（3200番）・Mailpit（8025番）を起動する |
| `pnpm run obs:stop` | Prometheus・Grafana・Loki・Alloy・Tempo・Mailpitを停止する |
| `pnpm run capture:seed` | LP撮影用ユーザーをローカルDBへ投入する |
| `pnpm run hooks:install` | リポジトリ管理のGitフックを有効にする |
| `pnpm wt:new <ブランチ名>` | 作業用のgit worktreeを作り、リンク・ポート・依存のインストールまで済ませる |
| `pnpm wt:remove <ブランチ名>` | worktreeを片付ける。マージ済みならブランチも消す |
| `pnpm run lock:check` | 隔離ディレクトリでmanifestとlockfileの整合性を検証する |
| `pnpm run lock:linux` | DockerのLinux/amd64環境でfrozen installとtsx・Viteの起動を検証する |
| `pnpm run lock:fix` | lockfileを更新し、Linuxで検証する |

`apps/web`と`db/`のテストは、それぞれ`pnpm --filter @juken-map/web test`と
`pnpm --filter @juken-map/db test`で個別に実行できます（`pnpm run check`には含まれます）。

## テストとCI

変更をpushする前に、次のコマンドで主要な検査をまとめて実行できます。

```bash
pnpm run check
```

ルートと`apps/web`・`db`はpnpm workspaceです（`apps/api`はGoのモジュールで、workspaceの外）。各アプリの依存はそれぞれの`package.json`に宣言し、解決結果はルートの`pnpm-lock.yaml`で共有します。依存追加は、例えば`pnpm --filter @juken-map/web add パッケージ名`、ルートの開発依存なら`pnpm add -Dw パッケージ名`を使います。

manifest・`pnpm-workspace.yaml`・`pnpm-lock.yaml`を含むコミットでは、pre-commitフックがステージ済みの内容に対して非破壊のlockfile検証を実行します。依存変更後は`pnpm install`に続けて`pnpm run lock:linux`でLinux/amd64のインストール成功を確認し、manifestとlockfileを一緒にコミットしてください。CIでも`pnpm install --frozen-lockfile`を使います。

依存パッケージのinstall scriptは`pnpm-workspace.yaml`の`allowBuilds`で必要なものだけ許可しています。新しい依存でビルド未承認のエラーが出た場合は、スクリプトの内容を確認してこの設定を更新します。

GitHub Actionsでは、次の4ジョブを実行します。

- `lockfile` — Linux向けネイティブ依存関係がlockfileに含まれるか確認し、本番の依存を監査する
- `check` — Lint、型チェック、Vitest、OpenAPIの型の生成結果、デプロイスクリプトの検査、SPAのビルドを実行する
- `go` — Goのフォーマット、vet、Lint、テスト、脆弱性とSQLインジェクションの検査、Dockerビルドを実行する
- `e2e` — MySQLサービスコンテナ上でPlaywrightを実行する
