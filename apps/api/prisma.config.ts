import { defineConfig } from "prisma/config";

// リポジトリ直下の prisma.config.ts は dotenv で .env を読むが、こちらは読まない。
// ローカルは tsx の --env-file、本番は docker の --env-file が環境変数を渡すため、
// dotenv を挟むと二重になるうえ apps/api に不要な依存が増える。
// schema と migrations はリポジトリ直下のものを共有する（DB は同じ1つ）。
export default defineConfig({
  schema: "../../prisma/schema.prisma",
  migrations: {
    path: "../../prisma/migrations",
  },
  datasource: {
    url: process.env["DATABASE_URL"],
  },
});
