# インフラ基礎の学習カリキュラム（Linux・ネットワーク・Bash）

受験マップ（juken-map.com）の本番構成を教材にして、Linux・ネットワーク・Bash の基礎を手を動かして学ぶ。
目標は次の2つ（JUK-182、2026-10-12 作成）。

- LPIC-1（101・102）レベルの知識
- 北米の DevOps / SRE の面接で、自分の構成を英語で説明し、「なぜそうしたか」に答えられること

全16回（8週間・週2回・1回30〜60分）。進捗は [infra-basics-progress.md](./infra-basics-progress.md) に追記する。
バックエンドの学習計画（[README.md](./README.md)）とは別系統。

---

## 1. 教材にする本番構成

```
ブラウザ
  │  DNS: juken-map.com → CloudFront（Cloudflare は DNS だけ。プロキシは通さない）
  ▼  TCP 443（1回目の TLS 終端。証明書は ACM・RSA）
CloudFront（キャッシュは /assets/* だけ。Host を含む利用者のヘッダーをそのまま渡す）
  │  DNS: origin.juken-map.com → EC2 のパブリック IP
  ▼  TCP 443（2回目の TLS 終端。証明書は Let's Encrypt、certbot.timer が webroot で更新）
     セキュリティグループは 443 を CloudFront の IP（origin-facing プレフィックスリスト）だけに許可。80 は閉じている
┌─ EC2 t3.micro（Ubuntu・1台・デフォルト VPC）──────────────────────────┐
│ nginx（ホストの systemd サービス。Docker の外）                          │
│   │ http://127.0.0.1:8080 か 8081（デプロイのたびに入れ替わる）          │
│   ▼                                                                      │
│ Docker ネットワーク juken-map                                            │
│   ├ juken-map-go   … Go（API・ログイン・SPA）。distroless・nonroot・128MB │
│   └ juken-map-alloy … ログ・メトリクスを Grafana Cloud へ送る            │
│ systemd timer … 毎日の通知（7:00 / 21:00 JST）、certbot、logrotate など │
└──────────┬───────────────────────────────────────────────────────────┘
           │ TCP 3306（セキュリティグループで EC2 からだけ許可）
           ▼
     RDS MySQL 8.4（db.t4g.micro・Single-AZ・publicly_accessible = false）
```

| 仕組み | 教材にするファイル |
| -- | -- |
| ネットワーク・サーバー・DB | `terraform/vpc.tf`・`sg.tf`・`ec2.tf`・`rds.tf`・`iam_ec2.tf`・`secrets.tf`、比較用に `terraform/loadtest/network.tf`（自前の VPC） |
| CloudFront・DNS | `terraform/cloudfront.tf`・`dns.tf`、`infra/nginx/README.md` の「CloudFront を前に置く」（JUK-185） |
| デプロイ | `.github/workflows/deploy.yml` → SSM Run Command → `.github/scripts/deploy-ec2.sh` |
| nginx | `infra/nginx/README.md`・`conf.d/*.conf`（`juken-map-cloudfront-realip.conf` を含む）・`juken-map-go-routes.conf` |
| systemd | `infra/systemd/*.timer`・`*.service`・`README.md` |
| コンテナ | `apps/api/Dockerfile`、`deploy-ec2.sh` の `docker run` |
| ログ・メトリクス | `apps/api/internal/telemetry/logger.go`（JSON ログ）、`observability/alloy/production.alloy` |
| SLO・障害対応 | `docs/slo.md`・`docs/incident-response.md` |
| 手元で試す道具 | `scripts/rehearse-zero-downtime.sh`・`test-deploy-ports.sh`・`test-deploy-timers.sh`・`local-proxy.sh` |

---

## 2. 進め方

### 毎回の流れ

1. **今日の目的**（2〜3行）
2. **予想**：コマンドを打つ前に、出力がどうなるかを自分の言葉で書く
3. **実行**：コマンドは自分で打つ（Claude Code では先頭に `!`）
4. **解説**：出力の読み方、「なぜこう設計されているか」「ほかの選択肢と比べて何が良いか」
5. **確認クイズ** 2〜3問（答えはすぐ言わない）
6. **記録**：[infra-basics-progress.md](./infra-basics-progress.md) に「やったこと・理解したこと・つまずいた点・次回やること」

用語は日本語と英語を併記する（例：権限 / permission）。各回の「面接の問い」は英語で答える練習をする。

### 実行する場所（環境）

| 記号 | 場所 | 使い方 |
| -- | -- | -- |
| 🖥 Mac | 手元の Mac | `! dig ...` のように Claude Code から打てる |
| 🧪 箱 | 使い捨ての Ubuntu コンテナ | `docker run --rm -it --name infra-lab -h infra-lab ubuntu:24.04 bash`。終われば消える。壊してよい |
| 🧪 VM | 使い捨ての Ubuntu VM（systemd が要る回） | Lima（`brew install lima` → `limactl start --name=lab template://ubuntu-24.04`）。S6 の前に用意する。使い終われば `limactl delete lab` |
| 🏠 手元の本番相当 | `pnpm dev`・`scripts/local-proxy.sh`・`scripts/rehearse-zero-downtime.sh` | 本番と同じ形の nginx・コンテナを手元で動かす |
| ☁️ 本番 | EC2（読み取りだけ） | 下の「本番への入り方」 |

**本番への入り方**：`aws ssm start-session --target i-0eeb166295363e11d --region ap-northeast-1`。
対話のセッションは `!` では使えないので、**別のターミナルで入り、出力を貼る**。入った直後のユーザーは `ssm-user`。

---

## 3. 安全ルール

### 本番でしてよいこと

読み取りだけ。`ls`・`stat`・`ps`・`ss`・`df`・`systemctl status / cat / list-*`・`journalctl`・`docker ps / logs / stats`・
`nginx -t`（設定の検査だけ）・`curl`（GET）・`aws ... describe-*` / `list-*`。

### 本番でしないこと（提示だけして、明示の許可を待つ）

