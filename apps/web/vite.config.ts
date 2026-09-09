import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      // Next.js 側と同じ書き味を保つ。@/shared は apps/api とも共有する。
      "@/web": fileURLToPath(new URL("./src", import.meta.url)),
      "@/shared": `${repoRoot}src/shared`,
      "@": repoRoot.replace(/\/$/, ""),
    },
  },
  server: {
    port: 5173,
    // 開発中は API を別プロセス（:4000）で動かす。同一オリジンに見せることで
    // 本番（nginx で /api を Fastify へ振る構成）と同じ Cookie の扱いになる。
    // 別オリジンにすると SameSite=None; Secure が必要になり、本番と条件がずれる。
    proxy: {
      "/api": {
        target: "http://127.0.0.1:4000",
        changeOrigin: false,
      },
    },
  },
});
