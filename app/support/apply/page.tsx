import type { Metadata } from "next";
import Link from "next/link";
import { headers } from "next/headers";
import { ArrowLeft, Check, Clock3 } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import InlineFeedback from "@/app/components/feedback/InlineFeedback";
import CheckoutForm from "@/app/support/apply/CheckoutForm";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { NOINDEX } from "@/lib/site";
import {
  SUPPORT_ANNUAL_PRICE_ESTIMATE,
  SUPPORT_MONTHLY_PRICE_TAX_INCLUDED,
  SUPPORT_TRIAL_HOURS,
} from "@/lib/support";
import { cn } from "@/lib/utils";
import { hashSupportInvitationToken } from "@/lib/support-subscription";

export const metadata: Metadata = {
  title: "お申し込み内容の最終確認｜受験マップ",
  description: "受験英語LINE質問サポートの申込条件を確認する画面です。",
  robots: NOINDEX,
};

const monthlyPrice = SUPPORT_MONTHLY_PRICE_TAX_INCLUDED.toLocaleString("ja-JP");
const annualPrice = SUPPORT_ANNUAL_PRICE_ESTIMATE.toLocaleString("ja-JP");

const SummaryRow = ({ label, children }: { label: string; children: React.ReactNode }) => (
  <div className="grid gap-1 border-b py-4 last:border-b-0 sm:grid-cols-[11rem_1fr] sm:gap-6">
    <dt className="text-sm text-muted-foreground">{label}</dt>
    <dd className="font-medium sm:text-right">{children}</dd>
  </div>
);

export default async function SupportApplyPage({
  searchParams,
}: {
  searchParams: Promise<{ invitation?: string }>;
}) {
  const { invitation: invitationToken } = await searchParams;
  const session = await auth.api.getSession({ headers: await headers() });
  const invitation = invitationToken
    ? await prisma.supportCheckoutInvitation.findUnique({
        where: { tokenHash: hashSupportInvitationToken(invitationToken) },
      })
    : null;
  const invitationIsUsable = Boolean(
    invitation && !invitation.usedAt && invitation.expiresAt > new Date()
  );
  const emailMatches = Boolean(
    session?.user.email &&
      invitation?.email.trim().toLowerCase() === session.user.email.trim().toLowerCase()
  );
  const canStartCheckout = invitationIsUsable && emailMatches && invitationToken;

  return (
    <main className="mx-auto w-full max-w-3xl px-4 py-10 sm:px-8 sm:py-14">
      <Link
        href="/support"
        className={cn(buttonVariants({ variant: "ghost", size: "lg" }), "-ml-2 min-h-11")}
      >
        <ArrowLeft aria-hidden="true" />
        サポート詳細へ戻る
      </Link>

      <header className="mt-6 border-b pb-8">
        <p className="text-sm font-semibold text-primary">契約前の最終確認</p>
        <h1 className="mt-2 text-3xl font-bold tracking-tight sm:text-4xl">
          お申し込み内容をご確認ください
        </h1>
        <p className="mt-4 leading-7 text-muted-foreground">
          条件をご確認のうえ、Stripeの安全な決済画面で支払い方法を登録してください。
          本日の請求額は0円です。
        </p>
      </header>

      <div className="space-y-6 py-8">
        <div role="status" className="rounded-lg border border-primary/20 bg-primary/8 p-4 text-sm leading-6">
          <p className="flex items-center gap-2 font-semibold text-primary">
            <Clock3 aria-hidden="true" className="size-4" />
            申込完了から7日間は無料です
          </p>
          <p className="mt-1 text-muted-foreground">
            無料体験の終了日と初回請求日は、次のStripe決済画面で確定前に表示されます。
          </p>
        </div>

        <Card>
          <CardHeader className="border-b">
            <CardTitle className="text-lg">受験英語LINE質問サポート</CardTitle>
          </CardHeader>
          <CardContent>
            <dl>
              <SummaryRow label="本日の支払い">0円</SummaryRow>
              <SummaryRow label="初回無料体験">申込完了から{SUPPORT_TRIAL_HOURS}時間</SummaryRow>
              <SummaryRow label="無料体験終了・初回請求">申込時に日本時間で表示</SummaryRow>
              <SummaryRow label="無料体験終了後">月額{monthlyPrice}円（税込）で自動更新</SummaryRow>
              <SummaryRow label="1年間の支払額目安">{annualPrice}円（税込・月額料金12回分）</SummaryRow>
              <SummaryRow label="質問回数">数値上の上限なし</SummaryRow>
            </dl>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b">
            <CardTitle>解約・返金と提供条件</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-3 text-sm leading-6 text-muted-foreground">
              {[
                "無料体験中に解約すれば月額料金は発生しません。",
                "いつでも次回更新を停止できます。",
                "有料期間中の解約後も、支払済み期間の終了まで利用できます。",
                "利用者都合による支払済み料金の日割り返金はありません。",
                "返信はできる限り24時間以内を目安としますが、保証ではありません。",
                "合格や成績向上を保証するサービスではありません。",
              ].map((item) => (
                <li key={item} className="flex items-start gap-3">
                  <Check aria-hidden="true" className="mt-1 size-4 shrink-0 text-primary" />
                  <span>{item}</span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="border-b">
            <CardTitle>確認事項</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-sm leading-6 text-muted-foreground">
              下記の有料サポート利用特約、プライバシーポリシー、重要条件を確認後に同意欄が表示されます。
            </p>
            <p className="text-sm text-muted-foreground">
              18歳未満の方は、申込前に法定代理人の同意確認が必要です。
            </p>
          </CardContent>
        </Card>

        <nav aria-label="申込条件の関連文書" className="flex flex-wrap gap-x-5 gap-y-3 text-sm">
          <Link href="/support/commercial-transactions" className="text-primary underline underline-offset-4">
            特定商取引法に基づく表記
          </Link>
          <Link href="/support/terms" className="text-primary underline underline-offset-4">
            有料サポート利用特約
          </Link>
          <Link href="/privacy" className="text-primary underline underline-offset-4">
            プライバシーポリシー
          </Link>
        </nav>

        {canStartCheckout ? (
          <CheckoutForm invitationToken={invitationToken} />
        ) : !session ? (
          <InlineFeedback variant="info">
            <p>面談後に案内されたメールアドレスでログインしてから、この招待URLをもう一度開いてください。</p>
            <Link href="/login" className="mt-2 inline-flex min-h-11 items-center font-medium underline underline-offset-4">
              ログインする
            </Link>
          </InlineFeedback>
        ) : (
          <InlineFeedback variant="error">
            <p>
              招待URLが無効、期限切れ、使用済み、またはログイン中のメールアドレスと一致しません。
              運営者へお問い合わせください。
            </p>
          </InlineFeedback>
        )}
      </div>
    </main>
  );
}
