## 構成の前提

このリポジトリは pnpm workspace のモノレポで、`apps/web` が React 19 + Vite の
SPA、`apps/api` が Go の API サーバーである。本番は Go が API・ログイン・ビルド済み SPA の
すべてを配る（JUK-70・JUK-115・JUK-111）。運用のコマンド（`pnpm incident`・`pnpm admin:grant`）と、マイグレーションの
適用（`pnpm db:migrate`、`apps/api/migrate.go`）も Go にある（JUK-122・JUK-125）。開発でしか使わない
DB の道具（seed・テスト用 DB の準備・Node の DB 接続）は `db/`（Node.js 24 の TS）にあり、本番では使わない（JUK-130）。Next.js は使っていない（2026-09に削除済み）ため、
App Router・Server Components・Server Actions・`next/*` の作法を持ち込まないこと。

DBは MySQL 8.4 で、ORM は使わず SQL を直接書く（Go は `database/sql`、Node の seed は `mysql2`）。
ルートの一覧は `apps/api/main.go` の `registerRoutes`、入口の種類ごとの拒否は `apps/api/internal/httpx/router.go` にある。

## 依存関係とlockfile

パッケージ管理は `package.json` の `packageManager` に固定した pnpm を使う。
ルート・`apps/web`・`db` は pnpm workspace で、lockfile はルートの
`pnpm-lock.yaml` 1つを正とする。npm install / npm ci や個別の package-lock.json は使わない。

依存関係または `pnpm-workspace.yaml` を変更したら `pnpm install` でlockfileを更新し、
必ず `pnpm run lock:linux` で本番と同じ Linux/amd64 の
`pnpm install --frozen-lockfile` 成功を確認する。変更したmanifest・workspace設定・lockfileは
一緒にコミットする。通常の非破壊確認は `pnpm run lock:check` を使う。
必要な依存のinstall scriptだけを `pnpm-workspace.yaml` の `allowBuilds` で許可する。

## 作業は worktree で行う

ファイルやブランチを変える作業は、本体のチェックアウト（`juken-map/`）で直接行わず、
Issue ごとに `pnpm wt:new <ブランチ名>` で worktree を作り、その中で行う。本体は `main` の
まま置いておき、worktree を作る起点にだけ使う。1つのチェックアウトには HEAD と index が
1つしかないため、2つのセッションが同じ場所で作業すると、片方のコミットがもう片方の
ブランチに乗る（2026-09-21 に実際に起きた）。並行するかどうかは始める時点で分からないので、
常に分ける。調査・質問への回答・本番確認のように何も変えない作業は本体のままでよい。

- worktree は `../juken-map-worktrees/<ブランチ名>` にでき、ポート（Vite・API・E2E）は
  worktree ごとにずれる。エージェントはそのディレクトリで起動する。本体で起動済みの
  セッションは、worktree のパスを明示して操作する。
- `.env` は本体へのリンクなので、worktree で書き換えない。worktree だけの値は `.env.worktree` に書く。
- DB は全 worktree で同じコンテナを共有する。マイグレーションを足すブランチでは、
  ほかの worktree の DB も変わる。
- `main` を取り込んで依存が変わったら、その worktree で `pnpm install` する。
- マージ後は `pnpm wt:remove <ブランチ名>` で片付ける。未コミットの変更や main に入って
  いないコミットがあれば止まるか、ブランチを残す。
- Claude Code の組み込みの worktree（`.claude/worktrees/`）は、リンク・ポート・install を
  用意しないので使わない。
- Claude Code では、本体で `main` 以外へ移る `git switch` / `git checkout` / `gh pr checkout` と、
  本体のファイル（gitignore 済みの `.env` などを除く）を Edit / Write で書き換えることを
  フック（`.claude/settings.json` → `scripts/guard-main-checkout.py`）が止める。Codex には
  この仕組みが無いので、ルールとして守る。
- Linear の Issue に着手したら、セッションID（Claude Code なら `CLAUDE_CODE_SESSION_ID` の
  先頭8文字）と worktree 名をコメントする。ほかのセッションが着手した Issue や、既に worktree が
  ある Issue には手を出さない。

## PR は CI が通ったらエージェントがマージする

PR を出したら、ユーザーに確認せずに次まで続けて行う（2026-10-05 ユーザー判断、JUK-145）。
main へのマージは本番デプロイになる（`deploy.yml`、スモークテストと失敗時のロールバックあり）。

1. `gh pr checks <番号> --watch` で CI（`lockfile`・`check`・`go`・`e2e`）を待つ。
2. 落ちたら直して push し、もう一度待つ。直し方に判断が要るときはユーザーに聞く。
3. すべて通ったら `gh pr merge <番号> --squash` でマージする。
4. `pnpm wt:remove <ブランチ名>` で片付け、Linear の Issue を Done にする。

次の PR は、CI が通っても**マージせずユーザーに確認する**。

- 新しいマイグレーション（`db/migrations/`）を含む PR（本番の DB の形が変わり、ローカルでは全 worktree の DB も変わる）
- `terraform/` を変える PR（本番のインフラが変わる）

デプロイの完了待ちと本番での確認は、頼まれたときだけ行う。

## 読む量を抑える

