#!/usr/bin/env bash
# 手元で Go のコマンド（apps/api-go の cli.go：incident・grant-admin）を実行する（JUK-122）。
#   pnpm incident <操作> [メールアドレス]        → bash scripts/go-cli.sh incident ...
#   pnpm admin:grant <メールアドレス> [--revoke]  → bash scripts/go-cli.sh grant-admin ...
# .env（と .env.worktree）の DATABASE_URL に繋ぐ。本番では EC2 の Go のコンテナの中で
# `sudo docker exec juken-map-go /api-go incident ...` と実行する（docs/incident-response.md）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
set -a
# shellcheck disable=SC1091
source "$ROOT/.env"
# shellcheck disable=SC1091
[ -f "$ROOT/.env.worktree" ] && source "$ROOT/.env.worktree"
set +a

cd "$ROOT/apps/api-go"
exec go run . "$@"
