# 本番相当の負荷試験環境（JUK-46）

本番には負荷をかけない。代わりに、**本番 EC2 の AMI から複製した EC2** と **本番と同じ種類の空の RDS** を
一時的に建てて限界点を測り、終わったら全部壊す。手元の Mac ではアプリ・DB・k6 が同じ CPU を分け合い、
しかも本番より強いので、本番の実力は測れない。

- 本番の Terraform（`../`）とは **state を分けてある**。本番のリソース・ID・state は参照しない。
  コマンドは必ず `terraform -chdir=terraform/loadtest` で動かす
- 名前は `juken-map-loadtest-*`、タグ `Project=juken-map-loadtest` を全部に付ける
- 費用の目安は1時間 $0.2〜0.3。1回（準備2〜3時間＋試験2時間）で約250〜400円。
  **壊し忘れると1日約1,000円**（EC2 は12時間で自分を消すが、RDS は残る）

## 構成

```
VPC 10.50.0.0/16（本番の既定 VPC とは別）
├─ public  10.50.0.0/24   負荷をかける EC2（c7i.large）: k6・Node 24・pnpm・mysql client
│                          合成データの投入とセッションの発行もここから
└─ private 10.50.10.0/24  複製 EC2（t3.micro、本番の AMI）: nginx＋アプリ＋Alloy。インターネットへの経路なし
   private 10.50.11.0/24  RDS（db.t4g.micro・MySQL 8.4.9・gp2 20GB）
   SSM のエンドポイント（ssm / ssmmessages / ec2messages）
```

**複製 EC2 は、起動した瞬間に本番の設定（秘密情報を含む）のまま動き出す**
（コンテナが `--restart always` のため）。そこで、置き場所と SG で外へ出られなくしてある。
出口は試験用 RDS の 3306 と SSM エンドポイントの 443 だけなので、本番の DB・Resend・Grafana Cloud には届かない。
そのうえで `app-prepare.sh` が、アプリを試験用の DB と秘密で作り直す。

## 手順

✋ の所は、進める前にユーザーの確認を取る。`auto mode` では `apply -auto-approve` が使えないので、
`plan -out` してから `apply <ファイル>` にする。

### 1. 本番のコミットを調べ、AMI を作る ✋

```sh
repo_ref=$(gh run list --workflow deploy.yml --branch main --status success -L 1 --json headSha -q '.[0].headSha')
# ⚠️ 本番 EC2 から作る。--no-reboot はスクリプトの中で固定してある
ami=$(bash scripts/loadtest-aws/create-source-ami.sh | tail -1)

cat > terraform/loadtest/terraform.tfvars <<EOF   # gitignore 済み。destroy でも同じ値を使う
source_ami_id = "$ami"
repo_ref      = "$repo_ref"
EOF
```

### 2. 建てる ✋

```sh
terraform -chdir=terraform/loadtest init
terraform -chdir=terraform/loadtest plan -out=tfplan   # 試験用のリソースだけが並んでいることを確かめる
terraform -chdir=terraform/loadtest apply tfplan
```

負荷をかける EC2 のセットアップ（10分ほど）が終わったかどうかは、次で見る。

```sh
bash scripts/loadtest-aws/ssm-run.sh driver --cmd 'test -f /var/lib/juken-map-loadtest-ready && echo ready; tail -3 /var/log/juken-map-loadtest-setup.log'
```

### 3. データを入れる（負荷をかける EC2 から）

```sh
bash scripts/loadtest-aws/ssm-run.sh driver --cmd 'pnpm db:migrate && pnpm db:seed'
# 合成データは長く掛かるので裏で流し、ログを見る
bash scripts/loadtest-aws/ssm-run.sh driver --cmd 'nohup env USERS=42000 MONTHS=36 ALLOW_REMOTE=on pnpm db:seed:synthetic > ~/seed.log 2>&1 &'
bash scripts/loadtest-aws/ssm-run.sh driver --cmd 'tail -5 ~/seed.log'
```

⚠️ gp2 20GB は、投入で IO の残り（`BurstBalance`）を使い切る見込み。回復には何時間もかかるので待たず、
各段階の `BurstBalance` を記録して解釈に添える（本番も同じ gp2 20GB）。

### 4. 複製 EC2 を試験用の設定で動かす

```sh
bash scripts/loadtest-aws/ssm-run.sh app --script scripts/loadtest-aws/app-prepare.sh --init-disk
bash scripts/loadtest-aws/ssm-run.sh app --script scripts/loadtest-aws/app-sampler.sh start
```

`app-prepare.sh` は次を確かめてから動く。どれかが違えば何もせず止まる。

- 試験用の VPC（10.50.0.0/16）の中にいる
- インターネットへ出られない

### 5. 測る

`BASE_URL` は `terraform -chdir=terraform/loadtest output -raw base_url`（`https://10.50.10.x`）。
長く掛かるので裏で流し、ログを見る。結果は段階ごとに S3 へ上がる。

```sh
base_url=$(terraform -chdir=terraform/loadtest output -raw base_url)
run() { bash scripts/loadtest-aws/ssm-run.sh driver --cmd "nohup env BASE_URL=$base_url $1 bash scripts/run-loadtest-limit.sh > ~/run.log 2>&1 &"; }

run 'RATES="50 100" DURATION=5m CONTINUE_AFTER_BREAK=on'   # 通常時の様子
run 'RATES="150 200 250 300 350 400"'                      # 限界点（割った段階で止まる）
run 'RATES="<限界点の前後>" CONTINUE_AFTER_BREAK=on'          # 3回ずつ
run 'SCENARIO=spike'                                        # 10人→100人
bash scripts/loadtest-aws/ssm-run.sh driver --cmd 'tail -20 ~/run.log'
```

JUK-7（混雑時に断る上限）の比較は、上限を変えてアプリを作り直してから同じ段階を流す。

```sh
bash scripts/loadtest-aws/ssm-run.sh app --script scripts/loadtest-aws/app-prepare.sh --overload-max 40
run 'OVERLOAD_LABEL=40 RATES="<限界点の前後>" CONTINUE_AFTER_BREAK=on'
```

### 6. 結果を手元へ落とし、EC2・RDS の様子を足す

```sh
bucket=$(terraform -chdir=terraform/loadtest output -raw results_bucket)
aws s3 cp "s3://$bucket/results/" load-tests/results/aws/ --recursive
bash scripts/loadtest-aws/collect-metrics.sh load-tests/results/aws/limit-XXXX.jsonl
```

数字は auto-memory の `quality-current-state` / `load-testing` に残す（リポジトリには書かない）。

### 7. 壊す ✋

```sh
terraform -chdir=terraform/loadtest plan -destroy -out=tfplan-destroy
terraform -chdir=terraform/loadtest apply tfplan-destroy
bash scripts/loadtest-aws/leftover-check.sh --delete-images   # AMI とスナップショットも消す。0件で終わる
```

## 本番との違い（結果を読むときに添える）

- RDS は自動バックアップを取らない（本番は1日保持）。中身は合成データ（本番は実データ）
- 複製 EC2 の CPU クレジットは0から始まる（Unlimited なので性能は落ちず、超過分に料金がかかる）
- Alloy は動いているが、外へ出られないので送信は失敗し続ける（本番は Grafana Cloud へ送れている）
- 定期処理（certbot・apt など）は止めてある
- k6 は IP で繋ぐので、証明書の検証を外している（暗号化の重さは同じ）
