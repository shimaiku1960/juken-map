import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath, URL } from "node:url";
import { parseEnv } from "node:util";

const repoRoot = fileURLToPath(new URL("../../", import.meta.url));

// git worktree では、ほかのチェックアウトとポートがぶつからないよう
// scripts/worktree-new.sh が .env.worktree に別のポートを書く。本体のチェックアウトには
// このファイルが無いので、既定の 5173 / 4200 になる。決め方は scripts/local-ports.sh と同じ。
const worktreeEnvPath = `${repoRoot}.env.worktree`;
const worktreeEnv = existsSync(worktreeEnvPath)
  ? parseEnv(readFileSync(worktreeEnvPath, "utf8"))
  : {};
const webPort = Number(process.env.WEB_PORT ?? worktreeEnv.WEB_PORT ?? 5173);
const slot = Number(worktreeEnv.WT_SLOT ?? 0);
// /api の送り先は nginx（pnpm dev の dev:proxy）。本番と同じ振り分けで Go へ送る（JUK-96・JUK-109）。
const proxyPort = Number(process.env.PROXY_PORT ?? worktreeEnv.PROXY_PORT ?? 4200 + slot);

export default defineConfig(({ isSsrBuild }) => ({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      // Next.js 側と同じ書き味を保つ。@/shared は apps/api とも共有する。
      "@/web": fileURLToPath(new URL("./src", import.meta.url)),
      "@/shared": `${repoRoot}src/shared`,
      "@": repoRoot.replace(/\/$/, ""),
    },
  },
  ssr: {
    // `vite build --ssr`（entry-server.tsx）の出力に、react-dom などの依存も同梱する。
    // ビルドの最後に scripts/prerender.mjs がこの出力を読み込んで SSG する。依存を外に出したままだと、
    // 読み込むときに node_modules の解決に頼ることになる。
    noExternal: true,
  },
  // SSR の出力（dist-server）は SSG のときに読み込むだけなので、public/ の画像などは複製しない。
  // 拡張子は .mjs にし、置き場所の package.json の "type" に関係なく ESM として読ませる
  // （以前は本番のサーバーが "type" の無い /app から読み込んでいた。今は SSG のときだけ読む）。
  build: isSsrBuild
    ? {
        copyPublicDir: false,
        rolldownOptions: {
          output: {
            entryFileNames: "[name].mjs",
            chunkFileNames: "assets/[name]-[hash].mjs",
          },
        },
      }
    : undefined,
  server: {
    port: webPort,
    // 使用中なら別の番号へずらさずに止める。ずれると Go の WEB_ORIGIN（メールのリンク・外部ログインの戻り先）と
    // 食い違う。
    strictPort: true,
    // 開発中は API を別プロセス（nginx の :4200 → Node :4000・Go :4100）で動かす。同一オリジンに見せることで
    // 本番（nginx の後ろに Go）と同じ Cookie の扱いになる。
    // 別オリジンにすると SameSite=None; Secure が必要になり、本番と条件がずれる。
    proxy: {
      "/api": {
        target: `http://127.0.0.1:${proxyPort}`,
        changeOrigin: false,
      },
    },
  },
}));
