#!/usr/bin/env bash
# E2E 用に、本番と同じ構成（Fastify が API と SPA の両方を配る）を 3000 番で起動する。
# Playwright の webServer から呼ばれる。E2E_BASE_URL を指定した場合は使われない。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

pnpm --dir "$ROOT" --filter @juken-map/web build

# apps/api は cwd が apps/api になるので、配信元は絶対パスで渡す。
export WEB_DIST_DIR="$ROOT/apps/web/dist"
# Playwright の baseURL は 3000 番。手元で確認したいときだけ API_PORT で上書きできる。
export API_PORT="${API_PORT:-3000}"
# Better Auth は自分の URL を BETTER_AUTH_URL から読む。.env は開発用に Vite(5173) を
# 指しているため、そのままだと E2E が 3000 番で起動したサーバーに対して origin 不一致で
# 全滅する。ここで起動するポートに合わせて上書きする。
export BETTER_AUTH_URL="http://localhost:${API_PORT}"

exec pnpm --dir "$ROOT" --filter @juken-map/api start
