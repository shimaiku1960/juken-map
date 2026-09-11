// マイグレーションを当てるコマンド。本番はコンテナの起動時（docker-entrypoint.sh）、
// ローカルは `pnpm dev` / `pnpm run db:migrate`、CI は E2E の前に流す。
import { applyMigrations } from "./infra/migrations.ts";

const applied = await applyMigrations({ databaseUrl: process.env.DATABASE_URL });
console.log(
  applied.length > 0
    ? `マイグレーションを${applied.length}本当てました`
    : "当てるマイグレーションはありません（最新です）"
);
