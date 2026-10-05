#!/usr/bin/env bash
# 手元で Go のコマンド（apps/api の cli.go：incident・grant-admin・migrate）を実行する（JUK-122・JUK-125）。
#   pnpm incident <操作> [メールアドレス]        → bash scripts/go-cli.sh incident ...
#   pnpm admin:grant <メールアドレス> [--revoke]  → bash scripts/go-cli.sh grant-admin ...
#   pnpm db:migrate                               → bash scripts/go-cli.sh migrate
# .env（と .env.worktree）の DATABASE_URL に繋ぐ。.env が無い CI（E2E の db:migrate）では、環境変数をそのまま使う。
# 本番では EC2 の Go のコンテナの中で（migrate はデプロイが1回きりのコンテナで流す）
# `sudo docker exec juken-map-go /api incident ...` と実行する（docs/incident-response.md）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
set -a
# shellcheck disable=SC1091
[ -f "$ROOT/.env" ] && source "$ROOT/.env"
# shellcheck disable=SC1091
[ -f "$ROOT/.env.worktree" ] && source "$ROOT/.env.worktree"
set +a
# migrate が当てる SQL の置き場。本番のイメージは /migrations に入れている（apps/api/Dockerfile）。
export MIGRATIONS_DIR="${MIGRATIONS_DIR:-$ROOT/db/migrations}"

cd "$ROOT/apps/api"
exec go run . "$@"
