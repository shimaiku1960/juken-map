import type { Metadata } from "next";
import Link from "next/link";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import { NOINDEX } from "@/lib/site";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import LineAccountLinkButton from "./LineAccountLinkButton";

export const metadata: Metadata = { robots: NOINDEX };

export default async function LineLinkPage({
  searchParams,
}: {
  searchParams: Promise<{ linkToken?: string }>;
}) {
  const { linkToken } = await searchParams;
  const session = await auth.api.getSession({ headers: await headers() });

  if (!linkToken) {
    return (
      <PageShell className="max-w-lg">
        <PageHeader title="LINE連携" description="連携情報を確認できませんでした。" />
        <InlineFeedback variant="error">LINEで「連携」と送って、届いたリンクからもう一度お試しください。</InlineFeedback>
      </PageShell>
    );
  }

  const callbackURL = `/line/link?linkToken=${encodeURIComponent(linkToken)}`;
  return (
    <PageShell className="max-w-lg">
      <PageHeader title="LINE連携" description="受験マップのアカウントとLINEを安全に接続します。" />
      {session ? (
        <>
          <InlineFeedback variant="info" className="mb-4">
            <p><strong>{session.user.email}</strong> とLINEを連携します。</p>
            <p className="mt-1">連携後、プロフィールで朝・夜のLINE通知を選べます。</p>
          </InlineFeedback>
          <LineAccountLinkButton linkToken={linkToken} />
        </>
      ) : (
        <>
          <InlineFeedback variant="info" className="mb-4">本人確認のため、先に受験マップへログインしてください。</InlineFeedback>
          <Link
            href={`/login?callbackURL=${encodeURIComponent(callbackURL)}`}
            className={cn(buttonVariants({ size: "lg" }), "h-11 w-full")}
          >
            ログインして連携を続ける
          </Link>
        </>
      )}
    </PageShell>
  );
}
