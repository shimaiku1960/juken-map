# 起きたときの手順

秘密情報の漏えいや乗っ取りが起きたときに、最初にやることを書いておく。
起きてから手順を考えると止めるのが遅れるため（開発基準 06 H5）。

## 機械の入口のトークンを差し替える

GitHub Actions から本番の API を呼ぶためのトークンが2つある。
漏れた（ログや画面に出た、コミットした、など）と思ったら、両方の置き場所を新しい値にそろえる。

| トークン | 守っている入口 | 置き場所 |
| --- | --- | --- |
| `DAILY_NOTIFICATION_SECRET` | `POST /api/cron/daily-study-notifications`（朝・夜の学習通知） | 本番サーバーの `.env` と GitHub Secrets |
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

4. GitHub Secrets を同じ値にする。

   ```sh
   gh secret set DAILY_NOTIFICATION_SECRET   # 値は標準入力から貼る
   ```

5. 確かめる。古い値で叩いて 401 になり、GitHub Actions の次の実行（または手動実行）が成功すること。

   ```sh
   curl -s -o /dev/null -w "%{http_code}\n" -X POST \
     -H "Authorization: Bearer <古い値>" -H "Content-Type: application/json" \
     -d '{"slot":"morning"}' https://juken-map.com/api/cron/daily-study-notifications
   ```

3 と 4 の間は GitHub Actions からの呼び出しが 401 になる。朝7時・夜21時の通知の前後は避ける。

`SIMULATION_SECRET` は、シミュレーションが保存するログイン状態の暗号化の鍵も兼ねている。
差し替えると前回までの状態を読めなくなり、次の実行はログインし直すところから始まる。
