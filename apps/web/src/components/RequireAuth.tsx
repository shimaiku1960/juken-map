import type { ReactNode } from "react";
import { Navigate, useLocation } from "react-router";
import { useSession } from "@/web/lib/auth-client";

/**
 * Next.js では各 page.tsx が `if (!session) redirect("/login")` を書いていた。
 * SPA ではセッション取得が非同期になるため、判定中の状態を挟む必要がある。
 * ここを飛ばすと、ログイン済みでも一瞬ログイン画面が見えてしまう。
 */
export default function RequireAuth({ children }: { children: ReactNode }) {
  const { data: session, isPending } = useSession();
  const location = useLocation();

  if (isPending) return null;

  if (!session) {
    const callbackURL = `${location.pathname}${location.search}${location.hash}`;
    return (
      <Navigate
        to={`/login?callbackURL=${encodeURIComponent(callbackURL)}`}
        replace
      />
    );
  }

  return <>{children}</>;
}
