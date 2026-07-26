import Link from "next/link";
import { headers } from "next/headers";
import { MapPinned } from "lucide-react";
import { auth } from "@/lib/auth";
import HeaderNavigation from "@/app/components/HeaderNavigation";

const Header = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });
  const user = session?.user;

  return (
    <>
      <header className="sticky top-0 z-40 border-b bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/80">
        <div className="mx-auto flex h-16 w-full max-w-7xl items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
          <Link
            href="/"
            className="flex shrink-0 items-center gap-2 rounded-lg font-bold tracking-tight focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
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
                      user.nickname?.trim() ||
                      user.name.trim() ||
                      user.email,
                    email: user.email,
                  }
                : null
            }
          />
        </div>
      </header>
    </>
  );
};

export default Header;
