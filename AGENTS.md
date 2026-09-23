## 構成の前提

このリポジトリは pnpm workspace のモノレポで、`apps/web` が React 19 + Vite の
SPA、`apps/api` が Fastify 5（Node.js 24）のAPIサーバーである。本番は Fastify が
APIとビルド済みSPAの両方を配る。Next.js は使っていない（2026-09に削除済み）ため、
App Router・Server Components・Server Actions・`next/*` の作法を持ち込まないこと。

DBは MySQL 8.4 で、ORM は使わず `mysql2` で SQL を直接書く。ルーティングは
`apps/api/src/routes/`、業務ルールとSQLは `apps/api/src/services/` にある。

## 依存関係とlockfile

パッケージ管理は `package.json` の `packageManager` に固定した pnpm を使う。
ルート・`apps/api`・`apps/web` は pnpm workspace で、lockfile はルートの
`pnpm-lock.yaml` 1つを正とする。npm install / npm ci や個別の package-lock.json は使わない。

依存関係または `pnpm-workspace.yaml` を変更したら `pnpm install` でlockfileを更新し、
必ず `pnpm run lock:linux` で本番と同じ Linux/amd64 の
`pnpm install --frozen-lockfile` 成功を確認する。変更したmanifest・workspace設定・lockfileは
一緒にコミットする。通常の非破壊確認は `pnpm run lock:check` を使う。
必要な依存のinstall scriptだけを `pnpm-workspace.yaml` の `allowBuilds` で許可する。

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

必要な分野の文書だけを追加で読むこと。**採点結果や実測値は dev-standards には書かず**、
auto-memory の該当トピックに残す。`./.standards` が無い環境では
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

- **作業開始時**は `git -C .agent-memory pull --rebase` で最新を取り込み、Linear の
  In Progress / Todo と、MEMORY.md から辿る関連トピックの文脈を把握する。
- **これからやるタスク**は Linear の Issue にする。完了したら Done、やめたら Canceled に
  する。作業の文脈（なぜ・注意点・実測値）は今までどおりトピックファイルに書く。
- **ブランチ名か PR 本文に `JUK-xx` を入れる**（Linear の GitHub 連携で状態が動く）。
  GitHub Issues は使わない。
- **Linear MCP が使えない環境**では、タスクをメモリに書いて済ませず、ユーザーに伝える。
- **メモリを更新したら** `.agent-memory` の中で commit して push する。
- **脆弱性・セキュリティ穴は公開の場に書かない**（本番リポジトリは public）。
  GitHub Security Advisory（非公開）で扱い、Linear にも書かない。
- **MEMORY.md は索引として短く保つ**。1トピック1行・120字以内。経緯・実測値・PR番号は
  トピックファイル側へ書く。毎セッション必ず読み込まれるファイルだからである。

### 自動更新の許可（常設）

意味のあるひとまとまりの作業が完了したときは、**毎回の確認なしに**共有プロジェクト
メモリを更新してよい。対象は、機能の実装や修正、恒久的なアーキテクチャ上・プロダクト上の
決定、プルリクエストの作成やマージ、デプロイなどの運用マイルストーン。逆に、質問への回答、
読み取り専用の調査、通常のテスト実行、途中経過、未完成の実装では更新しない。

記入者と絶対日付の帰属ルール、トピックファイルの選び方、`./.agent-memory` や
`./.standards` が無い環境での用意の仕方は、**`docs/agent-memory-rules.md`** に書いてある。
メモリを書き換えるときや、環境を新しく用意するときに、そのファイルを読むこと。
