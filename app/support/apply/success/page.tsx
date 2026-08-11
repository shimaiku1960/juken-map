import type { Metadata } from "next";
import { headers } from "next/headers";
import Link from "next/link";
import { CheckCircle2, Clock3 } from "lucide-react";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { NOINDEX } from "@/lib/site";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { reconcileSupportCheckoutSession } from "@/lib/support-subscription";

export const metadata: Metadata = {
  title: "無料体験のお申し込み完了｜受験マップ",
  robots: NOINDEX,
};

export default async function SupportApplySuccessPage({
  searchParams,
}: {
  searchParams: Promise<{ session_id?: string }>;
}) {
  const session = await auth.api.getSession({ headers: await headers() });
  const { session_id: checkoutSessionId } = await searchParams;
  const invitation =
    session && checkoutSessionId
      ? await prisma.supportCheckoutInvitation.findFirst({
          where: { checkoutSessionId, userId: session.user.id },
        })
      : null;
  let subscription = session
    ? await prisma.supportSubscription.findUnique({
        where: { userId: session.user.id },
      })
    : null;

  if (invitation && session && checkoutSessionId && !subscription) {
    try {
      await reconcileSupportCheckoutSession(checkoutSessionId, session.user.id);
      subscription = await prisma.supportSubscription.findUnique({
        where: { userId: session.user.id },
      });
    } catch (error) {
      console.error("Failed to reconcile support checkout", error);
    }
  }

  if (!invitation) {
    return (
      <main className="mx-auto max-w-2xl px-4 py-16 text-center">
        <h1 className="text-2xl font-bold">申込情報を確認できませんでした</h1>
        <p className="mt-4 text-muted-foreground">ログイン状態をご確認のうえ、運営者へお問い合わせください。</p>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-2xl px-4 py-16 text-center">
      {subscription ? (
        <CheckCircle2 aria-hidden="true" className="mx-auto size-12 text-primary" />
      ) : (
        <Clock3 aria-hidden="true" className="mx-auto size-12 text-primary" />
      )}
      <h1 className="mt-5 text-3xl font-bold">
        {subscription ? "7日間の無料体験が始まりました" : "お申し込みを受け付けました"}
      </h1>
      <p className="mt-4 leading-7 text-muted-foreground">
        {subscription
          ? "無料体験終了後は、解約されない限り月額1,980円（税込）で自動更新されます。"
          : "契約状態を確認しています。反映まで少し時間がかかる場合があります。"}
      </p>
      <Link href="/dashboard" className={cn(buttonVariants({ size: "lg" }), "mt-8 min-h-11")}>
        ダッシュボードへ
      </Link>
    </main>
  );
}
