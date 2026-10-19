# 運用ガイド

本番（https://juken-map.com）へのデプロイと、本番での監視・管理の手順です。障害や乗っ取りが起きたときの手順は[起きたときの手順](incident-response.md)、記録の書き方は[止まった・遅いときの記録と振り返り](outage-response.md)にあります。

## デプロイ

`main` ブランチへのpushを起点に、GitHub Actionsが次の順序で本番へデプロイします。

1. Dockerイメージをビルドします。
2. AWS OIDCで一時的な認証情報を取得します。
3. DockerイメージをAmazon ECRへpushします。
4. SSM Run CommandでEC2上のデプロイスクリプトを実行します。
5. EC2がIAMインスタンスロールでECRからイメージをpullします。
6. マイグレーションとコンテナの入れ替えを実行します。
7. スモークテストに失敗した場合は直前のイメージへ戻します。

デプロイジョブには `concurrency` を設定し、複数のデプロイが同時に本番環境を変更しないようにしています。

## 本番で管理者を付ける

管理者ページ（`/admin`）の権限は`user.role`列で決まり、画面からは付けられません。
本番のRDSには外から繋げないので、SSMでEC2に入り、動いているGoのコンテナの中で実行します。

```bash
aws ssm start-session --target <インスタンスID>
sudo docker exec juken-map-go /api grant-admin <メールアドレス>   # 戻すときは --revoke
```

付け替えは、そのユーザーが次にログインし直したときに画面へ反映されます
（APIの判定はセッションを読むたびなので、すぐ効きます）。今の管理者は `--list` で確かめます。

## 管理者の2段階認証

管理APIは、2段階認証（認証アプリのコードか予備コード）を通して作られたセッションでだけ開けます。
2段階認証はメールとパスワードのログインにかかり、Google・GitHubでのログインでは管理画面は開けません。

1. パスワードが無い（Google・GitHubだけで登録した）ときは、ログアウトしてログイン画面の「パスワードを忘れた方」から設定する
2. メールとパスワードでログインし、`/admin`で2段階認証を設定する（QRを認証アプリで読み取り、予備コードを保存し、6桁のコードを確かめる）
3. 次からは、パスワードのあとに認証アプリのコードを入力してログインする

認証アプリも予備コードも無くしたときは、本番の Go のコンテナで `incident reset-2fa <メール>` を実行して設定前に戻し、
設定し直します（利用者本人の確認ができたときだけ行う。手順は [docs/incident-response.md](incident-response.md)）。

## 本番の可観測性（Grafana Cloud）

本番のEC2は`t3.micro`（メモリ1GB）なので、Prometheus・Loki・Tempoの本体は載せず、収集役の
[Grafana Alloy](https://grafana.com/docs/alloy/)だけをアプリの隣で動かし、保存と画面はGrafana Cloudの
無料枠に任せます。設定は[observability/alloy/production.alloy](../observability/alloy/production.alloy)で、
デプロイのたびにこのファイルがEC2へ運ばれます（`.github/scripts/deploy-ec2.sh`）。

| 信号 | 経路 |
| --- | --- |
| Logs | アプリの標準出力 → Dockerのログ → Alloy（`loki.source.docker`）→ Loki |
| Metrics | アプリの`METRICS_PORT`（9464番）→ Alloyが30秒ごとに読む → Prometheus |
| Traces | アプリのOTLP送信 → Alloy（4318番）→ Tempo |

ログはDockerのログをそのまま読むので、`awslogs`ドライバへ替える場合と違い、SSMで入ったときの
`docker logs juken-map`は今まで通り使えます。`/metrics`とAlloyの画面（12345番）はDockerネットワークの
中だけに開くので、nginx越しには届きません。

Grafana Cloudを含め、外部のサービスへ何を送っているかは[docs/third-party-data.md](third-party-data.md)に
送信先ごとにまとめています。送るものを足すときは、先にこの表を書き換えます。

接続情報はSecrets Managerの`juken-map/production/runtime`に次のキーで入れます。
**このキーが入っていない間は、Alloyもアプリの送信も起動しません**（本番は今まで通り動きます）。

| キー | 値 |
| --- | --- |
| `GRAFANA_CLOUD_TOKEN` | Cloud Access Policyのトークン（3つで共通） |
| `GRAFANA_CLOUD_LOKI_URL` / `GRAFANA_CLOUD_LOKI_USER` | Lokiの送信先URLとユーザーID |
| `GRAFANA_CLOUD_PROM_URL` / `GRAFANA_CLOUD_PROM_USER` | Prometheusのremote write先URLとユーザーID |
| `GRAFANA_CLOUD_OTLP_URL` / `GRAFANA_CLOUD_OTLP_USER` | OTLPの送信先URLとインスタンスID |

本番のアラート（5xxの割合、Goの停止）と通知先は、Grafanaの画面ではなく[terraform/grafana/](../terraform/grafana/)で
管理しています。変えるときはそこを直して`terraform plan`で差分を見てから当てます。
