import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";
import { testDatabaseUrl } from "./src/test-db/config.ts";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

export default defineConfig({
  resolve: {
    // tsconfig の paths と同じ対応。テストからも @/backend などで辿れるようにする。
    alias: {
      "@/api": fileURLToPath(new URL("./src", import.meta.url)),
      "@/shared": `${repoRoot}src/shared`,
      "@": repoRoot.replace(/\/$/, ""),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
    // ファイルを同時に走らせない。本物の MySQL を共有しており、別々のファイルが同じ親を参照する行を
    // 同時に INSERT すると、InnoDB のロックがぶつかって `ER_LOCK_DEADLOCK` で落ちた（2026-09-20 の CI）。
    fileParallelism: false,
    // SQL は本物の MySQL に流して確かめる（モックでは SQL の誤りを検出できない）。
    // テスト用 DB の作成とマイグレーション（Go の migrate、JUK-125）は globalSetup が1回だけ行う。
    globalSetup: ["src/test-db/global-setup.ts"],
    env: {
      DATABASE_URL: testDatabaseUrl,
      // 開発機（Mac）と同じ JST で動かす。CI の既定は UTC で、プロセスの時間帯と
      // DB の時間帯が一致していると、日時の読み書きのずれがあっても表に出ないため。
      TZ: "Asia/Tokyo",
    },
  },
});
