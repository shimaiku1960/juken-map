import { defineConfig } from "vitest/config";
import { fileURLToPath, URL } from "node:url";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

export default defineConfig({
  resolve: {
    // tsconfig の paths と同じ対応。テストからも @/backend などで辿れるようにする。
    alias: {
      "@/backend": `${repoRoot}src/backend`,
      "@/shared": `${repoRoot}src/shared`,
      "@": repoRoot.replace(/\/$/, ""),
    },
  },
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
