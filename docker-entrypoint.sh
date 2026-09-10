#!/bin/sh
set -e

# prisma.config.ts と tsconfig の paths がどちらも apps/api を基準にしているため、
# マイグレーションもサーバー起動もこのディレクトリから行う。
cd /app/apps/api

node ./node_modules/prisma/build/index.js migrate deploy

exec ./node_modules/.bin/tsx src/server.ts
