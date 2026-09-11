import { execSync } from "node:child_process";

// E2E 用の通常ユーザーを用意する。seed はアプリの接続プール（ESM）を読み込むため、
// Playwright 本体ではなく tsx の子プロセス（prisma/seed-e2e.ts）で投入する。
export default function globalSetup() {
  // ローカルは .env、CI は環境変数だけ。--env-file-if-exists ならどちらでも動く。
  execSync("pnpm exec tsx --env-file-if-exists=.env prisma/seed-e2e.ts", {
    stdio: "inherit",
    env: process.env,
  });
}
