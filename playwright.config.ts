import { defineConfig, devices } from "@playwright/test";

// E2E は「記録→可視化」の毎日ループとデモ閲覧専用を検証する。
// ローカル: dev サーバ（＋ローカル Docker MySQL）に対して実行する。
// CI: 使い捨て MySQL にマイグレーション/seed を流し、ビルド済みアプリを
//     `next start` で起動して実行する（docker compose は使わない）。
const isCI = !!process.env.CI;
export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1, // 共有DB・共有dev サーバに対して直列実行して決定的にする
  retries: isCI ? 1 : 0, // CI の一時的なゆらぎに備えて1回だけ再試行
  reporter: isCI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: isCI ? "npx next start" : "npm run dev",
    url: "http://localhost:3000",
    reuseExistingServer: !isCI,
    timeout: 120_000,
  },
});
