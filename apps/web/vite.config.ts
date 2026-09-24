import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { parseEnv } from "node:util";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

// git worktree では、ほかのチェックアウトとポートがぶつからないよう
// scripts/worktree-new.sh が .env.worktree に別のポートを書く。本体のチェックアウトには
// このファイルが無いので、既定の 5173 / 4000 になる。
const worktreeEnvPath = `${repoRoot}.env.worktree`;
const worktreeEnv = existsSync(worktreeEnvPath)
  ? parseEnv(readFileSync(worktreeEnvPath, "utf8"))
  : {};
const webPort = Number(process.env.WEB_PORT ?? worktreeEnv.WEB_PORT ?? 5173);
const apiPort = Number(process.env.API_PORT ?? worktreeEnv.API_PORT ?? 4000);

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
    port: webPort,
    // 使用中なら別の番号へずらさずに止める。ずれると BETTER_AUTH_URL と食い違い、
    // ログインが origin 不一致で失敗する。
    strictPort: true,
    // 開発中は API を別プロセス（:4000）で動かす。同一オリジンに見せることで
    // 本番（nginx で /api を Fastify へ振る構成）と同じ Cookie の扱いになる。
    // 別オリジンにすると SameSite=None; Secure が必要になり、本番と条件がずれる。
    proxy: {
      "/api": {
        target: `http://127.0.0.1:${apiPort}`,
        changeOrigin: false,
      },
    },
  },
});
