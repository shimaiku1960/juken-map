import Image from "next/image";
import type { Metadata } from "next";
import Link from "next/link";
import {
  ArrowUpRight,
  BookOpenCheck,
  Check,
  CircleSlash,
  Clock3,
  GraduationCap,
  Mail,
  MessageCircleQuestion,
  ShieldCheck,
  Video,
} from "lucide-react";
import {
  AmbientOrb,
  HeroReveal,
  Reveal,
} from "@/app/components/landing/LandingMotion";
import { Button, buttonVariants } from "@/components/ui/button";
import { NOINDEX, SITE_URL } from "@/lib/site";
import {
  SUPPORT_BOOKING_URL,
  SUPPORT_CONTACT_EMAIL,
  SUPPORT_MONTHLY_PRICE_TAX_INCLUDED,
  SUPPORT_TRIAL_DAYS,
  SUPPORT_TRIAL_HOURS,
} from "@/lib/support";
import { cn } from "@/lib/utils";
import brandIcon from "@/public/icon-512.png";

const title = "受験英語のLINE質問サポート｜受験マップ";
const description =
  "受験英語で迷ったとき、早稲田大学 国際教養学部在学の運営者へLINEで質問できる月額サポートです。初回7日間は無料で体験できます。";
const url = SITE_URL + "/support";

export const metadata: Metadata = {
  title,
  description,
  alternates: { canonical: url },
  robots: SUPPORT_BOOKING_URL ? undefined : NOINDEX,
  openGraph: { title, description, type: "website", url },
};

const monthlyPrice = SUPPORT_MONTHLY_PRICE_TAX_INCLUDED.toLocaleString("ja-JP");

const BookingButton = ({
  className,
  variant,
}: {
  className?: string;
  variant?: "secondary";
}) => {
  if (!SUPPORT_BOOKING_URL) {
    return (
      <Button
        disabled
        size="lg"
        variant={variant}
        className={cn("h-12 gap-2 px-6 text-base", className)}
      >
        現在準備中です
      </Button>
    );
  }

  return (
    <a
      href={SUPPORT_BOOKING_URL}
      aria-label="無料面談の空き枠を見る（Googleカレンダーが新しいタブで開きます）"
      target="_blank"
      rel="noopener noreferrer"
      className={cn(
        buttonVariants({ size: "lg", variant }),
        "h-12 gap-2 px-6 text-base shadow-lg shadow-primary/15 transition-[transform,box-shadow,background-color] hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary/20",
        className
      )}
    >
      無料面談の空き枠を見る
      <ArrowUpRight aria-hidden="true" className="size-4" />
    </a>
  );
};

const SectionHeading = ({
  eyebrow,
  heading,
  lead,
}: {
  eyebrow: string;
  heading: string;
  lead?: string;
}) => (
  <Reveal>
    <div className="mx-auto max-w-3xl text-center">
      <p className="inline-flex items-center gap-2 text-sm font-semibold tracking-wide text-primary before:h-px before:w-6 before:bg-primary/50">
        {eyebrow}
      </p>
      <h2 className="mt-4 text-3xl font-bold tracking-[-0.025em] text-balance sm:text-4xl">
        {heading}
      </h2>
      {lead ? (
        <p className="mx-auto mt-5 max-w-2xl text-base leading-8 text-muted-foreground">
          {lead}
        </p>
      ) : null}
    </div>
  </Reveal>
);

const facts = [
  { value: monthlyPrice + "円", label: "月額・税込" },
  { value: SUPPORT_TRIAL_DAYS + "日間", label: "初回無料体験" },
  { value: "上限なし", label: "質問回数" },
  { value: "24時間以内", label: "できる限りの返信目安" },
] as const;