- 設定を変える・再起動・停止・削除：`systemctl restart / reload / stop / start / enable`、`docker stop / rm / run / exec`、`nginx -s`、ファイルの編集
- `terraform apply`（**絶対にしない**）。`terraform plan` までにする
- カオス実験（`internal/fault`・`internal/hostfault`、`juken-map-chaos-*` のタイマー）と負荷試験（`scripts/run-loadtest-*`、`terraform/loadtest/`）は**このカリキュラムでは動かさない**。ファイルを読むのはよい
- `/api incident` などの運用コマンド

### シークレットが画面に出るコマンド（使わない・形を変えて使う）

| 出てしまうもの | 代わりにこう打つ |
| -- | -- |
| `docker inspect juken-map-go`（`Config.Env` に環境変数がすべて出る） | `docker inspect -f '{{.State.Pid}}' juken-map-go` のように `-f` で項目を絞る |
| `/proc/<PID>/environ`、`ps e`・`ps auxe` | 読まない。`ps -o pid,user,cmd` |
| `/etc/juken-map/*.env`・`/home/ubuntu/juken-map/` の `.env` | `ls -l`・`stat` で権限と持ち主だけ見る。`cat` しない |
| `aws secretsmanager get-secret-value` | `aws secretsmanager describe-secret`（値は出ない） |
| `terraform show`・`terraform state show`（state に値が入りうる） | `terraform state list`（名前だけ）と `terraform plan` |
| インスタンスメタデータの `iam/security-credentials/` | 読まない。`local-ipv4` など場所の情報だけ |
| 本番での `tcpdump -A` / `-X`（127.0.0.1:8080 の中は平文で、Cookie が流れる） | 中身を見る練習は 🖥 Mac か 🏠 手元で。本番は許可を取り、ヘッダーだけ |

アクセスログの IP・URL は個人の情報なので、集計した数だけを貼り、生の行は会話や公開の場に貼らない。

---

## 4. 全体の予定

| 週 | 回 | テーマ | 分野 |
| -- | -- | -- | -- |
| 1 | S1 | 全体像と入り方（SSH と SSM） | Linux・ネットワーク |
| 1 | S2 | ユーザー・グループ・パーミッション | Linux |
| 2 | S3 | 特権の境界（docker グループ・nonroot・DynamicUser・SUID） | Linux |
| 2 | S4 | プロセスとしてのコンテナ（/proc・namespace・cgroup・メモリ） | Linux |
| 3 | S5 | シグナルと無停止デプロイ | Linux・Bash |
| 3 | S6 | systemd の基本と nginx（unit・target・reload と restart・パッケージ） | Linux |
| 4 | S7 | 自作の timer と cron の比較・時刻 | Linux・Bash |
| 4 | S8 | ログ①：journald・nginx のログ・logrotate・ディスク | Linux |
| 5 | S9 | Bash①：アクセスログの集計（パイプ・awk・sort・uniq・正規表現） | Bash |
| 5 | S10 | ログ②：docker logs と JSON ログを jq で読む | Linux・Bash |
| 6 | S11 | DNS：juken-map.com から RDS のエンドポイントまで | ネットワーク |
| 6 | S12 | TCP と TLS：curl -v・openssl s_client・tcpdump | ネットワーク |
| 7 | S13 | nginx の 443 → 127.0.0.1:8080/8081 と ss | ネットワーク |
| 7 | S14 | VPC・サブネット・CIDR・セキュリティグループ・EC2 → RDS | ネットワーク |
| 8 | S15 | Bash②：ヘルスチェックのスクリプトを自分で書く | Bash |
| 8 | S16 | Bash③：デプロイの切り替え処理を読み解く ＋ 構成を英語で説明する | Bash・総まとめ |

---

## 5. 各回の内容

各回の「考える問い」の答えはここに書かない。セッションの中で自分で考えてから解説を聞く。

### 第1週

#### S1. 全体像と入り方（SSH と SSM）〔45分〕

- **目的**：本番の箱の数と経路を自分で描けるようにする。サーバーへの入り方が SSH ではなく SSM である理由を言えるようにする
- **教材**：上の構成図、`terraform/ec2.tf`・`sg.tf`・`iam_ec2.tf`（`AmazonSSMManagedInstanceCore`）、`docs/incident-response.md` の「共通」
- **環境**：🖥 Mac → ☁️ 本番
- **コマンド**

  ```sh
  # 🖥 自分が誰として AWS を触っているか
  aws sts get-caller-identity --query Arn --output text
  # 🖥 EC2 の状態・型・IP・付いているセキュリティグループ
  aws ec2 describe-instances --instance-ids i-0eeb166295363e11d --region ap-northeast-1 \
    --query 'Reservations[0].Instances[0].{State:State.Name,Type:InstanceType,PrivateIp:PrivateIpAddress,PublicIp:PublicIpAddress,SG:SecurityGroups[].GroupName}'
  # ☁️ 別のターミナルで入る
  aws ssm start-session --target i-0eeb166295363e11d --region ap-northeast-1
  # ☁️ 入ったら
  whoami; id; hostname; uptime
  cat /etc/os-release | head -3; uname -r
  nproc; free -m; df -h /
  # 🧪 箱で SSH の鍵を作ってみる（本番の鍵には触らない）
  apt-get update -qq && apt-get install -y -qq openssh-client
  ssh-keygen -t ed25519 -f ~/.ssh/lab -N ''; ls -l ~/.ssh
  ```

- **考える問い**
  - `sg.tf` の `launch-wizard-1` は 22番を `0.0.0.0/0` に開けている。では本番に SSH で入れるか？ `describe-instances` の SG の欄と見比べる
  - SSM で入ると「22番を開けなくてよい」以外に何が良いか（認証・記録の観点）
  - `~/.ssh` と秘密鍵のパーミッションはなぜあの値なのか
- **LPIC-1**：102.6（仮想マシンのゲストとしての Linux）、110.3（SSH の鍵）
- **面接の問い**：*Walk me through your production architecture.* / *How do you get a shell on your server, and why not SSH?*

#### S2. ユーザー・グループ・パーミッション〔60分〕

