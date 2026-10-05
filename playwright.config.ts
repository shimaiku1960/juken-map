import { existsSync, readFileSync } from "node:fs";
import { parseEnv } from "node:util";
import { defineConfig, devices } from "@playwright/test";

// E2E は「記録→可視化」の毎日ループとデモ閲覧専用を検証する。
// ローカル: dev サーバ（＋ローカル Docker MySQL）に対して実行する。
// CI: 使い捨て MySQL にマイグレーション/seed を流し、ビルド済みアプリを
//     `next start` で起動して実行する（docker compose は使わない）。
//
// SPA 移行中の暫定運用: E2E_BASE_URL を渡すと、その URL に対して実行する。
// apps/web（Vite）と apps/api（Fastify）は別々に起動しておく必要があるため、
// このときは webServer を立てずに既存のサーバへつなぐ。
//   例: E2E_BASE_URL=http://localhost:5173 pnpm run e2e
const isCI = !!process.env.CI;
const externalBaseURL = process.env.E2E_BASE_URL;
// git worktree では scripts/worktree-new.sh が .env.worktree に別のポートを書く。
// 3000 番のままだと reuseExistingServer により、別の worktree が立てたサーバー
// （＝別のブランチのコード）に対してテストしてしまう。
const worktreeEnv = existsSync(".env.worktree")
  ? parseEnv(readFileSync(".env.worktree", "utf8"))
  : {};
const e2ePort = Number(process.env.E2E_PORT ?? worktreeEnv.E2E_PORT ?? 3000);
const e2eURL = `http://localhost:${e2ePort}`;
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
    baseURL: externalBaseURL ?? e2eURL,
    trace: "on-first-retry",
    // 利用者は日本にいて、ブラウザは日本時間で動く。サーバーも「今日」「今月」を日本時間で決めるので、
    // CI（UTC）でもブラウザを日本時間にそろえる。そろえないと、日本時間と UTC で日付・月がずれる
    // 時間帯（日本時間の0時〜9時）に、画面とサーバーの「今月」が食い違う（JUK-87）。
    timezoneId: "Asia/Tokyo",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: externalBaseURL
    ? undefined
    : {
        // 本番と同じ構成（nginx の後ろに Go だけ。Go が API と SPA を配る）を e2ePort（既定 3000）で起動する。
        // apps/web のビルドを含むので、初回は少し時間がかかる。
        command: "bash scripts/e2e-server.sh",
        // nginx はここで待ち受け、内側の Node・Go のポートは scripts/local-ports.sh が決める。
        env: { E2E_PORT: String(e2ePort) },
        // 既定の SIGKILL だと e2e-server.sh の後片付けが動かず、nginx のコンテナが残る（Docker は SIGTERM で止まる）。
        gracefulShutdown: { signal: "SIGTERM", timeout: 10_000 },
        url: e2eURL,
        reuseExistingServer: !isCI,
        timeout: 180_000,
      },
});
