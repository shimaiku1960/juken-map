# Grafana Cloud のアラートと通知先（JUK-83）

Grafana Cloud（`https://kindcrest3516.grafana.net`）のアラートと通知先を、画面での手作業ではなく
Terraform で管理する。変えたいときは `.tf` を直し、PR で差分を見てから `apply` する。

| 置いてあるもの | ファイル |
| -- | -- |
| フォルダ `juken-map`（uid `ffh5ls`） | `alerting.tf` |
| 通知先 `juken-map-email` | `alerting.tf` |
| ルールグループ `api-production`（1分ごと）：5xx率、Go の停止 | `alerting.tf` |
| 画面で作ってあったものの取り込み | `imports.tf` |

- 本番 AWS（`../`）とは **state を分けてある**。コマンドは必ず `terraform -chdir=terraform/grafana` で動かす
- **Terraform で管理しているアラートは、画面では編集しない**（provenance が付いて、画面からは編集できなくなっている）。
  通知先だけは付けられない事情があって画面からも編集できるが、変えるときはやはりこのファイルを直す（理由は `alerting.tf` のコメント）
- 次のものは持っていない
  - 通知ポリシー（ルートの受け手が `juken-map-email` のまま。ルールは `notification_settings` で通知先を直接指定している）
  - Synthetic Monitoring のチェック `juken-map health` と、SM のアプリが作ったアラート2本。
    チェックを Terraform に載せるには、サービスアカウントのトークンとは別に SM のアクセストークンが要る

## 最初に1回だけ

1. Grafana の Administration → Service accounts で、アラートを書き換えられるサービスアカウント（今は `sa-1-terraform`）の
   トークンを作る
2. 手元の環境変数に置く。どちらもリポジトリには入れない

   ```sh
   export TF_VAR_grafana_auth='glsa_...'
   export TF_VAR_alert_email_addresses='you@example.com'   # 複数ならセミコロン区切り
   ```

## 変えるとき

state（`terraform.tfstate`）は **本体のチェックアウト**（`juken-map/terraform/grafana/`）にだけ置く（gitignore 済み）。
本体で動かすなら、そのまま次を実行する。

```sh
terraform -chdir=terraform/grafana init
terraform -chdir=terraform/grafana plan -out=tfplan   # 想定したルールだけが変わることを確かめる
terraform -chdir=terraform/grafana apply tfplan
```

worktree から動かすときは、`plan` と `apply` の両方に `-state=<本体>/terraform/grafana/terraform.tfstate` を付ける
（worktree を消すと state も消えるため）。

## 通知が届くか確かめる

ルールの通知を試すには、Alerting → Contact points → `juken-map-email` の **Test** を押す。
ルールの条件（Go の停止など）を確かめるには、Alerting → Alert rules でルールを開き、クエリの結果が
想定どおり（Go が動いていれば `1`）かを見る。