- **目的**：`rwx`・8進数・持ち主とグループを読めるようにし、本番の「秘密のファイルを root だけが読める」設計を確かめる
- **教材**：`deploy-ec2.sh` の `chmod 600`、`infra/systemd/juken-map-daily-notification@.service` の `EnvironmentFile`
- **環境**：🧪 箱 → ☁️ 本番
- **コマンド**

  ```sh
  # 🧪 箱で、ユーザーとグループを作って壊してみる
  useradd -m alice; useradd -m bob; groupadd deploy; usermod -aG deploy alice
  id alice; getent group deploy; tail -3 /etc/passwd; tail -3 /etc/group
  su - alice -c 'umask; echo secret > ~/s.txt; ls -l ~/s.txt'
  chmod 600 /home/alice/s.txt; su - bob -c 'cat /home/alice/s.txt'   # 失敗するはず
  chmod 640 /home/alice/s.txt; chgrp deploy /home/alice/s.txt; ls -l /home/alice/s.txt
  # vi に慣れる（:wq・:q!・/検索・dd）
  apt-get install -y -qq vim-tiny && vi /tmp/memo.txt

  # ☁️ 本番では見るだけ（中身は cat しない）
  getent passwd ubuntu ssm-user root
  id ssm-user; sudo -l
  sudo ls -l /etc/juken-map/
  sudo stat -c '%a %U:%G %n' /etc/juken-map/*
  ls -ld /home/ubuntu /home/ubuntu/juken-map
  ```

- **考える問い**
  - `ssm-user` が `sudo` で何でもできるのは、どのファイルのどの設定によるか
  - 通知の共有トークンのファイルが 600・root なのに、`DynamicUser` の一時ユーザーで動くサービスがその値を使えるのはなぜか
  - `umask 022` と `077` で、新しく作ったファイルの権限はどう変わるか
- **LPIC-1**：104.5（パーミッションと持ち主）、107.1（ユーザーとグループ）、110.1（sudo）、103.8（vi）
- **面接の問い**：*How do you store secrets on the host, and who can read them?*

### 第2週

#### S3. 特権の境界（docker グループ・nonroot・DynamicUser・SUID）〔45分〕

- **目的**：「どこで root の力を持てるか」を一覧にし、コンテナを nonroot で動かす意味を説明できるようにする
- **教材**：`apps/api/Dockerfile`（`distroless/static-debian12:nonroot`・`USER nonroot`）、`@.service` の `ProtectSystem`・`NoNewPrivileges` など
- **環境**：🧪 箱 → ☁️ 本番
- **コマンド**

  ```sh
  # 🧪 SUID・SGID・スティッキービット
  ls -l /usr/bin/passwd /usr/bin/su; ls -ld /tmp
  find / -perm -4000 -type f 2>/dev/null

  # ☁️ 本番
  getent group docker
  ls -l /var/run/docker.sock
  sudo docker inspect -f '{{.Config.User}}' juken-map-go     # -f を必ず付ける（安全ルール）
  sudo docker inspect -f '{{.HostConfig.Memory}} {{.HostConfig.RestartPolicy.Name}}' juken-map-go
  systemd-analyze security juken-map-daily-notification@morning.service | tail -5
  ```

- **考える問い**
  - 「docker グループに入れる ＝ root を渡す」と言われるのはなぜか
  - distroless でシェルが無いことは、攻撃者にとって何が困るか。運用者にとって何が困るか（`docs/incident-response.md` の `/api incident` の呼び方を見る）
  - `systemd-analyze security` の点数を上げるとしたら、次に何を足すか
- **LPIC-1**：104.5（SUID・SGID・sticky）、110.1（SUID のファイルを探す）、110.2（ホストのセキュリティ）
- **面接の問い**：*Why do you run your containers as non-root, and what else limits the blast radius?*

#### S4. プロセスとしてのコンテナ〔60分〕

- **目的**：コンテナは「隔離された普通のプロセス」だと、ホストの `ps` と `/proc` で確かめる
- **教材**：`deploy-ec2.sh` の `docker run`（`--memory 128m`・`-p 127.0.0.1:...`）と `GOMEMLIMIT` のコメント
- **環境**：☁️ 本番（🧪 箱で `ps` に慣れてから）
- **コマンド**

  ```sh
  # ☁️ ホストから見たプロセスの木
  ps -eo pid,ppid,user,rss,etime,cmd --forest | grep -E 'containerd|docker|/api|alloy|nginx' | grep -v grep
  PID=$(sudo docker inspect -f '{{.State.Pid}}' juken-map-go); echo $PID
  ps -o pid,ppid,user,rss,cmd -p $PID
  sudo grep -E '^(Name|Uid|VmRSS|Threads)' /proc/$PID/status     # environ は読まない
  sudo ls -l /proc/$PID/ns
  sudo ls -l /proc/1/ns | head -5                                 # ホストの PID 1 と見比べる
  cat /proc/$PID/cgroup
  sudo docker stats --no-stream
  free -m; top -b -n 1 | head -15
  sudo journalctl -k --since -7d | grep -i -E 'oom|killed process' | tail -5
  ```

- **考える問い**
  - ホストの `ps` で見える `/api` の UID が 65532 になっているのはなぜか
  - `/proc/$PID/ns` の番号がホストと違うもの・同じものは、それぞれ何を意味するか
  - 128MB を超えたら何が起きるか。そのとき `--restart always` は何をするか
- **LPIC-1**：103.5（プロセスの作成・監視・終了）、103.6（優先度。`nice`・`renice` は 🧪 箱で試す）、102.6
- **面接の問い**：*What actually is a container from the kernel's point of view?*

### 第3週

#### S5. シグナルと無停止デプロイ〔60分〕

- **目的**：SIGTERM・SIGKILL・猶予時間の関係を、手元のリハーサルで 502 が出る・出ない形で体験する
- **教材**：`scripts/rehearse-zero-downtime.sh`、`deploy-ec2.sh` の「無停止デプロイ」の節、`infra/nginx/README.md` の「無停止デプロイ」「落とし穴」
- **環境**：🧪 箱 → 🏠 手元
- **コマンド**

  ```sh
  # 🧪 シグナルの基本
  kill -l | head -3
  sleep 300 & echo $!; jobs; kill -TERM %1; jobs
  bash -c 'trap "echo got TERM; exit 0" TERM; while :; do sleep 1; done' & sleep 1; kill -TERM $!
  nice -n 10 sleep 100 & ps -o pid,ni,cmd -p $!; renice 5 -p $!

  # 🏠 本番と同じ切り替え方を手元で再現する（Docker だけを使う。本番には触らない）
  bash scripts/rehearse-zero-downtime.sh 2>&1 | tail -30
  bash scripts/test-deploy-ports.sh 2>&1 | tail -15
  ```

