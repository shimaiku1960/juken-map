// マイグレーションを当てるコマンド。本番はデプロイがアプリの起動前に1回きりのコンテナで流し
// （.github/scripts/deploy-ec2.sh）、ローカルは `pnpm dev` / `pnpm run db:migrate`、CI は E2E の前に流す。
//
// 本番のアプリのユーザーはテーブルを作れない（infra/dbUsers.ts）ので、テーブル定義を変えられる
// ユーザーを MIGRATION_DATABASE_URL で渡す。無ければ DATABASE_URL で当てる（ローカル・CI）。
import { applyMigrations } from "./infra/migrations.ts";

const applied = await applyMigrations({
  databaseUrl: process.env.MIGRATION_DATABASE_URL || process.env.DATABASE_URL,
});
console.log(
  applied.length > 0
    ? `マイグレーションを${applied.length}本当てました`
    : "当てるマイグレーションはありません（最新です）"
);
