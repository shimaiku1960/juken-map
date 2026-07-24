"use client";

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  CalendarDays,
  ChevronDown,
  LogOut,
  Menu,
  Newspaper,
  Play,
  Search,
  Target,
  UserRound,
} from "lucide-react";
import { logout } from "@/app/auth/actions";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type HeaderUser = {
  name: string;
  email: string;
};

type HeaderNavigationProps = {
  user: HeaderUser | null;
};

const primaryLinks = [
  { href: "/#study-start", activeHref: "/", label: "学習", mobileLabel: "学習開始", icon: Play },
  {
    href: "/dashboard",
    label: "記録・予定",
    mobileLabel: "カレンダー",
    icon: CalendarDays,
  },
  { href: "/goals", label: "志望校", icon: Target },
] as const;

const isActivePath = (pathname: string, href: string) =>
  href === "/"
    ? pathname === href
    : pathname === href || pathname.startsWith(`${href}/`);

const initialsFor = (user: HeaderUser) => {
  const source = user.name.trim() || user.email;
  return source.slice(0, 1).toUpperCase();
};

const subscribeToHydration = () => () => {};

const HeaderNavigation = ({ user }: HeaderNavigationProps) => {
  const pathname = usePathname();
  const [menuOpen, setMenuOpen] = useState(false);
  const mounted = useSyncExternalStore(
    subscribeToHydration,
    () => true,
    () => false
  );
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;

    const closeOnOutsideClick = (event: PointerEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setMenuOpen(false);
    };

    document.addEventListener("pointerdown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [menuOpen]);

  if (!user) {
    return (
      <nav aria-label="公開ページ" className="flex items-center gap-2">
        <Link
          href="/blog"
          className="hidden rounded-lg px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:inline-flex"
        >
          ブログ
        </Link>
        <Link href="/login" className={buttonVariants({ size: "lg" })}>
          ログイン
        </Link>
      </nav>
    );
  }

  return (
    <>
      <nav aria-label="メインナビゲーション" className="hidden items-center gap-1 md:flex">
        {primaryLinks.map(({ href, label, ...link }) => {
          const active = isActivePath(pathname, "activeHref" in link ? link.activeHref : href);
          return (
            <Link
              key={href}
              href={href}
              aria-current={active ? "page" : undefined}
              className={cn(
                "rounded-lg px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                active
                  ? "bg-muted text-foreground"
                  : "text-muted-foreground hover:bg-muted/70 hover:text-foreground"
              )}
            >
              {label}
            </Link>
          );
        })}

        <Link
          href="/#study-start"
          className={cn(buttonVariants({ size: "lg" }), "ml-2 h-11 gap-2 px-4")}
        >
          <Play aria-hidden="true" className="size-4 fill-current" />
          学習を始める
        </Link>
      </nav>

      <div ref={menuRef} className="relative">
        <button
          type="button"
          aria-expanded={menuOpen}
          aria-haspopup="true"
          onClick={() => setMenuOpen((open) => !open)}
          className="flex min-h-11 items-center gap-2 rounded-xl px-1.5 py-1 transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <span className="flex size-9 items-center justify-center rounded-full bg-secondary text-sm font-semibold text-secondary-foreground">
            {initialsFor(user)}
          </span>
          <span className="hidden max-w-28 truncate text-sm font-medium lg:block">
            {user.name}
          </span>
          <ChevronDown
            aria-hidden="true"
            className={cn(
              "hidden size-4 text-muted-foreground transition-transform sm:block",
              menuOpen && "rotate-180"
            )}
          />
          <span className="sr-only">ユーザーメニュー</span>
        </button>

        {menuOpen ? (
          <div className="absolute right-0 top-[calc(100%+0.5rem)] z-50 hidden w-64 rounded-xl border bg-popover p-2 text-popover-foreground shadow-lg md:block">
            <div className="border-b px-3 py-2">
              <p className="truncate text-sm font-medium">{user.name}</p>
              <p className="truncate text-xs text-muted-foreground">
                {user.email}
              </p>
            </div>
            <div className="py-1">
              <MenuLink
                href="/explore"
                icon={Search}
                onNavigate={() => setMenuOpen(false)}
              >
                大学を探す
              </MenuLink>
              <MenuLink
                href="/profile"
                icon={UserRound}
                onNavigate={() => setMenuOpen(false)}
              >
                プロフィール
              </MenuLink>
              <MenuLink
                href="/blog"
                icon={Newspaper}
                onNavigate={() => setMenuOpen(false)}
              >
                ブログ
              </MenuLink>
            </div>
            <form action={logout} className="border-t pt-1">
              <button
                type="submit"
                className="flex min-h-11 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <LogOut aria-hidden="true" className="size-4" />
                ログアウト
              </button>
            </form>
          </div>
        ) : null}
      </div>

      {mounted
        ? createPortal(
            <>
              <nav
                aria-label="モバイルナビゲーション"
                data-mobile-bottom-nav
                className="fixed inset-x-0 bottom-0 z-40 grid h-[calc(4rem+env(safe-area-inset-bottom))] grid-cols-4 border-t bg-background/95 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden"
              >
                {primaryLinks.map(({ href, label, icon: Icon, ...link }) => {
                  const active = isActivePath(
                    pathname,
                    "activeHref" in link ? link.activeHref : href
                  );
                  return (
                    <Link
                      key={href}
                      href={href}
                      aria-current={active ? "page" : undefined}
                      className={cn(
                        "flex min-w-0 flex-col items-center justify-center gap-1 px-1 text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                        active
                          ? "text-foreground"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      <span
                        className={cn(
                          "rounded-full px-4 py-1",
                          active && "bg-primary/12 text-primary"
                        )}
                      >
                        <Icon aria-hidden="true" className="size-5" />
                      </span>
                      <span className="truncate">
                        {"mobileLabel" in link ? link.mobileLabel : label}
                      </span>
                    </Link>
                  );
                })}
                <button
                  type="button"
                  aria-expanded={menuOpen}
                  aria-controls="mobile-user-menu"
                  onPointerDown={(event) => event.stopPropagation()}
                  onClick={() => setMenuOpen((open) => !open)}
                  className={cn(
                    "flex min-w-0 flex-col items-center justify-center gap-1 px-1 text-[11px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                    menuOpen
                      ? "text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <span
                    className={cn(
                      "rounded-full px-4 py-1",
                      menuOpen && "bg-primary/12 text-primary"
                    )}
                  >
                    <Menu aria-hidden="true" className="size-5" />
                  </span>
                  <span>メニュー</span>
                </button>
              </nav>

              {menuOpen ? (
                <div
                  id="mobile-user-menu"
                  onPointerDown={(event) => event.stopPropagation()}
                  className="fixed inset-x-3 bottom-[calc(4rem+env(safe-area-inset-bottom)+0.75rem)] z-50 rounded-xl border bg-popover p-2 text-popover-foreground shadow-lg md:hidden"
                >
                  <div className="border-b px-3 py-2">
                    <p className="truncate text-sm font-medium">{user.name}</p>
                    <p className="truncate text-xs text-muted-foreground">
                      {user.email}
                    </p>
                  </div>
                  <div className="py-1">
                    <MenuLink
                      href="/explore"
                      icon={Search}
                      onNavigate={() => setMenuOpen(false)}
                    >
                      大学を探す
                    </MenuLink>
                    <MenuLink
                      href="/profile"
                      icon={UserRound}
                      onNavigate={() => setMenuOpen(false)}
                    >
                      プロフィール
                    </MenuLink>
                    <MenuLink
                      href="/blog"
                      icon={Newspaper}
                      onNavigate={() => setMenuOpen(false)}
                    >
                      ブログ
                    </MenuLink>
                  </div>
                  <form action={logout} className="border-t pt-1">
                    <button
                      type="submit"
                      className="flex min-h-11 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <LogOut aria-hidden="true" className="size-4" />
                      ログアウト
                    </button>
                  </form>
                </div>
              ) : null}
            </>,
            document.body
          )
        : null}
    </>
  );
};

type MenuLinkProps = {
  href: string;
  icon: typeof Search;
  onNavigate: () => void;
  children: React.ReactNode;
};

const MenuLink = ({ href, icon: Icon, onNavigate, children }: MenuLinkProps) => (
  <Link
    href={href}
    onClick={onNavigate}
    className="flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm font-medium transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
  >
    <Icon aria-hidden="true" className="size-4 text-muted-foreground" />
    {children}
  </Link>
);

export default HeaderNavigation;