- **考える問い**
  - 古いコンテナを「stop → rm → run」していた頃に 502 が出たのはなぜか。今はなぜ出ないか
  - `nginx reload` の直後に古いコンテナを止めると、まだ 502 が出うるのはなぜか（README の落とし穴）
  - `docker stop` の既定の猶予（10秒）と Go の `Shutdown` の上限が同じだと何が困るか
- **LPIC-1**：103.5（シグナル・`kill`・`jobs`）、103.6（`nice`・`renice`）
- **面接の問い**：*How do you deploy without downtime on a single host?* / *Blue-green vs rolling — which one is yours?*

#### S6. systemd の基本と nginx〔60分〕

- **目的**：unit・target・依存関係を読み、nginx の `reload` と `restart` の違いをプロセスの入れ替わりで説明できるようにする
- **教材**：`infra/nginx/README.md`（構成・`sites-enabled` のリンク）、`deploy-ec2.sh` の `nginx -t` → `systemctl reload nginx`
- **環境**：☁️ 本番（読むだけ）→ 🧪 VM（reload・restart を自分で打つ）
- **コマンド**

  ```sh
  # ☁️ 本番
  systemctl status nginx --no-pager | head -12
  systemctl cat nginx | grep -E '^\[|Exec|Type|PIDFile'
  systemctl is-enabled nginx docker certbot.timer
  systemctl get-default; systemctl list-dependencies multi-user.target --no-pager | head -20
  systemd-analyze blame | head -10
  ps -o pid,ppid,user,etime,cmd -C nginx
  ls -l /etc/nginx/sites-enabled/
  sudo nginx -t
  dpkg -l nginx | tail -1; apt-cache policy nginx | head -3

  # 🧪 VM で：reload と restart を見比べる（本番ではしない）
  sudo apt-get install -y nginx
  ps -o pid,ppid,etime,cmd -C nginx; sudo systemctl reload nginx; ps -o pid,ppid,etime,cmd -C nginx
  sudo systemctl restart nginx; ps -o pid,ppid,etime,cmd -C nginx
  ```

- **考える問い**
  - reload の前後で、master と worker の PID はそれぞれどうなるか
  - `nginx -t` を挟まずに reload すると、設定が壊れていたら何が起きるか
  - EC2 が再起動したとき、nginx と Docker のコンテナはそれぞれ何によって起き上がるか
- **LPIC-1**：101.3（target・起動と停止）、101.2（起動の流れ）、102.4（Debian のパッケージ）、104.6（シンボリックリンク）
- **面接の問い**：*What's the difference between reload and restart, and why does it matter for availability?*

### 第4週

#### S7. 自作の timer と cron の比較・時刻〔60分〕

- **目的**：毎日の通知のタイマーを読み解き、cron と systemd timer と GitHub Actions の schedule を比べられるようにする
- **教材**：`infra/systemd/` の3ファイルと `README.md`、`.github/workflows/daily-study-notifications.yml`、`scripts/test-deploy-timers.sh`
- **環境**：☁️ 本番（読むだけ）→ 🧪 VM・🧪 箱
- **コマンド**

  ```sh
  # ☁️ 本番
  systemctl list-timers --all --no-pager | head -15      # juken-map-chaos-* も見えるが、触らない
  systemctl cat juken-map-daily-notification-morning.timer
  systemctl cat juken-map-daily-notification@.service | grep -vE '^#'
  systemd-analyze calendar '*-*-* 07:00:00 Asia/Tokyo'
  timedatectl
  sudo crontab -l 2>&1; ls /etc/cron.d/ /etc/cron.daily/

  # 🧪 VM：自分のタイマーを作る（1分ごとに date を書く）
  #   /etc/systemd/system/hello.service と hello.timer を書き、daemon-reload → enable --now
  #   journalctl -u hello -f で動きを見て、最後に disable --now して消す
  # 🧪 箱：同じことを cron で
  apt-get install -y -qq cron && service cron start
  echo '* * * * * date >> /tmp/cron.log' | crontab -; sleep 70; cat /tmp/cron.log
  ```

- **考える問い**
  - GitHub Actions の schedule から EC2 の timer に移したのはなぜか（2〜4時間の遅れ）。代わりに何を失ったか
  - `Persistent=true` を付けなかった理由。付けたら EC2 が止まっていた日の夜に何が起きるか
  - `@.service` の `%i` と、`$$`・`%%` のエスケープはそれぞれ誰（systemd・sh）のための書き方か
  - cron と比べて timer が勝る点を3つ（ログ・依存関係・権限の絞り方）
- **LPIC-1**：107.2（cron・at・systemd timer）、108.1（システム時刻）、107.3（タイムゾーン）
- **面接の問い**：*Cron vs systemd timers — when would you choose each?*

#### S8. ログ①：journald・nginx のログ・logrotate・ディスク〔45分〕

- **目的**：ログがどこに出て、どれだけ溜まり、いつ消えるかを説明できるようにする
- **教材**：`deploy-ec2.sh` の「nginx のログの保存期間（JUK-128）」、`scripts/test-deploy-logrotate.sh`
- **環境**：☁️ 本番
- **コマンド**

  ```sh
  journalctl -u nginx --since today --no-pager | tail -5
  journalctl -u 'juken-map-daily-notification@*' -n 10 --no-pager
  journalctl -p warning -b --no-pager | tail -10
  journalctl --disk-usage
  ls -lh /var/log/nginx/
  cat /etc/logrotate.d/nginx
  df -h /; sudo du -sh /var/log /var/lib/docker 2>/dev/null
  findmnt /; lsblk
  ```

