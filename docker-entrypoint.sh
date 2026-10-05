#!/bin/sh
set -e

# まだ当てていないマイグレーション（db/migrations）を当てて終わる（src/infra/migrations.ts）。
# 本番のデプロイは、テーブル定義を変えられるユーザーで `<イメージ> migrate` を1回きりのコンテナで
# 先に流し、アプリ（Go）のコンテナにはその資格情報を渡さない。失敗したら set -e でここで止まる。
# このイメージはサーバーを持たない（JUK-121）ので、migrate 以外の使い方は受け付けない。
if [ "${1:-migrate}" != "migrate" ]; then
  echo "docker-entrypoint.sh: 使えるのは migrate だけです（受け取った引数: $*）" >&2
  exit 64
fi

# tsconfig の paths が apps/api を基準にしているため、このディレクトリから動かす。
cd /app/apps/api
exec ./node_modules/.bin/tsx src/migrate.ts