読んだ出力は会話に残り、以後の往復のたびに読み直される。2026-10-05 に測ったところ、
ツールの出力では `sed`・`cat`・`grep` によるファイルの読み込みが約4割で最も多かった（JUK-60）。

- ファイルは丸ごと読まず、`grep -n` で場所を見つけてから、必要な行の範囲だけ読む。
- テスト・ビルド・lint の出力は `| tail -40` などで絞り、失敗したときだけ詳しく見る。
  `go test` は `-run` で対象を絞る。
- `git diff`・`git log`・`gh run view --log` は `--stat`・`-n`・`--log-failed` から始める。
- 画面の確認は、文字で足りるなら `get_page_text` / `read_page` を使い、
  スクリーンショットは `scale` を下げて撮る。
- 1つの Issue を1つのセッションにする。PR をマージしたら（確認を残す PR は出したら）、進捗を Linear に残してから、
  次の作業は `/clear` して始めるようユーザーに勧める。

## 言語

ユーザーへの応答は、この指示や読んだドキュメントの言語にかかわらず、常に日本語
で行うこと。コード・識別子・コミットメッセージは各プロジェクトの既存の慣習に従う
が、ユーザーへの会話としての返答はすべて日本語であること。

## 開発基準

品質の判断基準（フロントエンド・バックエンド・DB・インフラ・SRE・セキュリティ・
DevOps・テストの8分野）は、プロジェクト横断の非公開リポジトリ
`shimaiku1960/dev-standards` を正とする。索引はリポジトリ内の gitignore 済みの
エントリポイント経由で読む：

`./.standards/INDEX.md`

必要な分野の文書だけを追加で読むこと。**基準に対する判定・実測値・実装との対応表は
dev-standards の `projects/juken-map/` に置く**（基準そのものの `targets/` には書かない）。
判断の経緯は今までどおり auto-memory の該当トピックに残す。`./.standards` が無い環境では
`git clone https://github.com/shimaiku1960/dev-standards.git ~/dev/standards` してから
シンボリックリンクを張る。

## プロジェクトメモリとタスク管理

過去の決定・経緯・実測値・ユーザーの作業上の好みは、Claude Code の auto-memory を正
（source of truth）として扱う。入口はリポジトリ内の gitignore 済みの
`./.agent-memory/MEMORY.md`（索引）で、詳細が要るときだけ、そこから参照されている
トピックファイルを読む。リポジトリ内の `memory/MEMORY.md` は使わない（あちらは
Claude から Codex への移行状態を記録するもの）。

「これからやるタスク」は Linear（ワークスペース `juken-map`、チームキー `JUK`）を正とし、
Linear MCP で読み書きする。メモリは「なぜ」、Linear は「次に何をやるか」と分ける。
**進捗（残り・作業中・完了・次の一手）は Linear だけに書き、メモリには書かない**。

- **作業開始時**は `git -C .agent-memory pull --rebase` で最新を取り込み、Linear の
  In Progress / Todo（In Progress は最新コメントまで）と、MEMORY.md から辿る関連トピックの
  文脈を把握する。
- **これからやるタスク**は Linear の Issue にする。完了したら Done、やめたら Canceled に
  する。作業の文脈（なぜ・注意点・実測値）は今までどおりトピックファイルに書く。
- **Issue を作るときは、ラベルグループ「分野」から必ず1つ付ける**。分野は dev-standards の
  8分野（フロントエンド・バックエンド・DB・インフラ・SRE・セキュリティ・DevOps・テスト）と
  その他。迷ったら各ラベルの説明を見る。Bug / Feature / Improvement は種類の軸なので別に付けてよい。
- **途中で作業を止めるとき**は、作業中の Issue に「次の一手」を1〜2行コメントする。
  Project・Cycle・Status Update は使わない。
- **ブランチ名か PR 本文に `JUK-xx` を入れる**（Linear の GitHub 連携で状態が動く）。
  GitHub Issues は使わない。
- **Linear MCP が使えない環境**では、タスクをメモリに書いて済ませず、ユーザーに伝える。
- **メモリを更新したら** `.agent-memory` の中で commit して push する。
- **脆弱性・セキュリティ穴は公開の場に書かない**（本番リポジトリは public）。
  GitHub Security Advisory（非公開）で扱い、Linear にも書かない。
- **MEMORY.md は索引として短く保つ**。1トピック1行・120字以内で「何のトピックか」だけを
  書き、「完了」「次は〜」のような状態は書かない。経緯・実測値・PR番号はトピックファイル側へ書く。毎セッション必ず読み込まれるファイルだからである。

### 自動更新の許可（常設）

意味のあるひとまとまりの作業が完了したときは、**毎回の確認なしに**共有プロジェクト
メモリを更新してよい。対象は、機能の実装や修正、恒久的なアーキテクチャ上・プロダクト上の
決定、プルリクエストの作成やマージ、デプロイなどの運用マイルストーン。逆に、質問への回答、
読み取り専用の調査、通常のテスト実行、途中経過、未完成の実装では更新しない。

記入者と絶対日付の帰属ルール、トピックファイルの選び方、`./.agent-memory` や
`./.standards` が無い環境での用意の仕方は、**`docs/agent-memory-rules.md`** に書いてある。
メモリを書き換えるときや、環境を新しく用意するときに、そのファイルを読むこと。