- **考える問い**
  - アクセスログを14日で消す理由（`dev-standards` 06 G3）。運用上の調べ物とどう両立させるか
  - `notifempty` のままだと、ほぼ空のエラーログがなぜ1か月以上残ったか
  - ディスク 30GB が埋まるとしたら、どこからか。コンテナのログ（json-file）は上限があるか
- **LPIC-1**：108.2（システムのログ・logrotate）、104.7（FHS・`/var/log` と `/var/lib`）、104.1・104.3（`lsblk`・`findmnt` の読み方）
- **面接の問い**：*Where do your logs go, and how long do you keep them? Why?*

### 第5週

#### S9. Bash①：アクセスログの集計〔60分〕

- **目的**：パイプ・`awk`・`sort`・`uniq`・`grep -E` で、「今日のステータス別件数」「遅いパス」などを1行で出せるようにする
- **教材**：nginx の `combined` 形式（`$remote_addr - - [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`）
- **環境**：🏠 手元で練習用のログを作る → ☁️ 本番では集計結果だけを見る
- **コマンド**

  ```sh
  # 🏠 手元：local-proxy の nginx にリクエストを流してログを作る（ポートは scripts/local-ports.sh の出力に合わせる）
  for p in / /api/health /login /nope; do curl -s -o /dev/null "http://localhost:<PROXY_PORT>$p"; done

  # ☁️ 本番：結果は数だけ（生の行は貼らない）
  sudo awk '{print $9}' /var/log/nginx/access.log | sort | uniq -c | sort -rn
  sudo awk '$9 ~ /^5/' /var/log/nginx/access.log | wc -l
  sudo awk '{print $7}' /var/log/nginx/access.log | sed 's/?.*//' | sort | uniq -c | sort -rn | head -10
  sudo awk '{print substr($4, 14, 5)}' /var/log/nginx/access.log | uniq -c | sort -rn | head -5   # 分ごとの件数
  sudo zcat -f /var/log/nginx/access.log.*.gz | wc -l
  sudo grep -cE '"(GET|POST) /api/health ' /var/log/nginx/access.log
  ```

- **考える問い**
  - `uniq -c` の前に `sort` が要るのはなぜか
  - `/api/health` が件数の大半を占めると、SLO の遅延の見え方にどう効くか（`docs/slo.md` の「分かっている穴」）
  - 応答時間はこの形式に入っていない。集計したいならどうするか（`log_format` に `$request_time`、または Go のメトリクス）
- **LPIC-1**：103.2（フィルタ）、103.4（パイプとリダイレクト）、103.7（正規表現）、103.1
- **面接の問い**：*Production is throwing 5xx. What's the first command you run on the box?*

#### S10. ログ②：docker logs と JSON ログを jq で読む〔45分〕

- **目的**：構造化ログ（structured logging）の利点を、`jq` で絞り込む体験で説明できるようにする
- **教材**：`apps/api/internal/telemetry/logger.go`（pino と同じ数字の `level`、`reqId`・`trace_id`）、`observability/alloy/production.alloy`
- **環境**：🏠 手元（`pnpm dev` の Go のログ）→ ☁️ 本番
- **コマンド**

  ```sh
  # ☁️ 本番（ログに個人の情報が含まれうるので、出力は件数か項目名だけ貼る）
  sudo docker logs --since 30m juken-map-go 2>&1 | head -1 | jq 'keys'
  sudo docker logs --since 1h juken-map-go 2>&1 | jq -c 'select(.level >= 40) | {time, level, msg, reqId}' | tail -5
  sudo docker logs --since 1h juken-map-go 2>&1 | jq -r '.level' | sort | uniq -c
  sudo docker logs --since 1h juken-map-go 2>&1 | jq -r 'select(.reqId) | .reqId' | wc -l
  sudo docker inspect -f '{{.HostConfig.LogConfig.Type}} {{json .HostConfig.LogConfig.Config}}' juken-map-go
  sudo docker inspect -f '{{.LogPath}}' juken-map-go | xargs sudo ls -lh
  ```

- **考える問い**
  - `level` が文字列でなく 30・40・50 なのはなぜか（Node の pino と揃えた理由）
  - 1つの `reqId` から、Grafana のトレースへどう辿れるか
  - ログドライバを変えずに Alloy が Docker のソケットから拾う形にした利点と弱点
- **LPIC-1**：103.2・103.4（フィルタとパイプ）、108.2（ログ）
- **面接の問い**：*Why structured logs? How do you correlate a log line with a trace?*

### 第6週

#### S11. DNS：juken-map.com から RDS のエンドポイントまで〔45分〕

- **目的**：名前解決の流れ（stub resolver → キャッシュ → 権威サーバー）を `dig` で追い、VPC の中と外で答えが変わることを確かめる。利用者向けの名前（CloudFront）とオリジンの名前（EC2）が分かれていることも確かめる
- **教材**：`terraform/dns.tf`（apex・www を CloudFront へ向ける CNAME、`origin` の A レコード）、`infra/nginx/README.md`（Cloudflare のプロキシを外した経緯・CloudFront を前に置いた経緯）、`conf.d/juken-map-www-redirect.conf`
- **環境**：🖥 Mac → ☁️ 本番
- **コマンド**

  ```sh
  # 🖥 Mac
  dig juken-map.com
  dig +short juken-map.com www.juken-map.com
  dig NS juken-map.com +short
  dig +trace juken-map.com | tail -8
  dig -x "$(dig +short juken-map.com | head -1)" +short
  dig +short origin.juken-map.com
  RDS=$(aws rds describe-db-instances --db-instance-identifier juken-map-db --region ap-northeast-1 \
    --query 'DBInstances[0].Endpoint.Address' --output text); echo $RDS
  dig +short $RDS

  # ☁️ 本番
  cat /etc/resolv.conf | grep -v '^#'
  resolvectl status | head -15
  getent hosts juken-map.com
  dig +short <上で出た RDS のエンドポイント>
  grep hosts /etc/nsswitch.conf
  ```

