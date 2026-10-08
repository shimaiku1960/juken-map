#!/usr/bin/env bash
# 開発でしか使わない Go の道具（apps/api/cmd/devtool）を、.env の DATABASE_URL で動かす（JUK-143）。
# seed・E2E・負荷試験が、ログインと同じ作り方のパスワードのハッシュ・セッション・メールのトークンを作るのに使う。
#   bash scripts/go-devtool.sh hash-password            < パスワード
#   bash scripts/go-devtool.sh email-token <メール> <用途>
#   bash scripts/go-devtool.sh sessions <User-Agent>    < 利用者 ID（1行に1つ）
# 操作の中身は apps/api/internal/devtool/devtool.go。
set -euo pipefail
GO_CLI_CMD=devtool exec bash "$(dirname "${BASH_SOURCE[0]}")/go-cli.sh" "$@"
