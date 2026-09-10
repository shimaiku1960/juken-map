import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
  },
  test: {
    environment: "node",
    // 単体/結合テストは *.test.ts のみ。E2E（e2e/ 配下の *.spec.ts）は Playwright が担当するため除外。
    include: ["**/*.test.{ts,tsx}"],
    // node_modules は glob で除外する。"node_modules" だけだとルート直下しか効かず、
    // apps/api/node_modules 配下の依存パッケージのテストまで拾ってしまう。
    // apps/ は独自の tsconfig を持つ別パッケージなので、テストもそちら側で実行する。
    exclude: [
      "**/node_modules/**",
      ".next",
      "e2e",
      "src/backend/generated",
      "apps",
    ],
  },
});