- **考える問い**
  - 同じ RDS のエンドポイントが、Mac と EC2 で同じ IP を返すか。違うなら何を意味するか
  - `127.0.0.53` は誰か。その先はどこに問い合わせているか
  - TTL を見て、IP を変えたときに切り替わるまでの時間を見積もる
  - `juken-map.com` は IP がいくつ返るか。`dig` を数回打つと変わるか。`origin.juken-map.com` と比べて何が違うか
  - apex（`juken-map.com`）には本来 CNAME を置けない。`dns.tf` ではどう扱っているか（Cloudflare の CNAME flattening）
- **LPIC-1**：109.4（クライアント側の DNS・`/etc/resolv.conf`・`/etc/hosts`・`nsswitch.conf`）、109.1
- **面接の問い**：*What happens when you type juken-map.com into a browser?*（DNS の部分を詳しく）

#### S12. TCP と TLS：curl -v・openssl s_client・tcpdump〔60分〕

- **目的**：3ウェイハンドシェイク → TLS → HTTP の順に、実際のパケットと証明書を見る。TLS が CloudFront と nginx の2か所で終端していて、証明書も2枚あることを確かめる
- **教材**：`terraform/cloudfront.tf`（ACM の証明書・`redirect-to-https`）、`conf.d/juken-map-www-redirect.conf`（www → apex）、`certbot.timer`（オリジンの Let's Encrypt。webroot で更新）
- **環境**：🖥 Mac（自分の通信だけを見るので安全）→ ☁️ 本番はオリジンの証明書と更新の確認だけ
- **コマンド**

  ```sh
  # 🖥 Mac：CloudFront は IP を複数返すので、1つに決めてから見る
  IP=$(dig +short juken-map.com | head -1); echo $IP
  # ターミナルを2つ使う。片方で（IP は上で出た値を入れる）
  sudo tcpdump -i en0 -nn -c 20 "host <上の IP> and tcp port 443"
  # もう片方で（--resolve でその IP に固定する）
  curl -sv --resolve juken-map.com:443:$IP https://juken-map.com/api/health -o /dev/null 2>&1 | grep -E '^\*|^> |^< ' | head -30
  curl -sI http://juken-map.com | head -3          # EC2 の 80 は閉じている。この応答は誰が返しているか
  curl -sI https://www.juken-map.com | head -3
  curl -s -o /dev/null -w 'dns=%{time_namelookup} tcp=%{time_connect} tls=%{time_appconnect} ttfb=%{time_starttransfer} total=%{time_total}\n' https://juken-map.com/api/health
  echo | openssl s_client -connect juken-map.com:443 -servername juken-map.com 2>/dev/null \
    | openssl x509 -noout -subject -issuer -dates -ext subjectAltName
  echo | openssl s_client -connect juken-map.com:443 -servername juken-map.com -alpn h2 2>/dev/null | grep -E 'ALPN|Protocol|Cipher'
  curl -sI https://juken-map.com/ | grep -i -E '^(x-cache|via|x-amz-cf-pop)'
  curl -sv --max-time 5 https://origin.juken-map.com/ -o /dev/null 2>&1 | tail -3   # 予想してから打つ

  # ☁️ 本番：オリジン（nginx）の証明書
  echo | openssl s_client -connect 127.0.0.1:443 -servername juken-map.com 2>/dev/null \
    | openssl x509 -noout -subject -issuer -dates
  systemctl list-timers certbot.timer --no-pager
  sudo certbot certificates 2>/dev/null | grep -E 'Domains|Expiry'
  sudo grep -E 'authenticator|webroot' /etc/letsencrypt/renewal/juken-map.com.conf
  ```

- **考える問い**
  - tcpdump の最初の3行の `[S]`・`[S.]`・`[.]` は何か。その次に来るのは何か
  - `-servername` を外すと何が変わりうるか（SNI）
  - `time_connect` と `time_appconnect` の差は何の時間か。東京とオレゴンでどう変わるか
  - Mac で見た証明書と、本番の 127.0.0.1:443 で見た証明書は、発行者が何か。なぜ2枚あるのか
  - Mac から `origin.juken-map.com` に直接つながらないのはなぜか（S14 の SG とつなげる）
  - 会社の Wi-Fi からつながらなかったのを、証明書を ACM に替えて直した（JUK-185）。証明書の「チェーン」の何が違ったと考えられるか
  - TLS を nginx で終端し、Go までは平文にしているのはなぜか。それで良い条件は何か
- **LPIC-1**：109.1（TCP・ポート）、109.3（ネットワークのトラブルシュート）。TLS・`openssl s_client` 自体は LPIC-1 の範囲外（面接向け）
- **面接の問い**：*Walk me through a TLS handshake. Where is TLS terminated in your setup — and why twice?*

### 第7週

#### S13. nginx の 443 → 127.0.0.1:8080/8081 と ss〔60分〕

- **目的**：CloudFront から届いた 443 が、どのプロセスを経て Go に届くかを、待ち受けのポートと設定ファイルで1本につなげる
- **教材**：`conf.d/juken-map-upstream.conf`（リポジトリの控え）、`juken-map-go-routes.conf`、`conf.d/juken-map-cloudfront-realip.conf`（利用者の IP を取り出す）、`deploy-ec2.sh` の `current_port`・`write_upstream`
- **環境**：☁️ 本番 → 🏠 手元（`scripts/local-proxy.sh`）
- **コマンド**

  ```sh
  # ☁️ 本番
  sudo ss -ltnp
  sudo ss -tn state established '( sport = :443 )' | wc -l
  sudo ss -tn state established '( sport = :443 )' | awk 'NR>1 {split($4,a,":"); print a[1]}' | sort | uniq -c | head   # 相手は誰か
  grep -v '^#' /etc/nginx/conf.d/juken-map-cloudfront-realip.conf | head -5
  cat /etc/nginx/conf.d/juken-map-upstream.conf /etc/nginx/conf.d/juken-map-go-upstream.conf | grep -v '^#'
  sudo docker port juken-map-go
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8080/api/health
  curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8081/api/health
  grep -n include /etc/nginx/sites-enabled/default
  sudo iptables -t nat -S DOCKER

  # 🏠 手元
  bash scripts/local-proxy.sh      # 別のターミナル。止めるときは Ctrl-C
  lsof -nP -iTCP -sTCP:LISTEN | grep -E 'nginx|docker|com.docke|api'
  ```

