# シミュレーション（sim/）

合成ユーザーが本番を、実際の利用者と同じ HTTP の動線で使い、「運用されているサービスの状態」
（データの量と偏り、夜に偏るアクセスの波、続く人・やめる人）を毎日少しずつ積み上げる仕組み。

負荷試験（`load-tests/`）が「一度に強くかけて限界を見る」のに対し、こちらは「普通の毎日を
長く続けて、データが育ったときに何が起きるかを見る」ためのもの。

## 動き方

GitHub Actions（`.github/workflows/simulation.yml`）が毎時 `sim/run-hour.ts` を動かす。1回ごとに次の順で進む。

1. `GET /api/sim/state` で合成ユーザーの一覧を受け取る
2. 寿命を過ぎた人に「来なくなった日」を付ける
3. 今日この時間に使う人を、ペルソナ（`persona.ts`）と日付から決めて、画面と同じ順番で API を叩く（`flows.ts`）
4. 今日の新規登録のうち、この時間までに済んでいるべき人数に足りない分を登録する

- **登録も実際と同じ経路を通る。** 宛先は Resend のテスト用アドレス（`delivered+simNNNNN@resend.dev`）。
  アプリは実際の利用者と同じく確認メールを送る。シミュレーターは Resend の API でその本文を読み、
  リンクを踏む（`resend-inbox.ts`）。
- ペルソナは連番と日付だけから決まる純粋関数なので、ランナーがまっさらでも同じ人は同じように振る舞う。
  DB に残すのは「実際に起きたこと」（`user.simLastActedOn` / `simDormantFrom`）だけ。
- 「今日すでにやったか」を DB の値で判断するので、同じ時間に2回動いても二重にはならない。
- ログイン状態（Cookie）は暗号化して `actions/cache` で次の回へ引き継ぐ。消えてもログインし直すだけ。
- シミュレーターのリクエストには `X-Sim-Run` ヘッダーが付き、API のログに `sim:true` が出るので、Grafana Cloud で実際の利用者と分けて読める。

## 守り

- `/api/sim/*` は `SIMULATION_ENABLED=on` のときだけ存在する（無ければ 404）
- `SIMULATION_SECRET` の Bearer が必須（未設定なら常に 401）
- `/api/sim/*` が触れるのは `delivered+simNNNNN@resend.dev` の利用者だけ
- 通知（朝・夜のメール）は ON にしない。送信が逐次なので、実際の利用者への通知まで遅れる
- `SIM_BASE_URL` は手元（`http://localhost:*`）か `https://juken-map.com` だけ。`--hour` は手元でだけ使える

## 必要な設定

| どこ | 名前 | 中身 |
|---|---|---|
| 本番サーバーの `.env` | `SIMULATION_ENABLED` | `on` で `/api/sim/*` を有効にする |
| 本番サーバーの `.env` | `SIMULATION_SECRET` | 長いランダムな値 |
| GitHub Secrets | `SIMULATION_SECRET` | 上と同じ値。Cookie を暗号化する鍵も兼ねる |
| GitHub Secrets | `RESEND_READ_API_KEY` | Resend の **Full access** のキー（送ったメールの本文を読むため）。アプリ本体には持たせない |
| GitHub Variables | `SIMULATION_SCHEDULE` | `on` で毎時の実行を始める。無ければ手動実行だけ |
| GitHub Variables | `SIGNUPS_PER_DAY` | 1日の新規登録の人数（既定 20） |

合成ユーザーの登録も、実際と同じく運営者（`ADMIN_NOTIFICATION_EMAIL`）へ通知が届く。
件名と本文の `delivered+sim` でメールのフィルタを作り、実際の利用者の通知と分けて読む。

⚠️ Resend の無料枠は 1日100通・月3,000通で、実際の利用者のメールと同じ枠を使う（テスト用アドレス宛ても数える）。
合成ユーザーの登録1人につき2通（本人への確認メール＋運営者への通知）。`SIGNUPS_PER_DAY` は枠に余裕を残して決める。

## 手元で動かす

```sh
# 確認用の API を別ポートで立てる（シミュレーション用 API を有効にして）
cd apps/api && SIMULATION_ENABLED=on SIMULATION_SECRET=local-sim-secret API_PORT=4100 \
  pnpm exec tsx --env-file=../../.env src/server.ts

# 別のターミナルで（リポジトリのルート）。.env に RESEND_READ_API_KEY が要る
export SIM_BASE_URL=http://localhost:4100 SIM_ORIGIN=http://localhost:5173 SIMULATION_SECRET=local-sim-secret
pnpm run sim:run --dry-run --hour 21 --signups 5   # 何もせず、動く予定の人だけ表示
pnpm run sim:run --hour 21 --signups 5             # 実際に登録・記録する
```

`SIM_ORIGIN` は画面の住所。ブラウザは Origin に画面の住所を付けるので、それに合わせる
（Better Auth の `trustedOrigins` に無い住所だと 403）。本番は画面と API が同じ住所なので要らない。

## 予定を見る

`pnpm run sim:plan` は、今日と明日の「何時に誰が来て何をするか」（登録・志望校・予定・記録）を出す。
毎時の実行と同じ計算（`schedule.ts`）を使い、何も書き換えない。今日の分には印が付く。

- `済` … もう登録した・今日もう来た
- `遅れ（次の回で登録）` … 登録の時刻を過ぎたが、まだ登録していない。次の回で取り戻す
- `飛ばされた` … 来る時刻に実行が無かった。来訪は取り戻さないので、その人は今日は来ない

```sh
SIM_BASE_URL=https://juken-map.com SIMULATION_SECRET=... pnpm run sim:plan            # 今日と明日
SIM_BASE_URL=https://juken-map.com SIMULATION_SECRET=... pnpm run sim:plan --days 3   # 3日分
```

本番の分は、毎時の実行が終わるたびに Actions の実行結果の画面（Summary）にも出る。
`SIMULATION_SECRET` を手元に置かなくても、そこで見られる。

## 止め方

1. GitHub の Variables から `SIMULATION_SCHEDULE` を消す（毎時の実行が止まる）
2. 急ぐときは Actions の画面でワークフローを Disable する
3. API 側も閉じるなら、本番の `.env` から `SIMULATION_ENABLED` を外して再デプロイする

## 掃除

合成ユーザーと、その人の記録・予定・セッションなどは、外部キーの CASCADE でまとめて消える。
本番の RDS は外から繋げないので、SSM で EC2 に入り、コンテナの中から流す。

```sql
DELETE FROM user WHERE email LIKE 'delivered+sim%@resend.dev';
```
