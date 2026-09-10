import { defineConfig, devices } from "@playwright/test";

// E2E は「記録→可視化」の毎日ループとデモ閲覧専用を検証する。
// ローカル: dev サーバ（＋ローカル Docker MySQL）に対して実行する。
// CI: 使い捨て MySQL にマイグレーション/seed を流し、ビルド済みアプリを
//     `next start` で起動して実行する（docker compose は使わない）。
//
// SPA 移行中の暫定運用: E2E_BASE_URL を渡すと、その URL に対して実行する。
// apps/web（Vite）と apps/api（Fastify）は別々に起動しておく必要があるため、
// このときは webServer を立てずに既存のサーバへつなぐ。
//   例: E2E_BASE_URL=http://localhost:5173 npm run e2e
const isCI = !!process.env.CI;
const externalBaseURL = process.env.E2E_BASE_URL;
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
    baseURL: externalBaseURL ?? "http://localhost:3000",
    trace: "on-first-retry",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: externalBaseURL
    ? undefined
    : {
        // 本番と同じ構成（Fastify が API と SPA の両方を配る）を 3000 番で起動する。
        // apps/web のビルドを含むので、初回は少し時間がかかる。
        command: "bash scripts/e2e-server.sh",
        url: "http://localhost:3000",
        reuseExistingServer: !isCI,
        timeout: 180_000,
      },
});