- **考える問い**
  - リポジトリの `juken-map-upstream.conf` は 3000 と書いてある。本番とどちらが正しいか、なぜずれているか
  - コンテナのポートを `0.0.0.0` でなく `127.0.0.1` に開けた理由。セキュリティグループがあっても、なぜそうするか（多層防御 / defense in depth）
  - 8080 と 8081 のうち片方が応えないのは正常か
  - `ss` の `LISTEN` と `ESTAB`、`Recv-Q` と `Send-Q` は何を表すか
  - 443 につないでいる相手の IP は、利用者か CloudFront か。realip の設定が無いと、アクセスログの IP はどうなるか
  - realip で「X-Forwarded-For の末尾」だけを信じる（`real_ip_recursive off`）のはなぜか。SG を広げたら、なぜ realip も外す必要があるか
- **LPIC-1**：109.3（`ss`・`lsof` でポートを調べる）、110.1（開いているポートの確認）
- **面接の問い**：*A request hits port 443. Trace it to your application process.*

#### S14. VPC・サブネット・CIDR・セキュリティグループ・EC2 → RDS〔60分〕

- **目的**：CIDR を計算でき、EC2 から RDS への通信が「経路（ルートテーブル）」と「許可（SG）」の両方で決まることを説明できるようにする
- **教材**：`terraform/vpc.tf`（デフォルト VPC）・`sg.tf`（EC2 への 443 を CloudFront のプレフィックスリストだけに許可、RDS への 3306 を SG の参照で許可）・`rds.tf`、比較用に `terraform/loadtest/network.tf`（`10.50.0.0/16` を /24 に切る）
- **環境**：🖥 Mac（aws の読み取り・`terraform plan`）→ ☁️ 本番
- **コマンド**

  ```sh
  # 🖥 Mac：CIDR の計算（ipcalc の代わりに Python の標準ライブラリ）
  python3 -c "import ipaddress as i; n=i.ip_network('10.50.0.0/16'); print(n.num_addresses, list(n.subnets(new_prefix=24))[:3])"
  python3 -c "import ipaddress as i; print(i.ip_address('10.50.10.5') in i.ip_network('10.50.10.0/24'))"
  aws ec2 describe-vpcs --region ap-northeast-1 --query 'Vpcs[].{Id:VpcId,Cidr:CidrBlock,Default:IsDefault}'
  aws ec2 describe-subnets --region ap-northeast-1 --query 'Subnets[].{Id:SubnetId,Az:AvailabilityZone,Cidr:CidrBlock,Public:MapPublicIpOnLaunch}' --output table
  aws ec2 describe-route-tables --region ap-northeast-1 --query 'RouteTables[].Routes[].{Dst:DestinationCidrBlock,Gw:GatewayId}'
  aws ec2 describe-security-groups --region ap-northeast-1 --query 'SecurityGroups[].{Name:GroupName,In:IpPermissions[].{Port:FromPort,Cidr:IpRanges[].CidrIp,FromSG:UserIdGroupPairs[].GroupId,PL:PrefixListIds[].PrefixListId}}'
  aws ec2 describe-managed-prefix-lists --region ap-northeast-1 --filters Name=prefix-list-name,Values=com.amazonaws.global.cloudfront.origin-facing \
    --query 'PrefixLists[].{Id:PrefixListId,Max:MaxEntries}'
  # 🖥 Mac：terraform は plan まで（state が本体の terraform/ にあるので、本体で読み取りだけ行う）
  # Cloudflare の DNS も管理しているので、キーチェーンの juken-map-cloudflare-dns を CLOUDFLARE_API_TOKEN に渡す（値は表示しない）
  cd ~/dev/product/juken-map/terraform && terraform plan -lock=false 2>&1 | tail -15

  # ☁️ 本番
  ip -br addr; ip route
  PID=$(sudo docker inspect -f '{{.State.Pid}}' juken-map-go)
  sudo nsenter -t $PID -n ss -tn '( dport = :3306 )'      # コンテナの中から見た RDS への接続
  sudo ss -tn '( dport = :3306 )'                          # ホストから見ると？
  nc -zv -w 3 <RDS のエンドポイント> 3306
  # 🖥 Mac から同じ nc を打つとどうなるか、予想してから
  ```

- **考える問い**
  - `/16` と `/20` と `/24` には、それぞれ何個のアドレスがあるか。AWS が各サブネットで5個を予約するのはなぜか
  - RDS の SG が CIDR ではなく「EC2 の SG」を参照して許可しているのはなぜか
  - EC2 の 443 を `0.0.0.0/0` ではなくプレフィックスリストで絞ると、何が防げるか。80 を閉じても困らないのはなぜか
  - `nsenter` で見た接続が、ホストの `ss` では見えないのはなぜか（network namespace と NAT）
  - Mac から 3306 に届かない理由は、名前解決・経路・SG・`publicly_accessible` のどれか。いくつ重なっているか
  - 本番は Single-AZ・EC2 1台（`docs/slo.md` の「分かっている穴」）。可用性を上げるなら、ネットワークの何から変えるか
- **LPIC-1**：109.1（IPv4・CIDR・プライベートアドレス）、109.2（`ip addr`・`ip route`）、109.3（`nc`・経路の確認）。VPC・SG・Terraform は範囲外（AWS・面接向け）
- **面接の問い**：*How does your app server reach the database, and what stops anyone else from reaching it — or from bypassing your CDN?*

### 第8週

#### S15. Bash②：ヘルスチェックのスクリプトを自分で書く〔60分〕

- **目的**：終了ステータス・タイムアウト・再試行を備えた小さなスクリプトを書き、デプロイのスモークテストや外形監視と比べる
- **教材**：`deploy-ec2.sh` のスモークテスト（最大45秒、3つの URL のステータスを見る）、`docs/slo.md`（Synthetic Monitoring は東京とオレゴンから1分ごと）
- **環境**：🖥 Mac（`~/infra-lab/healthcheck.sh` に書く。リポジトリには入れない）→ 🧪 箱の cron で定期実行
- **作るものの仕様**
  - 引数で URL を受け取る（省いたら `https://juken-map.com/api/health`）
  - `curl` に `--max-time` を付け、ステータスコードと応答時間を取る
  - 失敗したら N 回まで再試行し、間を空ける
  - 終了ステータス：成功 0・遅い 1・失敗 2。1行のログ（時刻・URL・コード・秒）を出す
  - `set -euo pipefail` を付け、どこで止まるかを確かめる
