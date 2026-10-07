# 起きたときの手順

秘密情報の漏えいや乗っ取りが起きたときに、最初にやることを書いておく。
起きてから手順を考えると止めるのが遅れるため（開発基準 06 H5）。
手順の中の操作は1回ずつ試し、試した日を[最後の表](#試した記録)に書く。手順を変えたら試し直す。

| 起きたこと | 最初にやること | 手順 |
| --- | --- | --- |
| 利用者のアカウントの乗っ取り | その人を止め、セッションを全部消す。本人へ連絡する | [1](#1-利用者のアカウントの乗っ取り) |
| 管理者のアカウントの乗っ取り | 管理者全員のセッションを消し、2段階認証を設定し直す。管理操作の記録を確かめる | [2](#2-管理者のアカウントの乗っ取り) |
| 秘密情報の漏えい | その値を差し替え、古い値を無効にする。その値でできた操作の記録を確かめる | [3](#3-秘密情報の漏えい) |
| 個人データの漏えい（おそれを含む） | 範囲を記録から特定する。報告と通知の要否と期限を判断する | [4](#4-個人データの漏えい) |
| CI・配布経路の侵害 | デプロイを止め、CI のロールを無効にする。本番を信頼できるイメージに戻す | [5](#5-ci配布経路の侵害) |

## 共通

- **記録は非公開の場に残す**。このリポジトリは公開なので、起きたことの経過は GitHub Security Advisory
  （非公開）に書く。Linear や Issue・PR には書かない。何時に何を見て何をしたかを、その場で書き足していく
- **本番の EC2 には SSM で入る**（SSH は閉じている）。入った直後は `ssm-user` なので `sudo` を付ける

  ```sh
  aws ssm start-session --target i-0eeb166295363e11d --region ap-northeast-1
  ```

- **アカウントの操作は Go の `incident` コマンドで行う**（`apps/api/internal/app/cli.go`・`apps/api/internal/feature/ops/`、手元では `pnpm incident`）。
  本番の RDS には外から繋げないので、EC2 で動いている Go のコンテナの中で実行する（コンテナの DB の接続をそのまま使う。
  distroless でシェルが無いので、実行ファイルを直接呼ぶ）。管理画面の「停止」と違い、管理者も止められる

  ```sh
  # EC2 の中で。最後の2語を差し替える
  sudo docker exec juken-map-go /api incident sessions <メールアドレス>
  ```

  実行ファイルは JUK-131 で `/api-go` から `/api` に名前を変えた。それより前のイメージへロールバックしているときは
  `/api-go` で呼ぶ（コンテナ名 `juken-map-go` は変えていない）。

  | 操作 | すること |
  | --- | --- |
  | `sessions <メール>` | ログイン中のセッションを、作られた日時・最後に使った日時・期限・2段階認証の有無・IP・ブラウザつきで出す |
  | `revoke <メール>` | セッションを全部消す（止めはしない） |
  | `ban <メール>` | 止めて、セッションを全部消す。止めた人は次のログインで断られる |
  | `unban <メール>` | 止めたのを戻す |
  | `revoke-admins` | 管理者全員のセッションを消す |
  | `revoke-all` | 全員のセッションを消す（全員がログインし直し）。ログインの不具合や、セッションの表が漏れた疑いのとき |
  | `reset-2fa <メール>` | 2段階認証を設定前に戻し、セッションを全部消す |
  | `log` | 下の「運用コマンドの記録」を新しい順に50件出す |

- **運用コマンドで変えたことは DB の `OpsAuditLog` に残る**（JUK-138、セキュリティ基準 H4）。`docker exec` の出力は
  実行した人の端末にしか出ず、コンテナのログ（Loki）に乗らないため。`incident` の変える操作と `grant-admin`（`set-role`）が
  対象で、`sessions`・`--list`・`log` のような見るだけの操作は残さない。1年で消える（CloudTrail と同じ）

  ```sh
  # EC2 の中で。列は「日時（UTC）・操作・対象の userId・実行したコンテナ・前後の値」
  sudo docker exec juken-map-go /api incident log
  ```

  コンテナの中からは SSM で入った人が分からないので、**誰が**は CloudTrail の `StartSession` と日時で突き合わせる
  （90日より前は 5 の手順 3 のとおり S3 から）。

  ```sh
  aws cloudtrail lookup-events --region ap-northeast-1 \
    --lookup-attributes AttributeKey=EventName,AttributeValue=StartSession \
    --query 'Events[].{t:EventTime,who:Username}' --output text
  ```

- **Grafana はエージェントか CLI で見る**。トークンは Terraform 用（`TF_VAR_grafana_auth`）を使う。

  ```sh
  G=https://kindcrest3516.grafana.net/api/datasources/proxy/uid
  H="Authorization: Bearer $TF_VAR_grafana_auth"
  # メトリクス（例：直近15分の 401 を route 別に）
  curl -sS -G -H "$H" --data-urlencode \
    'query=sum by (route, method) (increase(http_requests_total{env="production",status_code="401"}[15m])) > 0' \
    $G/grafanacloud-prom/api/v1/query
  # ログ（例：直近1時間の管理操作。start はナノ秒）
  curl -sS -G -H "$H" --data-urlencode 'query={job="juken-map-api", env="production"} |= "admin user action"' \
    --data-urlencode "start=$(( $(date +%s) - 3600 ))000000000" $G/grafanacloud-logs/loki/api/v1/query_range
  ```

- **アプリのログに IP は無い**（nginx 越しで意味が無いため出していない）。どこから来たかは、
  セッションの IP（`pnpm incident sessions`）と、EC2 の nginx のアクセスログ（`/var/log/nginx/access.log*`）で見る

## アラートが鳴ったら

アラートの本文にある「確かめること」を見て、下の手順のどれに当たるかを決める。
Grafana のアラートとは別に、AWS の GuardDuty の検出結果が「[GuardDuty] …」の件名でメールに届く（JUK-37、`terraform/detection.tf`）。
アプリや Grafana が止まっていても届く。試験用に作ったものは、題名や種類に `[SAMPLE]`・`i-99999999` が入る。

| アラート | まず見るもの | 当たりそうな手順 |
| --- | --- | --- |
| 認証失敗（401）の急増 | 401 を route 別に（上の例）。ログの `[auth] sign_in_failure` の `reason` と `ip` | 総当たりなら B4 の制限が効いているかを 429 で見る。特定の人が入られていれば 1 |
| 権限なし（403）の急増 | 403 を route 別に。`/api/admin/*` が多いか | 管理画面を探られているなら 2 の準備。ほかの人の ID を探られているなら 4 |
| 回数制限（429）の急増 | ログの `TOO_MANY_SIGN_IN_ATTEMPTS` と、nginx のアクセスログの IP。ログインした人の叩きすぎは `request limited: per user`（`userId` と読み取り・書き込みの別） | 狙われたアカウントの本人へ連絡（1 の手順 4）。特定の利用者が叩き続けているなら、乗っ取りやスクリプトを疑い、必要なら止める（1） |
| 管理画面の書き込みの急増 | ログの `admin user action`（誰が・誰に・何を） | 自分の操作でなければ 2 |
| メール送信の急増・上限で止めた | `email_sends_total` を `kind` 別に。ログの `[email-limits]` | 登録や再設定の連打。続くなら送信元の IP を nginx で止める |
| 監視の途絶（メトリクス・ログ） | EC2 で `sudo docker ps`。`juken-map` と `juken-map-alloy` が動いているか | 止められていれば、誰が止めたかを 5 の手順 3 で調べる |
| API（Go）：停止 | EC2 で `sudo docker ps`、`sudo docker logs juken-map-go` | 障害なら直す。侵害の疑いがあれば 5 |
| GuardDuty の検出結果（メール） | メールのリンクから検出結果を開き、「リソース」（どの EC2・IAM の鍵・ロールか）と「アクター」（どこの IP から何をしたか）を見る。同じ時刻の操作を CloudTrail で引く（5 の手順 3） | IAM の鍵なら 3 の「手元の AWS の鍵」、CI のロールなら 5。EC2 なら乗っ取りとみなして 3（EC2 の中の秘密情報）と 5。ルートでのログイン（`RootCredentialUsage`）や CloudTrail の停止は、自分でなければすぐルートのパスワードと MFA を見直す |

## 1. 利用者のアカウントの乗っ取り

1. 止めて、セッションを全部消す。本人のパスワードが変わっていても、止めれば入れない

   ```sh
   sudo docker exec juken-map-go /api incident sessions <メール>
   sudo docker exec juken-map-go /api incident ban <メール>
   ```

   止める前に `sessions` の結果（IP・ブラウザ・作成日時）を記録に写す。消すと残らない。

2. 何をされたかを見る。その人のデータ（志望校・記録・予定・参考書）が変わった時刻を、
   セッションの作成日時と突き合わせる。ログで見るなら、その時間帯の書き込み（`method` が GET 以外）を引く
3. 入られた経路を考える。パスワードの使い回しか、Google / GitHub のアカウント側か、セッションの盗用か。
   ほかの人にも同じ形の入り方が無いか、401・429 のアラートの時間帯と比べる
4. 本人へ連絡する（登録のメールアドレスへ、運営者のアドレスから）。伝えること：止めたこと、
   パスワードの再設定（「パスワードを忘れた方」）、ほかのサービスで同じパスワードを使っていれば変えること
5. 本人と確かめが取れたら戻す。戻したあと、本人に再設定してもらう

   ```sh
   sudo docker exec juken-map-go /api incident unban <メール>
   ```

## 2. 管理者のアカウントの乗っ取り

管理 API は、2段階認証を通したセッションだけを通す（B7）。それでも入られたなら、パスワードと
認証アプリの両方か、セッションそのものを取られている。

1. 管理者全員のセッションを消す。今の管理者は `grant-admin --list` で確かめる

   ```sh
   sudo docker exec juken-map-go /api incident revoke-admins
   sudo docker exec juken-map-go /api grant-admin --list
   ```

2. 乗っ取られた管理者を止めるか、管理者から外す。知らない管理者が増えていれば外す

   ```sh
   sudo docker exec juken-map-go /api incident ban <メール>
   sudo docker exec juken-map-go /api grant-admin <メール> --revoke
   ```

3. 2段階認証を設定し直す。認証アプリの秘密が漏れたかもしれないので、全員分を戻し、
   パスワードを再設定してから `/admin` で設定し直す（QR を読み、予備コードを保存し直す）

   ```sh
   sudo docker exec juken-map-go /api incident reset-2fa <メール>
   ```

4. 管理操作の記録を確かめる。ログの `admin user action` に、誰が（`adminId`）・誰に（`targetId`・`targetEmail`）・
   何を（`action`：ban・unban・delete）したかが残る。マスター（大学・参考書）の編集は、
   `/api/admin/` の書き込みを route 別に数えて時間帯を見る
5. 消された利用者がいれば、戻せるかを考える。RDS の自動バックアップから、時刻を指定して
   別のインスタンスに復元できる（保持は1日。本番を上書きせず、別インスタンスから必要な行だけ写す）

## 3. 秘密情報の漏えい

漏れた値を差し替え、古い値を無効にする。そのあと、古い値で何ができたかを、その値の記録で確かめる。

| 値 | 置き場所 | 差し替えと古い値の無効化 | 記録 |
| --- | --- | --- | --- |
| `BETTER_AUTH_SECRET` | EC2 の `.env` | 今の用途は、2段階認証の秘密を暗号化する鍵（版 v0）を導くことだけ（`AUTH_TOTP_KEYS` が無いとき。JUK-115）。漏れた値だけでは何もできない（セッション・メールのリンクは乱数のトークンを DB にハッシュで持つので、偽造できない）。DB も漏れていれば2段階認証の秘密が読めるので、管理者は `reset-2fa` で設定し直す。新しい値にして再デプロイすると v0 の鍵が変わり、v0 で暗号化した秘密は読めなくなるので、同じく `reset-2fa` | DB の漏えいの有無（下の 4） |
| `AUTH_TOTP_KEYS` | EC2 の `.env`（置いたとき） | 新しい版を先頭に足して再デプロイ（古い版も残す）。秘密は次にコードを通したときに新しい版で書き直される。鍵と DB の両方が漏れていれば秘密が読めるので、`reset-2fa` で設定し直し、古い版を消す | 同上 |
| `DATABASE_URL`（`juken_app`）・`MIGRATION_DATABASE_URL`（`juken_migrate`） | Secrets Manager `juken-map/production/runtime` | 下の「DB のパスワード」 | RDS には外から繋げないので、使えるのは EC2 の中からだけ。EC2 に入られていないかを 5 の手順 3 で見る |
| RDS のマスター（`admin`） | EC2 の `.env`（JUK-86 で外す予定） | `aws rds modify-db-instance --db-instance-identifier juken-map-db --master-user-password <新しい値> --apply-immediately` | 同上 |
| `AUTH_GOOGLE_SECRET`・`AUTH_GITHUB_SECRET` | EC2 の `.env` | Google Cloud・GitHub の OAuth アプリの設定で新しい秘密を作り、古い方を消す。`.env` を直して再デプロイ。漏れた値だけでは利用者になりすませない（認可コードも要り、コードは PKCE の code_verifier が無いとトークンに替えられない）。受験マップを名乗る偽の OAuth アプリを作られうる。作り直すと、差し替えるまで Google・GitHub ログインが止まる（セッションとパスワードには影響しない） | 各 OAuth アプリの設定画面 |
| `LINE_*` | Secrets Manager | LINE Developers でチャネルシークレット・アクセストークンを再発行する。Secrets Manager を直して再デプロイ | LINE 公式アカウントの送信履歴 |
| `RESEND_API_KEY` | EC2 の `.env` | Resend の API Keys で新しい鍵を作り、古い鍵を消す。`.env` を直して再デプロイ。漏れた値では受験マップの差出人でメールを送れる（なりすましのメール）、送信枠を使い切って確認・再設定のメールを止められる。作り直すと、差し替えるまでメールが送れない（ログインそのものは止まらない） | Resend の Emails（送ったメールの一覧） |
| `RESEND_READ_API_KEY` | GitHub Secrets | 同上（読み取りだけの鍵）。`gh secret set RESEND_READ_API_KEY` | 同上 |
| `MICROCMS_API_KEY` | EC2 の `.env` と GitHub Secrets（ビルドで記事を SSG するため、JUK-110） | microCMS の API キーで作り直し、古いキーを消す。`.env` と `gh secret set MICROCMS_API_KEY` の両方を直して再デプロイ | microCMS の API キーの設定画面 |
| `MICROCMS_WEBHOOK_SECRET` | Secrets Manager（Go にだけ渡す、JUK-112） | 新しい値を作り、microCMS の API の設定の Webhook のシークレットと Secrets Manager の両方を直して再デプロイ。漏れた値では、記事を作り直すデプロイを何度でも動かせる（記事の中身は変えられない） | microCMS の Webhook のログと、Actions の deploy.yml の実行（`workflow_dispatch`） |
| `GITHUB_DEPLOY_TOKEN` | Secrets Manager（Go にだけ渡す、JUK-112） | GitHub の Fine-grained tokens で古いトークンを消し、同じ権限（このリポジトリの Actions: Read and write だけ）で作り直す。Secrets Manager を直して再デプロイ。漏れたトークンでは、ワークフローの実行・取り消し・再実行、実行の記録の削除、ワークフローの有効・無効の切り替えができる（コードと Secrets には触れない） | リポジトリの Actions の実行の一覧（`workflow_dispatch` で誰が動かしたか） |
| `GRAFANA_CLOUD_TOKEN` | Secrets Manager | Grafana Cloud の Access Policies でトークンを作り直し、古いトークンを消す。Secrets Manager を直して再デプロイ | Grafana Cloud の使用量 |
| Terraform 用の Grafana のトークン | 手元の `~/.zshrc`（`TF_VAR_grafana_auth`） | Grafana の Service accounts（`sa-1-terraform`）でトークンを作り直し、古いトークンを消す | Grafana の監査（アラートの設定が変わっていないかを `terraform plan` で） |
| `DAILY_NOTIFICATION_SECRET`・`SIMULATION_SECRET` | EC2 の `.env` と GitHub Secrets | 下の「機械の入口のトークンを差し替える」 | 401 以外の応答をログで |
| 手元の AWS の鍵 | 手元の `~/.aws` | IAM でアクセスキーを無効にし、新しいキーを作る | CloudTrail（5 の手順 3） |

**新しい値を作る**：

```sh
openssl rand -base64 48
```

**EC2 の `.env` を直す**：SSM で入り、`sudo -iu ubuntu` で `/home/ubuntu/juken-map/.env` の該当行を書き換える。
コンテナは作成時にしか `.env` を読まないので、GitHub Actions の「Deploy to EC2」を手動で実行する
（`gh workflow run deploy.yml`）。
対話のセッションを使えないとき（エージェントの `!` など）は `aws ssm send-command` でも直せるが、
コマンドの中身は SSM の実行履歴に残るので、行を消すような秘密を含まない変更だけにする。

**Secrets Manager を直す**：1つのシークレットに JSON でまとめて入っているので、1つの項目だけを差し替えて書き戻す。
値はシェルの履歴に残さないよう、`read -s` で受ける。

```sh
read -rs NEW_VALUE   # 新しい値を貼って Enter
aws secretsmanager get-secret-value --region ap-northeast-1 --secret-id juken-map/production/runtime \
    --query SecretString --output text \
  | jq --arg v "$NEW_VALUE" '.<項目名> = $v' \
  | aws secretsmanager put-secret-value --region ap-northeast-1 --secret-id juken-map/production/runtime \
    --secret-string file:///dev/stdin
unset NEW_VALUE
```

そのあと再デプロイする。前の版は `AWSPREVIOUS` として残るので、間違えたら戻せる。

**DB のパスワード**：EC2 の中から、マスターで RDS に入って変える。RDS のエンドポイントは
`aws rds describe-db-instances --db-instance-identifier juken-map-db --query 'DBInstances[0].Endpoint.Address'` で引く。

```sh
# EC2 の中で。パスワードを聞かれたらマスターのものを入れる
sudo docker run --rm -it mysql:8.4 mysql -h <RDS のエンドポイント> -u admin -p juken_map
```

```sql
ALTER USER 'juken_app'@'%' IDENTIFIED BY '<新しい値>';
```

`ALTER USER` した時点で古いパスワードの新しい接続は断られるが、アプリのコンテナは古い値を持ったままなので、
すぐに Secrets Manager の `DATABASE_URL` を新しい値に直して再デプロイする（その間、新しい接続を作れず API が失敗する）。

### 機械の入口のトークンを差し替える

本番の機械向けの入口を守るトークンが2つある。
漏れた（ログや画面に出た、コミットした、など）と思ったら、両方の置き場所を新しい値にそろえる。

| トークン | 守っている入口 | 置き場所 |
| --- | --- | --- |
| `DAILY_NOTIFICATION_SECRET` | `POST /api/cron/daily-study-notifications`（朝・夜の学習通知） | 本番サーバーの `.env` と GitHub Secrets（手動の送り直し用）。定期の送信は EC2 の systemd timer が呼ぶ（JUK-85）。タイマーが読む `/etc/juken-map/daily-notification.env` は、デプロイが `.env` から書く |
| `SIMULATION_SECRET` | `/api/sim/*`（シミュレーション） | 本番サーバーの `.env` と GitHub Secrets |

用途ごとに別の値にする。同じ値を使い回すと、片方が漏れたときに両方を差し替えることになる。

1. 新しい値を作る。

   ```sh
   openssl rand -base64 48
   ```

2. 本番サーバーの `.env`（`/home/ubuntu/juken-map/.env`）の該当行を書き換える。
   SSH は閉じているので、SSM Session Manager で入る。

3. アプリを作り直して新しい値を読ませる。コンテナは作成時にしか `.env` を読まないため、
   GitHub Actions の「Deploy to EC2」を手動で実行する（`gh workflow run deploy.yml`）。
   `DAILY_NOTIFICATION_SECRET` なら、同じデプロイで通知のタイマーが読むファイルも新しい値になる。

4. GitHub Secrets を同じ値にする。

   ```sh
   gh secret set DAILY_NOTIFICATION_SECRET   # 値は標準入力から貼る
   ```

5. 確かめる。古い値で叩いて 401 になり、GitHub Actions の手動実行が成功すること。
   通知のタイマーは、次の送信のあとに `journalctl -u 'juken-map-daily-notification@*'` で結果を見る。

   ```sh
   curl -s -o /dev/null -w "%{http_code}\n" -X POST \
     -H "Authorization: Bearer <古い値>" -H "Content-Type: application/json" \
     -d '{"slot":"morning"}' https://juken-map.com/api/cron/daily-study-notifications
   ```

3 と 4 の間は、GitHub Actions からの呼び出しが 401 になる（定期の送信はタイマーなので影響しない）。
`SIMULATION_SECRET` は定期の呼び出しに使われているので、差し替えはその実行の前後を避ける。

`SIMULATION_SECRET` は、シミュレーションが保存するログイン状態の暗号化の鍵も兼ねている。
差し替えると前回までの状態を読めなくなり、次の実行はログインし直すところから始まる。

## 4. 個人データの漏えい

おそれの段階でも始める。報告の期限は「知った時」から数えるので、確かめ終わるのを待たない。

1. 範囲を記録から特定する。何が・何人分・いつからいつまで、の3つを、分かった根拠と一緒に記録に書く
   - DB の主な個人データ：`user`（メール・名前・画像・ニックネーム）、`AuthIdentity`（Google / GitHub の連携。トークンは持たない）、
     `AuthSession`（IP・ブラウザ）、学習のデータ（志望校・記録・予定・参考書）、LINE の連携（LINE のユーザー ID）。
     Better Auth の表（`account`・`session` など）と移管済みの課金の表は、2026-10-05 に消した（JUK-127）
   - 経路ごとの記録：アプリのログとメトリクス（Grafana Cloud、プランの保存期間まで）、nginx のアクセスログ（EC2）、
     AWS の操作（CloudTrail。90日より前は S3 に1年。5 の手順 3）、送ったメール（Resend）
   - 他人のデータを読まれた疑いなら、403・404 が多い route と時間帯から、どの ID が試されたかを見る
2. 漏れ続けていれば止める。入口がアプリなら、その route を nginx で止めるか、直してデプロイする。
   資格情報が漏れていれば 3 を、配布経路なら 5 を並行して行う
3. 報告と本人への通知の要否を判断する（個人情報保護法）。次のどれかに当たれば、個人情報保護委員会への報告と、
   本人への通知が要る。**外部からの不正アクセスによるものは、人数に関係なく③に当たる**
   - ① 要配慮個人情報（このアプリは集めていない）　② 財産的被害のおそれがあるもの
   - ③ 不正の目的によるおそれがあるもの　④ 1,000人を超えるもの
4. 期限を守って報告する。速報は知ってから**おおむね3〜5日以内**、確報は**30日以内（③は60日以内）**。
   要件と様式は、報告の前に委員会の「漏えい等の対応」（https://www.ppc.go.jp/personalinfo/legal/leakAction/ ）で確かめる
5. 本人へ通知する。伝えること：何が漏れたか（漏れたおそれがあるか）、いつ分かったか、何をしたか、
   本人にしてほしいこと（パスワードの変更、Google / GitHub の連携の見直しなど）、問い合わせ先。
   利用者には高校生が多いので、平易に書く

## 5. CI・配布経路の侵害

CI のロール（`github-actions-juken-map-ecr`）は、ECR への push と、EC2 への SSM Run Command ができる。
SSM のコマンドは EC2 の root で動くので、CI を乗っ取られたら、EC2 の中の秘密情報もすべて漏れたとみなす。

1. デプロイを止める。動いている実行は取り消す

   ```sh
   for wf in deploy.yml simulation.yml daily-study-notifications.yml; do gh workflow disable "$wf"; done
   gh run list --status in_progress
   gh run cancel <実行の ID>
   ```

2. CI のロールを無効にする。すべてを拒否するポリシーを足すと、すでに発行された一時的な資格情報も次の呼び出しから断られる

   ```sh
   aws iam put-role-policy --role-name github-actions-juken-map-ecr --policy-name incident-deny-all \
     --policy-document '{"Version":"2012-10-17","Statement":[{"Effect":"Deny","Action":"*","Resource":"*"}]}'
   ```

3. 何をされたかを見る

   ```sh
   # CI のロールが引き受けられた記録。sub が main 以外なら怪しい（lookup-events で引けるのは90日分）
   aws cloudtrail lookup-events --region ap-northeast-1 \
     --lookup-attributes AttributeKey=EventName,AttributeValue=AssumeRoleWithWebIdentity \
     --query 'Events[].{t:EventTime,sub:Username}' --output text
   # 90日より前は S3 から（1年残る。JUK-37）。日付のフォルダごとに取って開く。
   # IAM の操作やルートのログインのような全体の操作は us-east-1 のフォルダに入る
   A=$(aws sts get-caller-identity --query Account --output text)
   aws s3 cp --recursive s3://juken-map-cloudtrail-$A/AWSLogs/$A/CloudTrail/ap-northeast-1/<年>/<月>/<日>/ ./ct
   gunzip -c ./ct/*.json.gz | jq -r '.Records[] | select(.eventName=="AssumeRoleWithWebIdentity")
     | [.eventTime, .userIdentity.userName] | @tsv'
   # EC2 で実行されたコマンド。日付で絞らないと古いものから返る
   aws ssm list-commands --region ap-northeast-1 --filters key=InvokedAfter,value=<UTC の日時> \
     --query 'Commands[].[RequestedDateTime,Status,Comment]' --output text
   # ECR に push されたイメージ（タグはコミット、push の時刻、ダイジェスト）
   aws ecr describe-images --region ap-northeast-1 --repository-name juken-map \
     --query 'sort_by(imageDetails,&imagePushedAt)[-10:].[imageTags[0],imagePushedAt,imageDigest]' --output text
   # ワークフローの変更と、リポジトリの鍵・Webhook・共同作業者
   git log --since=<日付> -p -- .github/
   gh api repos/shimaiku1960/juken-map/keys
   gh api repos/shimaiku1960/juken-map/hooks
   gh api repos/shimaiku1960/juken-map/collaborators --jq '.[].login'
   ```

4. 秘密情報を差し替える。GitHub Secrets（`gh secret list`）の全部と、EC2 の `.env`・Secrets Manager の全部を 3 の表で差し替える
5. 本番を信頼できるイメージに戻す
   - 信頼できるコミットを決める（侵害より前の `main`）。ECR のそのタグの push の時刻が、そのコミットの
     「Deploy to EC2」の実行の時刻と合うかを確かめる。合わなければ、手元で作り直して別のタグで push する

     ```sh
     aws ecr get-login-password --region ap-northeast-1 \
       | docker login --username AWS --password-stdin 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com
     git worktree add --detach ../juken-map-restore <信頼できるコミット> && cd ../juken-map-restore
     c=$(git rev-parse HEAD)
     # deploy.yml と同じ引数で作る。APP_COMMIT が無いと /api/health のコミットが空になり、
     # microCMS の鍵が無いとブログの記事が SSG されない。鍵は手元の .env から読む（4 で差し替えたなら新しい値）
     set -a; . ../juken-map/.env; set +a
     docker buildx build --platform linux/amd64 --push \
       --build-arg APP_COMMIT=$c \
       --secret id=microcms_api_key,env=MICROCMS_API_KEY \
       --build-arg MICROCMS_SERVICE_DOMAIN --build-arg SSG_ARTICLES=required \
       -t 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map:restore-$c .
     # Go のイメージは画面（JUK-111）を上のイメージから写し、マイグレーション（JUK-125）を db/migrations から入れる。
     # web を忘れると画面の無いイメージになり、デプロイのスモークテスト（Go の /login）で止まる。
     # migrations を忘れると、デプロイの migrate が「マイグレーションがありません」で止まる
     docker buildx build --platform linux/amd64 --push --build-arg APP_COMMIT=$c \
       --build-context web=docker-image://961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map:restore-$c \
       --build-context migrations=db/migrations \
       -t 961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map-go:restore-$c apps/api
     ```

   - CI を使わずに、手元から deploy.yml と同じコマンドを送る（信頼できるコミットのファイルを使う）

     ```sh
     tag=<コミット か restore-<コミット>>
     remote="echo '$(base64 < .github/scripts/deploy-ec2.sh | tr -d '\n')' | base64 -d | bash -s -- '$tag' \
       '$(base64 < observability/alloy/production.alloy | tr -d '\n')' \
       '$(base64 < infra/nginx/juken-map-go-routes.conf | tr -d '\n')' \
       '$(COPYFILE_DISABLE=1 tar --no-xattrs -C infra/systemd -cz . | base64 | tr -d '\n')'"
     aws ssm send-command --region ap-northeast-1 --instance-ids i-0eeb166295363e11d \
       --document-name AWS-RunShellScript --timeout-seconds 900 --comment "Restore juken-map $tag" \
       --parameters "$(jq -cn --arg c "$remote" '{commands: [$c]}')" --query Command.CommandId --output text
     ```

     結果は `aws ssm get-command-invocation --region ap-northeast-1 --instance-id i-0eeb166295363e11d --command-id <ID>` で見る。
     `/api/health` のコミットが信頼できるコミットになっていればよい。`COPYFILE_DISABLE=1` と `--no-xattrs` は、Mac の tar が
     拡張属性と `._` で始まるファイルを混ぜないようにするため（EC2 の tar が警告を出す）。戻したイメージは、次に CI がデプロイするまで動き続ける

   - EC2 そのものに手を入れられた疑い（知らないプロセス・cron・ユーザー）があれば、Terraform で EC2 を作り直す
6. 原因を取り除いてから戻す。ワークフローを直してマージし、ロールの拒否を外し、ワークフローを有効にする

   ```sh
   aws iam delete-role-policy --role-name github-actions-juken-map-ecr --policy-name incident-deny-all
   for wf in deploy.yml simulation.yml daily-study-notifications.yml; do gh workflow enable "$wf"; done
   ```

## 試した記録

本番を壊さずに試せるものは、手元の環境か、本番の読み取りだけで試した。
「未」は本番の設定を一時的に変えないと試せないもので、試したら日付を入れる。

| 操作 | 試した日 | どこで・どう試したか |
| --- | --- | --- |
| `pnpm incident` の6つの操作 | 2026-10-02 | 手元の DB に使い捨ての管理者（2段階認証つき・セッション2件）を作り、6つを順に実行して DB の変化を確かめた。`apps/api/src/services/incident-service.test.ts` が CI で毎回確かめる（2026-10-05 に Go へ移し、今は `apps/api/internal/feature/ops/incident_db_test.go`。JUK-122）（2026-10-04 に `revoke-all` を足し、表をログインの自作の表に替えた。JUK-115） |
| コンテナの中から `tsx src/incident.ts` を実行する | 2026-10-02 | 本番と同じ Dockerfile で作ったイメージを手元の DB に繋ぎ、`docker exec` と同じ形で実行した。2026-10-05 に本番の Node のコンテナが無くなり（JUK-109）、この形は使えなくなった |
| Go のコンテナの中から `/api-go incident`・`grant-admin` を実行する | 2026-10-05 | 本番と同じ Dockerfile（distroless）で作った Go のイメージを手元の DB に繋ぎ、`/api-go` を直接呼んだ。同日、本番の `juken-map-go` で `grant-admin --list` と、いない人への `incident sessions`（終了コード 1）を `docker exec` で実行した（JUK-122） |
| 運用コマンドの記録を残して `incident log` で引く | 2026-10-05 | 本番と同じ Dockerfile（distroless）で作った Go のイメージを手元のテスト用 DB に繋ぎ、使い捨ての利用者に `ban`・`unban` をしてから `log` を実行した。日時・操作・userId・コンテナ ID・前後の `bannedAt` が出た。`apps/api/internal/feature/ops/incident_db_test.go` が CI で毎回確かめる（JUK-138） |
| `grant-admin.ts --list` | 2026-10-02 | 手元の DB（2026-10-05 から Go の `grant-admin --list`） |
| Grafana の問い合わせ（メトリクス・ログ） | 2026-10-02 | 本番を読み取りだけ |
| アラートが鳴って受け口に届く | 2026-10-01・10-02 | 本番で実際に起こした（JUK-98）。H3 は Alloy を止めて、3本が約8〜20分で届いた |
| CloudTrail・SSM・ECR の記録を引く | 2026-10-02 | 本番を読み取りだけ |
| S3 に残した CloudTrail のログを引く | 2026-10-05 | 本番を読み取りだけ。5 の手順 3 のとおりにその日のフォルダを取り、jq で読めた（JUK-37） |
| GuardDuty の検出結果がメールに届く | 2026-10-05 | 本番で試験用の検出結果を1種類（`Recon:EC2/PortProbeUnprotectedPort`）作り、数秒で届いた（JUK-37） |
| CI のロールに付ける拒否のポリシー | 2026-10-02 | IAM のシミュレーションで、`ecr:PutImage`・`ssm:SendCommand` が `explicitDeny` になることを確かめた。実際に付けて外したのは下の行（2026-10-05） |
| Secrets Manager の1項目の差し替え | 2026-10-02（一部）・10-04 | jq の置き換えは偽の JSON で、`file:///dev/stdin` から値を渡せることは読み取りの API（`validate-resource-policy`）で確かめた。10-04 に本番で、上の手順のまま1項目を足した（JUK-112）。ほかの項目が残ることをキー名で、再デプロイで値が Go に渡ることを確かめた |
| `ALTER USER` で DB のパスワードを変える | 2026-10-02 | 手元の MySQL に `mysql:8.4` のコンテナから入り、使い捨てのユーザーで実行した。古い値は 1045 で断られ、新しい値で入れた |
| 新しい値を作る（`openssl rand`） | 2026-10-02 | 手元 |
| ワークフローを止めて戻す（`gh workflow disable / enable`） | 2026-10-05 | 本番で、5 の手順 1・6 のループのとおり3つを止め、`gh workflow list --all` で `disabled_manually` を確かめてから戻した（数秒） |
| CI のロールに拒否のポリシーを付けて外す | 2026-10-05 | 本番で 5 の手順 2 のとおりに付け、IAM のシミュレーションで `ecr:PutImage`・`ssm:SendCommand` が `explicitDeny` になるのを見てから、手順 6 のとおりに外した（約15秒） |
| EC2 の `.env` を直して再デプロイする | 2026-10-05 | 本番の `.env` にアプリが読まない行（`INCIDENT_DRILL`）を `send-command` で足し、`gh workflow run deploy.yml` のあと、新しいコンテナの `printenv` に出ることを確かめてから行を消した |
| 手元で amd64 のイメージを作って ECR に push し、手元から SSM でデプロイする | 2026-10-05 | 本番で、その時の `main`（`8d9d7fc`）を `restore-` のタグで作って push し、上の手順のまま SSM で送った。`/api/health` のコミットとブログ記事のページを確かめた。このとき、手順のビルドに `APP_COMMIT`・microCMS の鍵・`SSG_ARTICLES` が抜けていた（コミットが空・記事が SSG されないイメージになる）のを直した |
| 外部サービスの鍵の作り直し（Google・GitHub・LINE・Resend・microCMS・Grafana） | 未 | 各サービスの画面の操作。漏えいが無いのに作り直すと、差し替えまでの間アプリが止まるため |
