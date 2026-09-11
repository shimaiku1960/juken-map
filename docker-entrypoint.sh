#!/bin/sh
set -e

# tsconfig の paths が apps/api を基準にしているため、
# マイグレーションもサーバー起動もこのディレクトリから行う。
cd /app/apps/api

# まだ当てていないマイグレーションを当てる（src/infra/migrations.ts）。
# 失敗したら set -e でここで止まり、サーバーは起動しない（デプロイのスモークテストが落ちて前のイメージへ戻る）。
./node_modules/.bin/tsx src/migrate.ts

exec ./node_modules/.bin/tsx src/server.ts
