import { createAuthClient } from "better-auth/react";
import { twoFactorClient } from "better-auth/client/plugins";

// Next.js 版と同じく baseURL を渡さない。SPA は API と同一オリジンで配信するため
// （開発は Vite の proxy、本番は nginx のパス振り分け）、相対パスで /api/auth へ届く。
// 別オリジンにすると Cookie が SameSite=None; Secure 必須になり、本番と条件がずれる。
//
// twoFactorClient は2段階認証（管理者に必須）の API を足す。ログインでコードが要るときは
// signIn.email の結果に twoFactorRedirect が入るので、ログイン画面がその場でコード入力に切り替える
// （onTwoFactorRedirect で別の画面へ飛ばさない）。
export const authClient = createAuthClient({
  plugins: [twoFactorClient()],
});

export const useSession = authClient.useSession;
