#!/bin/sh
set -e

# tsconfig の paths が apps/api を基準にしているため、
# マイグレーションもサーバー起動もこのディレクトリから行う。
cd /app/apps/api

# 引数に migrate を付けたときは、マイグレーションだけを当てて終わる。本番のデプロイは、
# テーブル定義を変えられるユーザーでこれを先に流し、アプリのコンテナにはその資格情報を渡さない。
if [ "${1:-}" = "migrate" ]; then
  exec ./node_modules/.bin/tsx src/migrate.ts
fi

# まだ当てていないマイグレーションを当てる（src/infra/migrations.ts）。
# 失敗したら set -e でここで止まり、サーバーは起動しない（デプロイのスモークテストが落ちて前のイメージへ戻る）。
# 本番はデプロイが先に当て終えているので SKIP_MIGRATIONS=1 で飛ばす（アプリのユーザーには当てる権限が無い）。
if [ "${SKIP_MIGRATIONS:-}" != "1" ]; then
  ./node_modules/.bin/tsx src/migrate.ts
fi

# OpenTelemetry（トレース）は、fastify・mysql2・pino が読み込まれるより先に仕込む必要が
# あるため、server.ts の import ではなく --import で先に読ませる（apps/api の dev と同じ）。
# instrumentation.ts は OTEL_EXPORTER_OTLP_ENDPOINT が無ければ何もしないので、
# 送り先を設定していない環境では今まで通り動く。
exec ./node_modules/.bin/tsx --import ./src/instrumentation.ts src/server.ts
