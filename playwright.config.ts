import { defineConfig, devices } from "@playwright/test";

// E2E は「記録→可視化」の毎日ループとデモ閲覧専用を、ローカル dev サーバ
// （＋ローカル Docker MySQL）に対して検証する。CI 併走は将来対応。
export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1, // 共有DB・共有dev サーバに対して直列実行して決定的にする
  reporter: "list",
  use: {
    baseURL: "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "npm run dev",
    url: "http://localhost:3000",
    reuseExistingServer: true,
    timeout: 120_000,
  },
});
