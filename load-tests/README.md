# Load tests

負荷試験は本番を対象にしない。`load-tests/phase0.js`はローカルURL以外を拒否する。

## Phase 0

Better AuthのVU別Cookie維持と、同じ学習予定を同時完了した場合の競合応答を確認する。

1. 別ターミナルで`npm run dev`を起動する。
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