const consultTopics = [
  {
    icon: BookOpenCheck,
    title: "参考書・勉強法",
    body: "いまの自分が何を、どの順番で進めるか",
  },
  {
    icon: MessageCircleQuestion,
    title: "目の前の問題",
    body: "構文や解き方が分からない問題を画像で質問",
  },
  {
    icon: Clock3,
    title: "計画の立て直し",
    body: "遅れた計画から、残り期間の優先順位を整理",
  },
] as const;

const included = [
  "受験英語の勉強法と参考書の選び方",
  "単語・文法・英文解釈・長文・リスニング",
  "個別問題の質問（問題画像も送信できます）",
  "過去問の進め方と志望校帯のレベル感",
  "学習計画・優先順位・遅れの立て直し",
] as const;

const notIncluded = [
  "英語以外の科目の詳しい指導",
  "問題集や過去問の大量な全問解説",
  "大量・継続的な英作文添削",
  "宿題や提出物の代行",
  "即時返信、合格・成績向上の保証",
] as const;

const flowSteps = [
  {
    number: "01",
    title: "空き日時を選ぶ",
    body: "Googleカレンダーで火・木の19:00〜21:00から選びます。",
  },
  {
    number: "02",
    title: "確認メールを受け取る",
    body: "予約後のメールに、当日使うGoogle MeetのURLが届きます。",
  },
  {
    number: "03",
    title: "無料で30分話す",
    body: "悩みと相性を確認します。面談前の決済や、その場での契約はありません。",
  },
  {
    number: "04",
    title: "納得したときだけ始める",
    body: "決済情報を登録してLINE公式アカウントを連携すると、7日間の無料体験が始まります。",
  },
] as const;

const faqs = [
  {
    question: "返信は必ず24時間以内ですか？",
    answer:
      "できる限り24時間以内に返信しますが、保証ではありません。大学の試験期間など対応が難しい期間は、事前にLINEでお知らせします。",
  },
  {
    question: "18歳未満でも利用できますか？",
    answer:
      "利用できますが、保護者など法定代理人の同意が必要です。無料面談の最後に同席いただくか、保護者ご本人から同意フォームを送っていただきます。",
  },
  {
    question: "質問の回数に上限はありますか？",
    answer:
      "上限はありません。ただし大量の問題を丸ごと預けるのではなく、特に分からない問題へ絞って質問してください。",
  },
  {
    question: "無料体験後は自動で料金が発生しますか？",
    answer:
      "はい。無料体験を開始した日時から168時間後に、初回の月額1,980円（税込）が請求されます。無料体験中に解約した場合、料金は発生しません。",
  },
  {
    question: "いつでも解約できますか？",
    answer:
      "いつでも解約できます。無料体験中の解約後は体験終了まで、有料期間中の解約後は支払済み期間の終了まで利用できます。利用者都合の日割り返金はありません。",
  },
  {
    question: "面談の日程を変更・キャンセルしたいときは？",
    answer:
      "原則として開始の24時間前までにご連絡ください。開始から10分お待ちし、参加もご連絡もない場合は欠席として扱います。",
  },
  {
    question: "確認メールが届かないときは？",
    answer:
      "迷惑メールフォルダと、予約時に入力したメールアドレスをご確認ください。見つからない場合はページ下部のメールからお問い合わせください。",
  },
  {
    question: "受験マップのアカウントは必要ですか？",
    answer:
      "必須ではありません。無料の学習記録アプリを併用すると、日々の記録を相談の共通材料として使えます。",
  },
] as const;

