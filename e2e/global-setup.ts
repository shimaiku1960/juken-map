import { execSync } from "node:child_process";
import { loadEnvConfig } from "@next/env";

// E2E 用の通常ユーザーを用意する。Prisma クライアントは ESM のため、
// Playwright 本体ではなく tsx の子プロセス（prisma/seed-e2e.ts）で投入する。
export default function globalSetup() {
  // Next と同じ流儀で .env / .env.local を読み込み、子プロセスへ引き継ぐ
  loadEnvConfig(process.cwd());

  execSync("npx tsx prisma/seed-e2e.ts", {
    stdio: "inherit",
    env: process.env,
  });
}
