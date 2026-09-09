import { createAuthClient } from "better-auth/react";

// Next.js 版と同じく baseURL を渡さない。SPA は API と同一オリジンで配信するため
// （開発は Vite の proxy、本番は nginx のパス振り分け）、相対パスで /api/auth へ届く。
// 別オリジンにすると Cookie が SameSite=None; Secure 必須になり、本番と条件がずれる。
export const authClient = createAuthClient();

export const useSession = authClient.useSession;
