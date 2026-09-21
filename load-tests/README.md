# Load tests

負荷試験は本番を対象にしない。`load-tests/phase0.js`はローカルURL以外を拒否する。

## Phase 0

Better AuthのVU別Cookie維持と、同じ学習予定を同時完了した場合の競合応答を確認する。

1. ビルド済みSPAを配信するAPIを3000番で起動する（`bash scripts/e2e-server.sh`）。
2. `bash scripts/run-loadtest-phase0.sh`を実行する。

実行スクリプトはローカルDBへ専用の合成ユーザーと予定を再作成し、Docker版k6を実行する。期待結果は次のとおり。

- 2 VUがそれぞれログインし、`GET /api/study-plans`で200を受け取る。
- 同一予定への同時完了が201と409を1件ずつ返す。
- 想定外の500を返さない。

Phase 0が現在の競合不具合を検出した場合はテスト失敗になる。その結果を固定してからAPIを修正する。

### 2026-09-03の初回結果

- 2 VUのログインと認証後GETはすべて200。
- 同一予定の同時完了は`201,500`になった。
- 500の原因は`StudyLog.studyPlanId`の一意制約に対するPrisma `P2002`。事前確認を両リクエストが通過し、後着の`studyLog.create`が未処理例外になっている。

期待する修正後の結果は`201,409`で、`unexpected_5xx=0`とする。

## 限界点の測定（limit.js）

「どこで壊れるか」を測る。想定規模のデータ量（study_log 1,000万行）を入れた手元のDBに対し、
RPSを段階的に上げ、最初に基準を割った段階を限界点として記録する。

1. 想定規模のデータを入れる（約8分・ディスク約7GB）。

   ```
   USERS=42000 MONTHS=36 pnpm db:seed:synthetic
   ```

2. 本番と同じ構成（Fastify が API と SPA を配る）を3000番で起動する。
   本番は `NODE_ENV=production` なので、合わせないとSQLログの分だけ遅くなる。

   ```
   NODE_ENV=production bash scripts/e2e-server.sh
   ```

3. 別のターミナルで段階実行する。

   ```
   bash scripts/run-loadtest-limit.sh
   RATES="100 200" DURATION=30s bash scripts/run-loadtest-limit.sh   # 短く試す
   ```

各段階の結果は `load-tests/results/limit-<日時>.jsonl` に1行ずつ入る（gitignore 済み。
数字は auto-memory の quality-current-state へ残す）。段階ごとに次を記録する。

- APIごとの応答時間（p50 / p95 / p99 / 最大）と応答の大きさ
- 失敗率・5xx・取りこぼし（dropped_iterations＝k6が予定した時刻に出せなかった数）
- DBの `Threads_running` 最大とCPU、`SHOW GLOBAL STATUS` の差分

基準（`dev-standards` の暫定値）は read p95 < 1秒、write p95 < 1.5秒、失敗率 < 1%、
想定外の5xx = 0、取りこぼし0。どれかを割った段階で止まる（`CONTINUE_AFTER_BREAK=on` で続行）。

⚠️ 手元のMacはアプリ・DB・k6が同じCPUを取り合うので、出てくるRPSの絶対値は本番（EC2＋RDS）の
値ではない。ここで分かるのは「どのAPIが最初に遅くなるか」と「何が原因か」で、
本番の実速度は Grafana Cloud 側で見る。