- **コマンド**

  ```sh
  mkdir -p ~/infra-lab && vi ~/infra-lab/healthcheck.sh
  bash -n ~/infra-lab/healthcheck.sh             # 構文の確認
  shellcheck ~/infra-lab/healthcheck.sh          # brew install shellcheck
  bash ~/infra-lab/healthcheck.sh; echo "exit=$?"
  bash ~/infra-lab/healthcheck.sh http://localhost:1/; echo "exit=$?"   # わざと失敗させる
  bash -x ~/infra-lab/healthcheck.sh 2>&1 | head -20
  echo $PATH; type curl; alias                     # シェルの環境
  ```

- **考える問い**
  - `/api/health` は DB に ping する。ヘルスチェックが DB まで見ることの良い点と悪い点（liveness と readiness）
  - 自分のスクリプトを本番の EC2 の中から回すのと、外から回すのでは、見つけられる障害がどう違うか
  - 本番に対しては何秒ごとまでなら回してよいか（`/api/health` のリクエスト数と SLO の集計への影響）
- **LPIC-1**：105.2（簡単なスクリプト）、105.1（シェルの環境・`PATH`・`alias`）、107.2（cron で回す）
- **面接の問い**：*How do you know your service is up? What's the difference between a liveness and a readiness check?*

#### S16. Bash③：デプロイの切り替え処理を読み解く ＋ 構成を英語で説明する〔60分〕

- **目的**：`deploy-ec2.sh` の主要な節を自分の言葉で説明し、16回の内容を「5分で話せる構成の説明」にまとめる
- **教材**：`.github/workflows/deploy.yml`（base64 で運び、SSM Run Command で流し、結果をポーリング）、`.github/scripts/deploy-ec2.sh`
- **環境**：🖥 Mac（読むだけ）→ 🏠 手元（`scripts/test-deploy-*.sh`）
- **コマンド**

  ```sh
  grep -nE '^[a-z_]+\(\)|^# ----' .github/scripts/deploy-ec2.sh       # 関数と節の一覧
  sed -n '47,70p' .github/scripts/deploy-ec2.sh                       # current_port と pipefail の落とし穴
  grep -nE '\$\{[A-Z_]+:-' .github/scripts/deploy-ec2.sh | head       # 既定値の書き方
  bash scripts/test-deploy-ports.sh 2>&1 | tail -10
  bash scripts/test-deploy-timers.sh 2>&1 | tail -10
  bash scripts/test-deploy-logrotate.sh 2>&1 | tail -10
  # 自分で試す：pipefail と set -e の効き方
  bash -c 'set -euo pipefail; sed -n 1p /nope | head -1; echo "ここに来る？"'; echo "exit=$?"
  ```

- **読み解く節**
  1. 引数（イメージのタグ・base64 の設定）の受け取り方と、`${VAR:-default}` で手元から差し替えられるようにした理由
  2. `current_port`：ファイルが無い初回に `sed | head` が無言で落ちた事故（2026-09-23）
  3. 新しいポートでコンテナを起こす → `curl -w '%{http_code}'` のループで待つ → `nginx -t` → `reload` → 待つ → 古い方を止める
  4. 失敗したとき、どの時点なら「本番は無傷」と言えるか
  5. シークレットを一時ファイル（600）に置いて `--env-file` で渡し、引数に載せない理由（`ps` で見える）
- **まとめの課題**：次の問いに英語で5分ずつ答え、`infra-basics-progress.md` に要点を書く
  - *Walk me through your infrastructure end to end.*
  - *Tell me about an incident you had and what you changed.*（2026-09-23 の pipefail、または 502 の件）
  - *What would you change first if traffic grew 100x?*
- **LPIC-1**：105.2（スクリプト・条件・ループ・関数）、103.4（リダイレクト・パイプの失敗）
- **面接の問い**：*Explain your deployment pipeline. What happens if the new version is broken?*

---

## 6. LPIC-1 の出題範囲との対応

| 範囲 | 扱う回 |
| -- | -- |
| 101.2 起動・101.3 target と停止 | S6 |
| 102.4 Debian のパッケージ | S6 |
| 102.6 仮想マシンのゲスト | S1・S4 |
| 103.1 コマンドライン・103.2 フィルタ・103.4 パイプ | S9・S10・S16 |
| 103.5 プロセス・103.6 優先度 | S4・S5 |
| 103.7 正規表現 | S9 |
| 103.8 vi | S2・S15 |
| 104.1・104.3 パーティションとマウント（読むだけ） | S8 |
| 104.5 パーミッション | S2・S3 |
| 104.6 リンク | S6 |
| 104.7 FHS | S8 |
| 105.1 シェルの環境・105.2 スクリプト | S15・S16 |
| 107.1 ユーザーとグループ | S2 |
| 107.2 ジョブの予約 | S7・S15 |
| 107.3・108.1 タイムゾーンと時刻 | S7 |
| 108.2 ログ | S8・S10 |
| 109.1 プロトコル・109.2 ネットワークの設定・109.3 トラブルシュート | S12・S13・S14 |
| 109.4 クライアント側の DNS | S11 |
| 110.1 セキュリティの管理・110.2 ホストのセキュリティ | S2・S3・S13 |
| 110.3 暗号化（SSH の鍵） | S1 |

**この構成では扱えない範囲**（別の教材で補う）：101.1 ハードウェア、102.1 ディスクの設計・102.2 ブートローダー（GRUB）・
102.3 共有ライブラリ・102.5 RPM と YUM、104.1・104.2 ファイルシステムの作成と検査（🧪 VM でループデバイスを使えば練習できる）、
106 デスクトップ、108.3 メール（MTA）・108.4 プリンター。
