import { defineConfig, devices } from "@playwright/test";

// デプロイ後に本番へ流す E2E（JUK-101）。deploy.yml の production-e2e ジョブが実行する。
//
// e2e/ の設定（playwright.config.ts）は、テスト用の DB にデータを入れ（globalSetup）、
// 手元にサーバーを立ててから走る。本番にはどちらも使えないので、設定もテストも分ける。
// 本番のデータは書き換えない。デモ（閲覧専用）で入り、画面を開くだけにする。
//
//   pnpm exec playwright test -c playwright.production.config.ts
//   EXPECTED_COMMIT=<SHA> を渡すと、/api/health のコミットがそれと一致するかも確かめる
export default defineConfig({
  testDir: "./e2e-production",
  timeout: 30_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  // 本番までの通信のゆらぎに備えて1回だけ再試行する。
  retries: 1,
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
  use: {
    baseURL: process.env.PRODUCTION_URL ?? "https://juken-map.com",
    trace: "on-first-retry",
    timezoneId: "Asia/Tokyo",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
