import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    tsconfigPaths: true,
  },
  test: {
    environment: "node",
    // 単体/結合テストは *.test.ts のみ。E2E（e2e/ 配下の *.spec.ts）は Playwright が担当するため除外。
    include: ["**/*.test.{ts,tsx}"],
    exclude: ["node_modules", ".next", "e2e", "app/generated"],
  },
});