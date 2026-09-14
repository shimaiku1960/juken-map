import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";
import { testDatabaseUrl } from "./src/test-db/config.ts";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

// マイグレーションを空の DB に全部当てるテストは、他のテストファイルと同時に走ると
// 同じ MySQL を取り合って数倍遅くなり、CI で制限時間（5秒）を超えることがあった。
// このファイルだけ別のプロジェクトに分け、他が全部終わってから単独で走らせる。
const MIGRATIONS_TEST = "src/infra/migrations.test.ts";

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
    // include / exclude は各プロジェクトに書く（ここに書くと extends で配列が足し合わされる）。
    projects: [
      {
        extends: true,
        test: {
          name: "api",
          include: ["src/**/*.test.ts"],
          exclude: ["**/node_modules/**", "src/generated/**", MIGRATIONS_TEST],
        },
      },
      {
        extends: true,
        test: {
          name: "migrations",
          include: [MIGRATIONS_TEST],
          // 番号の小さいグループから順に走る。api（0）が終わってから始まる。
          sequence: { groupOrder: 1 },
        },
      },
    ],
    // SQL は本物の MySQL に流して確かめる（モックでは SQL の誤りを検出できない）。
    // テスト用 DB の作成とマイグレーションは globalSetup が1回だけ行う。
    globalSetup: ["src/test-db/global-setup.ts"],
    env: {
      DATABASE_URL: testDatabaseUrl,
      // 開発機（Mac）と同じ JST で動かす。CI の既定は UTC で、プロセスの時間帯と
      // DB の時間帯が一致していると、日時の読み書きのずれがあっても表に出ないため。
      TZ: "Asia/Tokyo",
    },
  },
});
