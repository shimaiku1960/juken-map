import { Link } from "react-router";
import { MapPinned } from "lucide-react";
import { useSession } from "@/web/lib/auth-client";
import HeaderNavigation from "@/web/components/HeaderNavigation";

// Next.js 版はサーバーでセッションを取り、app/layout.tsx から props で受け取っていた。
// SPA では取得が非同期なので、ここで購読する。
const Header = () => {
  const { data: session } = useSession();
  const user = session?.user;

  return (
    <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
      <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
        <Link
          to="/"
          // アイコンは 36px だが、タップ領域として 44px の高さを確保する。
          className="flex min-h-11 shrink-0 items-center gap-2 rounded-lg font-bold tracking-tight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        >
          <span className="flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground">
            <MapPinned aria-hidden="true" className="size-5" />
          </span>
          <span className="sr-only sm:not-sr-only sm:text-xl">受験マップ</span>
        </Link>

        <HeaderNavigation
          user={
            user
              ? {
                  name:
                    (user as { nickname?: string | null }).nickname?.trim() ||
                    user.name?.trim() ||
                    user.email,
                  email: user.email,
                }
              : null
          }
        />
      </div>
    </header>
  );
};

export default Header;
