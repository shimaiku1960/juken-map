import type { Metadata } from "next";
import Link from "next/link";
import { ArrowLeft, Check, Clock3, LockKeyhole } from "lucide-react";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { NOINDEX } from "@/lib/site";
import {
  SUPPORT_ANNUAL_PRICE_ESTIMATE,
  SUPPORT_MONTHLY_PRICE_TAX_INCLUDED,
  SUPPORT_TRIAL_HOURS,
} from "@/lib/support";
import { cn } from "@/lib/utils";

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

export default function SupportApplyPage() {
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
          この画面は決済導入前の確認用です。現在は申し込みを確定できず、請求も発生しません。
        </p>
      </header>

      <div className="space-y-6 py-8">
        <div role="status" className="rounded-lg border border-primary/20 bg-primary/8 p-4 text-sm leading-6">
          <p className="flex items-center gap-2 font-semibold text-primary">
            <Clock3 aria-hidden="true" className="size-4" />
            現在、決済と契約管理機能を準備中です
          </p>
          <p className="mt-1 text-muted-foreground">
            本番の申込画面では、無料体験終了日時と初回請求日時を日本時間で表示します。
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
            <label className="flex min-h-11 items-start gap-3 text-sm leading-6 text-muted-foreground">
              <Checkbox disabled aria-label="有料サポート特約と重要条件への同意（準備中）" className="mt-1" />
              <span>
                <Link href="/support/terms" className="font-medium text-primary underline underline-offset-4">
                  有料サポート利用特約
                </Link>
                と上記の重要条件を確認し、同意します。
              </span>
            </label>
            <label className="flex min-h-11 items-start gap-3 text-sm leading-6 text-muted-foreground">
              <Checkbox disabled aria-label="プライバシーポリシーの確認（準備中）" className="mt-1" />
              <span>
                <Link href="/privacy" className="font-medium text-primary underline underline-offset-4">
                  プライバシーポリシー
                </Link>
                を確認しました。
              </span>
            </label>
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

        <Button disabled size="lg" className="h-12 w-full text-base">
          <LockKeyhole aria-hidden="true" />
          現在は申し込めません
        </Button>
        <p className="text-center text-sm text-muted-foreground">
          このボタンを押せるようになるまで、契約や請求は発生しません。
        </p>
      </div>
    </main>
  );
}