const SupportPage = () => (
  <main className="overflow-hidden bg-background">
    <section className="relative isolate border-b">
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_85%_15%,color-mix(in_oklch,var(--primary)_15%,transparent),transparent_34%),linear-gradient(to_bottom,var(--background),color-mix(in_oklch,var(--secondary)_45%,var(--background)))]" />
      <AmbientOrb className="absolute -right-28 top-16 -z-10 size-80 rounded-full bg-primary/10 blur-3xl" />
      <div className="mx-auto grid max-w-6xl items-center gap-12 px-4 py-16 sm:px-6 sm:py-20 lg:grid-cols-[minmax(0,1.05fr)_minmax(22rem,0.95fr)] lg:px-8 lg:py-24">
        <HeroReveal>
          <p className="inline-flex items-center gap-2 rounded-full border border-primary/20 bg-primary/8 px-3 py-1.5 text-sm font-medium text-primary">
            <MessageCircleQuestion aria-hidden="true" className="size-4" />
            受験英語のLINE質問サポート
          </p>
          <h1 className="mt-6 text-4xl font-bold leading-[1.15] tracking-[-0.035em] text-balance sm:text-5xl lg:text-[3.4rem]">
            英語で止まったとき、
            <br />
            <span className="text-primary">次にやることが決まる。</span>
          </h1>
          <p className="mt-6 max-w-xl text-base leading-8 text-muted-foreground sm:text-lg">
            参考書選びから目の前の1問まで。早稲田大学
            国際教養学部在学の受験マップ運営者へ、必要なときにLINEで質問できます。
          </p>
          <div className="mt-8 flex flex-col items-start gap-3">
            <BookingButton className="w-full sm:w-auto" />
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <ShieldCheck aria-hidden="true" className="size-4 text-primary" />
              面談は無料・面談前の決済なし・相談者のカメラは任意
            </p>
            <p className="text-sm text-muted-foreground">
              18歳未満の方は、利用開始前に保護者の同意が必要です
            </p>
            {SUPPORT_BOOKING_URL ? (
              <p className="text-xs text-muted-foreground">
                Googleカレンダーが新しいタブで開きます
              </p>
            ) : (
              <a
                href={"mailto:" + SUPPORT_CONTACT_EMAIL}
                className="inline-flex min-h-11 items-center text-sm font-medium text-primary underline underline-offset-4"
              >
                サービスについて問い合わせる
              </a>
            )}
          </div>
        </HeroReveal>

        <HeroReveal delay={0.12}>
          <div className="rounded-xl border bg-card/95 p-5 shadow-[0_30px_80px_-36px_color-mix(in_oklch,var(--primary)_65%,transparent)] backdrop-blur sm:p-7">
            <div className="flex items-end justify-between gap-4 border-b pb-5">
              <div>
                <p className="text-sm font-semibold text-primary">
                  初回{SUPPORT_TRIAL_DAYS}日間の無料体験
                </p>
                <p className="mt-2 text-4xl font-bold tracking-tight">
                  {monthlyPrice}
                  <span className="ml-1 text-base font-semibold">円／月（税込）</span>
                </p>
              </div>
              <span className="rounded-full bg-primary/10 px-3 py-1 text-xs font-semibold text-primary">
                いつでも解約可
              </span>
            </div>
            <dl className="mt-5 grid gap-px overflow-hidden rounded-lg border bg-border sm:grid-cols-3">
              {facts.slice(1).map((fact) => (
                <div
                  key={fact.label}
                  className="bg-background p-4"
                >
                  <dt className="text-xs text-muted-foreground">{fact.label}</dt>
                  <dd className="mt-1 font-bold">{fact.value}</dd>
                </div>
              ))}
            </dl>
            <p className="mt-4 text-xs leading-5 text-muted-foreground">
              無料体験は開始日時から{SUPPORT_TRIAL_HOURS}
              時間です。終了後は月額料金が自動で請求されます。24時間以内は返信の目安であり、保証ではありません。
            </p>
          </div>
        </HeroReveal>
      </div>
    </section>

    <section className="border-b bg-secondary/25 px-4 py-10 sm:px-6 lg:px-8">
      <div className="mx-auto grid max-w-6xl gap-3 md:grid-cols-3">
        {consultTopics.map(({ icon: Icon, title: topicTitle, body }, index) => (
          <Reveal key={topicTitle} delay={index * 0.06}>
            <div className="flex h-full items-start gap-4 rounded-xl border bg-card p-5 shadow-sm">
              <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Icon aria-hidden="true" className="size-5" />
              </span>
              <div>
                <h2 className="font-bold">{topicTitle}</h2>
                <p className="mt-1 text-sm leading-6 text-muted-foreground">{body}</p>
              </div>
            </div>
          </Reveal>
        ))}
      </div>
    </section>

    <section className="px-4 py-20 sm:px-6 sm:py-24 lg:px-8">
      <div className="mx-auto max-w-6xl">
        <SectionHeading
          eyebrow="相談イメージ"
          heading="答えだけでなく、次に自力で解く手順まで。"
          lead="実際の相談内容ではなく、サポートでの回答イメージです。問題の画像もそのまま送れます。"
        />
        <div className="mt-12 grid items-center gap-10 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] lg:gap-16">
          <Reveal>
            <div className="mx-auto max-w-md rounded-[2rem] border bg-secondary/30 p-3 shadow-[0_28px_70px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)]">
              <div className="overflow-hidden rounded-[1.4rem] border bg-background">
                <div className="flex items-center gap-3 border-b bg-card px-4 py-3">
                  <Image src={brandIcon} alt="" className="size-9 rounded-full" />
                  <div>
                    <p className="text-sm font-bold">
                      受験マップ運営（早稲田の高橋）
                    </p>
                    <p className="text-xs text-muted-foreground">LINE質問サポート</p>
                  </div>
                </div>
                <ol aria-label="相談イメージの会話" className="space-y-5 p-4 sm:p-5">
                  <li className="ml-auto max-w-[85%]">
                    <p className="mb-1 text-right text-xs font-medium text-muted-foreground">
                      相談者
                    </p>
                    <p className="rounded-xl rounded-tr-sm bg-muted px-4 py-3 text-sm leading-6">
                      英文解釈を1周したのに、長文になると構造が取れません。次は何をすればいいですか？
                    </p>
                  </li>
                  <li className="max-w-[90%]">
                    <p className="mb-1 text-xs font-medium text-primary">
                      受験マップ運営
                    </p>
                    <p className="rounded-xl rounded-tl-sm bg-primary/10 px-4 py-3 text-sm leading-6">
                      まず「解釈の知識がない」のか、「長文の中で使えていない」のかを分けましょう。今日読めなかった1文を送ってください。
                    </p>
                  </li>
                  <li className="max-w-[90%]">
                    <p className="mb-1 text-xs font-medium text-primary">
                      受験マップ運営
                    </p>
                    <p className="rounded-xl rounded-tl-sm bg-primary/10 px-4 py-3 text-sm leading-6">
                      主語・動詞を自分で印をつける → 解説と比較する → 翌日に同じ文を印なしで読む、の順で確認します。結果を見て次の練習量を決めましょう。
                    </p>
                  </li>
                </ol>
              </div>
            </div>
          </Reveal>
          <Reveal delay={0.08}>
            <div>
              <p className="text-sm font-semibold text-primary">一問一答で終わらせない</p>
              <h3 className="mt-3 text-2xl font-bold tracking-tight sm:text-3xl">
                「なぜ止まったか」を切り分けて、次の行動へつなげます。
              </h3>
              <p className="mt-5 leading-8 text-muted-foreground">
                正解だけを返すのではなく、どこで判断が止まったかを確認します。同じ形が出たときに、自分で進めるための練習方法まで一緒に整理します。
              </p>
              <ul className="mt-7 grid gap-3 sm:grid-cols-2">
                {["問題画像で質問OK", "勉強法・教材も相談", "計画の遅れも整理", "必要なときに送信"].map(
                  (item) => (
                    <li key={item} className="flex items-center gap-2 text-sm font-medium">
                      <Check aria-hidden="true" className="size-4 text-primary" />
                      {item}
                    </li>
                  )
                )}
              </ul>
            </div>
          </Reveal>
        </div>
      </div>
    </section>

    <section className="border-y bg-secondary/30 px-4 py-20 sm:px-6 sm:py-24 lg:px-8">
      <div className="mx-auto grid max-w-5xl items-center gap-10 lg:grid-cols-[auto_1fr] lg:gap-14">
        <Reveal>
          <div className="mx-auto flex size-36 items-center justify-center rounded-xl border bg-card p-4 shadow-sm lg:size-44">
            <Image
              src={brandIcon}
              alt="受験マップ"
              className="size-full rounded-lg object-cover"
            />
          </div>
        </Reveal>
        <Reveal delay={0.08}>
          <p className="text-sm font-semibold text-primary">相談相手について</p>
          <h2 className="mt-3 text-3xl font-bold tracking-tight">
            受験マップ運営（早稲田の高橋）
          </h2>
          <p className="mt-2 flex items-center gap-2 font-semibold">
            <GraduationCap aria-hidden="true" className="size-5 text-primary" />
            早稲田大学 国際教養学部在学
          </p>
          <p className="mt-5 max-w-2xl leading-8 text-muted-foreground">
            自身の受験で、参考書選びや学習順序を調べ続けた経験から、受験生が迷う時間を減らすためにこのサポートを用意しました。無料面談では運営者がカメラをONにし、実際に話してから相性を判断できます。
          </p>
          <a
            href={"mailto:" + SUPPORT_CONTACT_EMAIL}
            className="mt-6 inline-flex min-h-11 items-center gap-2 rounded-lg px-3 text-sm font-medium text-primary hover:bg-primary/10"
          >
            <Mail aria-hidden="true" className="size-4" />
            メールで問い合わせる
          </a>
        </Reveal>
      </div>
    </section>

    <section className="px-4 py-20 sm:px-6 sm:py-24 lg:px-8">
      <div className="mx-auto max-w-5xl">
        <SectionHeading
          eyebrow="サポートの範囲"
          heading="できることも、できないことも明確に。"
          lead="自分の相談が対象か分からない場合は、無料面談で確認できます。"
        />
        <div className="mt-12 overflow-hidden rounded-xl border bg-card shadow-sm md:grid md:grid-cols-2">
          <Reveal className="p-6 sm:p-8">
            <h3 className="text-lg font-bold">相談できること</h3>
            <ul className="mt-5 space-y-3">
              {included.map((item) => (
                <li key={item} className="flex items-start gap-3">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                    <Check aria-hidden="true" className="size-3" />
                  </span>
                  <span className="text-sm leading-6">{item}</span>
                </li>
              ))}
            </ul>
          </Reveal>
          <Reveal className="border-t bg-muted/30 p-6 sm:p-8 md:border-l md:border-t-0" delay={0.08}>
            <h3 className="text-lg font-bold">対象外のこと</h3>
            <ul className="mt-5 space-y-3">
              {notIncluded.map((item) => (
                <li key={item} className="flex items-start gap-3">
                  <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-background text-muted-foreground">
                    <CircleSlash aria-hidden="true" className="size-3" />
                  </span>
                  <span className="text-sm leading-6 text-muted-foreground">{item}</span>
                </li>
              ))}
            </ul>
          </Reveal>
        </div>
      </div>
    </section>

    <section className="border-y bg-secondary/30 px-4 py-20 sm:px-6 sm:py-24 lg:px-8">
      <div className="mx-auto max-w-6xl">
        <SectionHeading
          eyebrow="無料面談から利用開始まで"
          heading="話して、納得してから決められます。"
        />
        <ol className="mt-12 grid gap-4 md:grid-cols-4">
          {flowSteps.map(({ number, title: stepTitle, body }, index) => (
            <Reveal key={number} className="h-full" delay={index * 0.06}>
              <li className="relative h-full rounded-xl border bg-card p-5 shadow-sm">
                <p className="font-mono text-xs font-bold text-primary">STEP {number}</p>
                <h3 className="mt-3 font-bold">{stepTitle}</h3>
                <p className="mt-2 text-sm leading-6 text-muted-foreground">{body}</p>
              </li>
            </Reveal>
          ))}
        </ol>
        <Reveal className="mt-8 text-center">
          <BookingButton />
          <p className="mt-3 text-xs text-muted-foreground">
            {SUPPORT_BOOKING_URL
              ? "空き枠がない場合は"
              : "受付開始時期など、サービスについてのご質問は"}{" "}
            <a
              href={"mailto:" + SUPPORT_CONTACT_EMAIL}
              className="underline underline-offset-4 hover:text-foreground"
            >
              メールでお問い合わせください
            </a>
          </p>
        </Reveal>
      </div>
    </section>

    <section className="px-4 py-20 sm:px-6 sm:py-24 lg:px-8">
      <div className="mx-auto max-w-3xl">
        <SectionHeading eyebrow="よくある質問" heading="申し込む前に確認できます。" />
        <div className="mt-10 divide-y overflow-hidden rounded-xl border bg-card shadow-sm">
          {faqs.map(({ question, answer }) => (
            <details key={question} className="group p-5 open:bg-muted/20 sm:px-6">
              <summary className="flex min-h-8 cursor-pointer list-none items-center justify-between gap-4 font-bold focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden">
                {question}
                <span
                  aria-hidden="true"
                  className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-lg font-normal transition-transform group-open:rotate-45"
                >
                  +
                </span>
              </summary>
              <p className="pt-4 text-sm leading-7 text-muted-foreground">{answer}</p>
            </details>
          ))}
        </div>
      </div>
    </section>

    <section className="px-4 pb-20 sm:px-6 sm:pb-24 lg:px-8">
      <Reveal className="mx-auto max-w-5xl">
        <div className="relative isolate overflow-hidden rounded-xl bg-primary px-6 py-14 text-center text-primary-foreground shadow-[0_30px_80px_-36px_color-mix(in_oklch,var(--primary)_75%,transparent)] sm:px-12 sm:py-16">
          <div
            aria-hidden="true"
            className="absolute inset-0 -z-10 opacity-20 [background-image:radial-gradient(circle_at_center,var(--primary-foreground)_1px,transparent_1px)] [background-size:24px_24px]"
          />
          <Video aria-hidden="true" className="mx-auto size-7" />
          <h2 className="mt-4 text-3xl font-bold tracking-tight text-balance sm:text-4xl">
            まずは30分、話してみませんか。
          </h2>
          <p className="mx-auto mt-5 max-w-xl leading-7 text-primary-foreground/80">
            面談は無料です。相談内容と相性を確認し、納得した場合にだけ申込方法をご案内します。
          </p>
          <div className="mt-8">
            <BookingButton variant="secondary" />
          </div>
          <div className="mt-6 flex flex-wrap justify-center gap-x-5 gap-y-2 text-sm text-primary-foreground/80">
            <span>面談前の決済なし</span>
            <span>相談者のカメラ任意</span>
            <span>保護者の同席可</span>
          </div>
        </div>
      </Reveal>
    </section>

    <footer className="border-t bg-secondary/20 px-4 py-8 sm:px-6 lg:px-8">
      <div className="mx-auto flex max-w-6xl flex-col gap-5 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
        <p>受験マップ運営（早稲田の高橋）</p>
        <nav aria-label="サポートページの補助情報" className="flex flex-wrap gap-x-5 gap-y-3">
          <Link href="/terms" className="hover:text-foreground">
            利用規約
          </Link>
          <Link href="/support/terms" className="hover:text-foreground">
            有料サポート利用特約
          </Link>
          <Link href="/support/commercial-transactions" className="hover:text-foreground">
            特定商取引法に基づく表記
          </Link>
          <Link href="/privacy" className="hover:text-foreground">
            プライバシーポリシー
          </Link>
          <a href={"mailto:" + SUPPORT_CONTACT_EMAIL} className="hover:text-foreground">
            お問い合わせ
          </a>
        </nav>
      </div>
    </footer>
  </main>
);

export default SupportPage;
