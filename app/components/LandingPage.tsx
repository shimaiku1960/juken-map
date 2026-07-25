import Link from "next/link";
import {
  ArrowRight,
  BarChart3,
  CalendarDays,
  Check,
  Clock3,
  MapPinned,
  Play,
  Target,
  TimerReset,
} from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  AmbientOrb,
  AnimatedBar,
  HeroReveal,
  Reveal,
} from "@/app/components/landing/LandingMotion";

const signupClassName = cn(
  buttonVariants({ size: "lg" }),
  "h-12 px-6 text-base shadow-lg shadow-primary/15 transition-[transform,box-shadow,background-color] hover:-translate-y-0.5 hover:shadow-xl hover:shadow-primary/20"
);

const SectionHeading = ({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string;
  title: string;
  description?: string;
}) => (
  <Reveal>
    <div className="mx-auto max-w-3xl text-center">
      <p className="inline-flex items-center gap-2 text-sm font-semibold tracking-wide text-primary before:h-px before:w-6 before:bg-primary/50">
        {eyebrow}
      </p>
      <h2 className="mt-4 text-3xl font-bold tracking-[-0.025em] text-balance sm:text-4xl lg:text-[2.75rem] lg:leading-[1.18]">
        {title}
      </h2>
      {description ? (
        <p className="mx-auto mt-5 max-w-2xl text-base leading-8 text-muted-foreground sm:text-lg">
          {description}
        </p>
      ) : null}
    </div>
  </Reveal>
);

const PhoneMockup = () => (
  <figure className="relative mx-auto w-full max-w-[21rem]">
    <div className="rounded-[2.5rem] border-[7px] border-foreground/90 bg-background p-2 shadow-[0_32px_80px_-28px_color-mix(in_oklch,var(--primary)_55%,transparent)]">
    <div className="relative aspect-[9/18.5] overflow-hidden rounded-[1.9rem] bg-gradient-to-b from-primary/8 via-background to-secondary/60 px-5 py-8">
      <div className="mx-auto h-1.5 w-16 rounded-full bg-foreground/15" />
      <div className="flex h-full flex-col items-center justify-center text-center">
        <div className="mb-8 flex size-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-sm">
          <MapPinned aria-hidden="true" className="size-6" />
        </div>
        <p className="text-xs text-muted-foreground">第一志望まで</p>
        <p className="mt-1 text-sm font-medium">
          早稲田大学 人間科学部
          <span className="mx-1 text-lg font-bold">あと198日</span>
        </p>
        <p className="mt-12 text-xl font-bold">今日の学習を始めよう</p>
        <div className="mt-6 flex h-12 items-center gap-2 rounded-lg bg-primary px-6 text-sm font-semibold text-primary-foreground shadow-sm">
          <Play aria-hidden="true" className="size-4 fill-current" />
          学習を始める
        </div>
        <p className="mt-5 text-xs font-medium text-primary">
          今日の予定・記録を見る →
        </p>
      </div>
    </div>
    </div>
    <p className="absolute -right-3 top-20 rounded-full border bg-card/95 px-3 py-1.5 text-xs font-medium shadow-lg backdrop-blur">
      1タップで開始
    </p>
    <figcaption className="sr-only">
      受験マップの学習開始画面イメージ
    </figcaption>
  </figure>
);

const StudyFlowPreview = () => (
  <div className="relative overflow-hidden rounded-xl border bg-card shadow-[0_28px_70px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)]">
    <div className="bg-gradient-to-br from-secondary/80 via-background to-primary/10 p-5 sm:p-8">
      <div className="relative overflow-hidden rounded-lg border bg-background/85 px-5 py-8 sm:px-8 sm:py-10">
        <div
          aria-hidden="true"
          className="absolute inset-0 opacity-40 [background-image:linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [background-size:32px_32px]"
        />
        <div className="relative grid items-center gap-8 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:gap-12">
          <figure className="mx-auto w-full max-w-[18rem]">
            <div className="overflow-hidden rounded-[2.3rem] border-[6px] border-foreground/90 bg-foreground shadow-2xl shadow-primary/20">
              <video
                className="block aspect-[606/1140] w-full bg-background object-cover"
                autoPlay
                loop
                muted
                playsInline
                preload="metadata"
                poster="/landing/study-flow-poster.jpg"
                aria-label="学習内容を選び、タイマーで計測し、実績を保存してカレンダーで振り返る操作例"
              >
                <source src="/landing/study-flow.mp4" type="video/mp4" />
                お使いのブラウザでは動画を再生できません。
              </video>
            </div>
            <figcaption className="mt-3 text-center text-xs font-medium text-muted-foreground">
              実際の操作画面
            </figcaption>
          </figure>

          <div>
            <div className="flex flex-wrap items-end justify-between gap-3">
              <div>
                <p className="text-sm font-semibold text-primary">
                  ひとつながりの学習記録
                </p>
                <p className="mt-1 text-lg font-bold sm:text-xl">
                  始めてから、振り返るまで。
                </p>
              </div>
              <span className="rounded-full border bg-card px-3 py-1 text-xs font-medium text-muted-foreground">
                約13秒
              </span>
            </div>
            <ol className="mt-8 grid gap-3 sm:grid-cols-2">
              {[
                { number: "01", label: "内容を選ぶ", icon: Clock3 },
                { number: "02", label: "時間を計る", icon: TimerReset },
                { number: "03", label: "そのまま保存", icon: Check },
                { number: "04", label: "記録を振り返る", icon: CalendarDays },
              ].map(({ number, label, icon: Icon }) => (
                <li
                  key={number}
                  className="flex items-center gap-4 rounded-lg border bg-card p-4 shadow-sm"
                >
                  <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                    <Icon aria-hidden="true" className="size-5" />
                  </div>
                  <div>
                    <p className="font-mono text-[11px] font-bold text-primary">
                      STEP {number}
                    </p>
                    <p className="mt-1 text-sm font-semibold">{label}</p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
        </div>
      </div>
    </div>
  </div>
);

const CalendarMockup = () => {
  const activity = [0, 1, 2, 0, 3, 1, 2, 3, 2, 0, 1, 3, 4, 2, 3, 1, 2, 4];

  return (
    <div className="rounded-xl border bg-card p-4 shadow-[0_24px_60px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)] sm:p-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="font-semibold">学習カレンダー</p>
          <p className="mt-1 text-xs text-muted-foreground">
            今日の学習時間：2時間15分
          </p>
        </div>
        <span className="rounded-full bg-secondary px-3 py-1 text-xs font-medium text-secondary-foreground">
          8日連続
        </span>
      </div>
      <div className="mt-5 grid grid-cols-7 gap-1.5">
        {activity.map((level, index) => (
          <div
            // Static presentation data; index is stable.
            key={index}
            className={cn(
              "aspect-square rounded-sm border",
              level === 0 && "bg-muted/50",
              level === 1 && "border-primary/10 bg-primary/15",
              level === 2 && "border-primary/15 bg-primary/30",
              level === 3 && "border-primary/20 bg-primary/55",
              level === 4 && "border-primary/30 bg-primary/80"
            )}
          />
        ))}
      </div>
      <div className="mt-5 rounded-lg bg-muted/60 p-3">
        <div className="flex items-center justify-between gap-3">
          <p className="text-sm font-medium">今日の実績</p>
          <p className="text-sm font-bold">2時間15分</p>
        </div>
        <div className="mt-3 space-y-2 text-xs">
          <div className="flex items-center justify-between">
            <span>英語・英単語帳</span>
            <span className="text-muted-foreground">30分</span>
          </div>
          <div className="flex items-center justify-between">
            <span>数学・二次関数</span>
            <span className="text-muted-foreground">60分</span>
          </div>
          <div className="flex items-center justify-between">
            <span>国語・現代文</span>
            <span className="text-muted-foreground">45分</span>
          </div>
        </div>
      </div>
    </div>
  );
};

const SubjectMockup = () => {
  const subjects = [
    { label: "英語", time: "5時間10分", width: "82%" },
    { label: "数学", time: "4時間30分", width: "72%" },
    { label: "国語", time: "2時間30分", width: "40%" },
  ];

  return (
    <div className="rounded-xl border bg-card p-5 shadow-[0_24px_60px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)]">
      <p className="font-semibold">直近7日間の科目別学習時間</p>
      <div className="mt-6 space-y-5">
        {subjects.map((subject, index) => (
          <div key={subject.label}>
            <div className="mb-2 flex items-center justify-between text-sm">
              <span className="font-medium">{subject.label}</span>
              <span className="text-muted-foreground">{subject.time}</span>
            </div>
            <div className="h-2.5 overflow-hidden rounded-full bg-muted">
              <AnimatedBar width={subject.width} delay={index * 0.1} />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

export default function LandingPage() {
  // 公開ランディングページ。未ログインで `/` にアクセスしたときに表示する。
  return (
    <main className="overflow-hidden bg-background">
      <section className="relative isolate border-b">
        <div className="absolute inset-0 -z-10 bg-[radial-gradient(circle_at_80%_20%,color-mix(in_oklch,var(--primary)_16%,transparent),transparent_38%),linear-gradient(to_bottom,var(--background),color-mix(in_oklch,var(--secondary)_48%,var(--background)))]" />
        <div
          aria-hidden="true"
          className="absolute inset-0 -z-10 opacity-35 [background-image:linear-gradient(to_right,color-mix(in_oklch,var(--border)_50%,transparent)_1px,transparent_1px),linear-gradient(to_bottom,color-mix(in_oklch,var(--border)_50%,transparent)_1px,transparent_1px)] [background-size:64px_64px] [mask-image:linear-gradient(to_bottom,black,transparent_85%)]"
        />
        <AmbientOrb className="absolute -right-24 top-16 -z-10 size-72 rounded-full bg-primary/10 blur-3xl" />
        <AmbientOrb
          delay={1.5}
          className="absolute -left-32 bottom-10 -z-10 size-64 rounded-full bg-accent/70 blur-3xl"
        />
        <div className="mx-auto grid min-h-[calc(100svh-4rem)] max-w-7xl items-center gap-14 px-4 py-16 sm:px-6 lg:grid-cols-[1.05fr_0.95fr] lg:px-8 lg:py-20">
          <HeroReveal className="max-w-2xl text-center lg:text-left">
            <p className="inline-flex items-center gap-2 rounded-full border border-primary/20 bg-primary/8 px-3 py-1.5 text-sm font-medium text-primary">
              <TimerReset aria-hidden="true" className="size-4" />
              大学受験生のための学習時間管理
            </p>
            <h1 className="mt-6 text-4xl font-bold leading-[1.15] tracking-[-0.035em] text-balance sm:text-5xl lg:text-[4rem]">
              今日の勉強を、
              <br />
              <span className="text-primary">合格までの積み上がりに。</span>
            </h1>
            <p className="mx-auto mt-7 max-w-xl text-base leading-8 text-muted-foreground sm:text-lg lg:mx-0">
              学習を始めるときにタイマーを押すだけ。
              勉強時間や予定、毎日の積み上がりをひとつのカレンダーで振り返れます。
            </p>
            <div className="mt-8 flex flex-col items-center gap-4 sm:flex-row sm:justify-center lg:justify-start">
              <Link href="/signup" className={signupClassName}>
                無料で始める
                <ArrowRight aria-hidden="true" />
              </Link>
              <Link
                href="/login#demo-login"
                className={cn(
                  buttonVariants({ variant: "link", size: "lg" }),
                  "h-11 text-muted-foreground"
                )}
              >
                登録せずにデモを見る
              </Link>
            </div>
            <p className="mt-4 text-sm text-muted-foreground">
              無料で登録・すぐに始められます
            </p>
          </HeroReveal>
          <HeroReveal className="relative" delay={0.12}>
            <div className="absolute left-1/2 top-1/2 -z-10 size-[26rem] -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/10 blur-3xl" />
            <PhoneMockup />
          </HeroReveal>
        </div>
      </section>

      <section className="px-4 py-24 sm:px-6 sm:py-28 lg:px-8">
        <div className="mx-auto max-w-6xl">
          <SectionHeading
            eyebrow="こんな悩みはありませんか？"
            title="勉強したはずなのに、積み上がりが見えない。"
          />
          <div className="mt-12 grid gap-4 md:grid-cols-3">
            {[
              "勉強時間を後から思い出して記録するのが面倒",
              "予定を立てても、実績との違いが分からない",
              "最近どの科目に時間を使ったか把握できない",
            ].map((problem, index) => (
              <Reveal key={problem} delay={index * 0.08}>
                <div className="group flex h-full gap-4 rounded-xl border bg-card p-6 shadow-sm transition-[transform,box-shadow,border-color] duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-md hover:shadow-primary/5">
                  <span className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-secondary text-primary transition-colors duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] group-hover:bg-primary group-hover:text-primary-foreground">
                    <Check aria-hidden="true" className="size-4" />
                  </span>
                  <p className="leading-7">{problem}</p>
                </div>
              </Reveal>
            ))}
          </div>
          <Reveal delay={0.12}>
            <p className="mt-12 text-center text-lg font-semibold">
              受験マップは、勉強を始める瞬間から記録をつなげます。
            </p>
          </Reveal>
        </div>
      </section>

      <section
        id="how-it-works"
        className="relative border-y bg-secondary/30 px-4 py-24 sm:px-6 sm:py-28 lg:px-8"
      >
        <div className="mx-auto max-w-6xl">
          <SectionHeading
            eyebrow="かんたん記録"
            title="始めるだけで、記録が残る。"
            description="今日の予定を選んでタイマーを開始。勉強が終わったら、そのまま学習実績として保存できます。"
          />
          <Reveal className="mt-12" delay={0.08}>
            <StudyFlowPreview />
          </Reveal>
          <Reveal className="mt-9 text-center" delay={0.12}>
            <Link href="/signup" className={signupClassName}>
              無料で始める
              <ArrowRight aria-hidden="true" />
            </Link>
          </Reveal>
        </div>
      </section>

      <section id="features" className="px-4 py-24 sm:px-6 sm:py-28 lg:px-8">
        <div className="mx-auto max-w-6xl">
          <SectionHeading
            eyebrow="受験マップでできること"
            title="毎日の学習サイクルを、ひとつにつなぐ。"
            description="始める、記録する、振り返る。別々になりがちな行動を、ひとつの流れにまとめます。"
          />

          <Reveal className="mt-20 grid items-center gap-10 lg:grid-cols-2 lg:gap-20">
            <div>
              <div className="flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Clock3 aria-hidden="true" className="size-5" />
              </div>
              <h3 className="mt-5 text-2xl font-bold">すぐに始められる</h3>
              <p className="mt-4 leading-7 text-muted-foreground">
                今日の予定や学習内容を選んで、そのままタイマーを開始。
                過去の学習も時間を選ぶだけで、すばやく記録できます。
              </p>
            </div>
            <div className="rounded-xl border bg-gradient-to-br from-primary/10 to-secondary/70 p-6 shadow-inner">
              <div className="mx-auto max-w-sm rounded-xl border bg-card p-5 shadow-xl shadow-primary/10">
                <p className="text-sm text-muted-foreground">今日の予定</p>
                <div className="mt-3 space-y-2">
                  {["数学IA 二次関数", "英単語帳 1201〜1300", "その他の学習"].map(
                    (item, index) => (
                      <div
                        key={item}
                        className={cn(
                          "flex items-center gap-3 rounded-lg border p-3 text-sm",
                          index === 0 && "border-primary bg-primary/5"
                        )}
                      >
                        <span
                          className={cn(
                            "size-3 rounded-full border",
                            index === 0 && "border-primary bg-primary"
                          )}
                        />
                        {item}
                      </div>
                    )
                  )}
                </div>
                <div className="mt-4 flex h-10 items-center justify-center gap-2 rounded-lg bg-primary text-sm font-medium text-primary-foreground">
                  <Play aria-hidden="true" className="size-4 fill-current" />
                  計測を始める
                </div>
              </div>
            </div>
          </Reveal>

          <Reveal className="mt-28 grid items-center gap-10 lg:grid-cols-2 lg:gap-20">
            <div className="order-2 lg:order-1">
              <CalendarMockup />
            </div>
            <div className="order-1 lg:order-2">
              <div className="flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <CalendarDays aria-hidden="true" className="size-5" />
              </div>
              <h3 className="mt-5 text-2xl font-bold">
                予定と実績がつながる
              </h3>
              <p className="mt-4 leading-7 text-muted-foreground">
                何を勉強する予定だったか、実際にどれだけ取り組んだかを、
                ひとつのカレンダーで確認できます。
              </p>
            </div>
          </Reveal>

          <Reveal className="mt-28 grid items-center gap-10 lg:grid-cols-2 lg:gap-20">
            <div>
              <div className="flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <BarChart3 aria-hidden="true" className="size-5" />
              </div>
              <h3 className="mt-5 text-2xl font-bold">
                科目ごとの積み上がりが分かる
              </h3>
              <p className="mt-4 leading-7 text-muted-foreground">
                学習時間、継続日数、科目ごとの配分を振り返り、
                次に取り組むことを考えられます。
              </p>
            </div>
            <SubjectMockup />
          </Reveal>
        </div>
      </section>

      <section className="relative border-y bg-secondary/30 px-4 py-24 sm:px-6 sm:py-28 lg:px-8">
        <Reveal className="mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-2 lg:gap-20">
          <div>
            <p className="text-sm font-semibold tracking-wide text-primary">
              志望校とのつながり
            </p>
            <h2 className="mt-3 text-3xl font-bold tracking-tight text-balance sm:text-4xl">
              毎日の学習を、
              <br />
              志望校というゴールにつなげる。
            </h2>
            <p className="mt-5 leading-8 text-muted-foreground">
              第一志望までの残り日数や受験日程を確認できます。
              今日の勉強が何のためなのかを忘れずに、日々の学習を積み重ねられます。
            </p>
          </div>
          <div className="rounded-xl border bg-card p-5 shadow-[0_24px_60px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)]">
            <div className="flex flex-wrap items-center gap-3 border-b pb-5">
              <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Target aria-hidden="true" className="size-5" />
              </div>
              <div>
                <p className="text-xs text-muted-foreground">志望校までの予定</p>
                <p className="font-semibold">受験スケジュール</p>
              </div>
              <p className="ml-auto whitespace-nowrap text-sm">
                あと <span className="text-xl font-bold text-primary">198</span> 日
              </p>
            </div>
            <ol className="mt-2 divide-y">
              {[
                {
                  date: "2月9日",
                  dateTime: "2027-02-09",
                  school: "早稲田大学 人間科学部",
                  role: "第一志望",
                  primary: true,
                },
                {
                  date: "2月12日",
                  dateTime: "2027-02-12",
                  school: "併願校 A",
                  role: "併願",
                  primary: false,
                },
                {
                  date: "2月16日",
                  dateTime: "2027-02-16",
                  school: "併願校 B",
                  role: "併願",
                  primary: false,
                },
              ].map(({ date, dateTime, school, role, primary }) => (
                <li
                  key={school}
                  className="grid grid-cols-[4.5rem_1fr] items-center gap-4 py-4 sm:grid-cols-[5rem_1fr_auto]"
                >
                  <time
                    dateTime={dateTime}
                    className="text-sm font-semibold tabular-nums text-foreground"
                  >
                    {date}
                  </time>
                  <p className="min-w-0 text-sm font-medium">{school}</p>
                  <span
                    className={cn(
                      "col-start-2 w-fit rounded-full px-2.5 py-1 text-xs font-medium sm:col-start-auto",
                      primary
                        ? "bg-primary text-primary-foreground"
                        : "bg-secondary text-secondary-foreground"
                    )}
                  >
                    {role}
                  </span>
                </li>
              ))}
            </ol>
          </div>
        </Reveal>
      </section>

      <section className="px-4 py-24 sm:px-6 sm:py-28 lg:px-8">
        <div className="mx-auto max-w-5xl">
          <SectionHeading
            eyebrow="今日から始める"
            title="3ステップですぐに記録できます。"
          />
          <ol className="mt-12 grid gap-5 md:grid-cols-3">
            {[
              ["01", "無料で登録", "メールアドレスなどでアカウントを作成します。"],
              ["02", "学習内容を選ぶ", "今日の予定や、これから勉強する内容を選びます。"],
              ["03", "タイマーを開始", "終了後は、学習時間がそのまま実績に残ります。"],
            ].map(([number, title, description], index) => (
              <li key={number}>
                <Reveal className="h-full" delay={index * 0.08}>
                  <div className="group relative h-full overflow-hidden rounded-xl border bg-card p-6 shadow-sm transition-[transform,box-shadow,border-color] duration-500 ease-[cubic-bezier(0.22,1,0.36,1)] hover:-translate-y-0.5 hover:border-primary/20 hover:shadow-md hover:shadow-primary/5">
                    <p className="relative font-mono text-sm font-bold text-primary">
                      {number}
                    </p>
                    <h3 className="relative mt-4 text-xl font-bold">{title}</h3>
                    <p className="relative mt-3 text-sm leading-7 text-muted-foreground">
                      {description}
                    </p>
                  </div>
                </Reveal>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section className="px-4 pb-24 sm:px-6 sm:pb-28 lg:px-8">
        <Reveal className="mx-auto max-w-6xl">
          <div className="relative isolate overflow-hidden rounded-xl bg-primary px-6 py-16 text-center text-primary-foreground shadow-[0_30px_80px_-36px_color-mix(in_oklch,var(--primary)_75%,transparent)] sm:px-12 sm:py-20">
          <div
            aria-hidden="true"
            className="absolute inset-0 -z-10 opacity-20 [background-image:radial-gradient(circle_at_center,var(--primary-foreground)_1px,transparent_1px)] [background-size:24px_24px]"
          />
          <h2 className="text-3xl font-bold tracking-tight text-balance sm:text-4xl">
            今日の勉強を、記録に残そう。
          </h2>
          <p className="mx-auto mt-5 max-w-2xl leading-7 text-primary-foreground/80">
            学習を始めるところから、毎日の積み上がりを振り返るところまで。
            受験マップで、今日から記録を始められます。
          </p>
          <div className="mt-8">
            <Link
              href="/signup"
              className={cn(
                buttonVariants({ variant: "secondary", size: "lg" }),
                "h-12 px-6 text-base shadow-sm"
              )}
            >
              無料で始める
              <ArrowRight aria-hidden="true" />
            </Link>
          </div>
          <p className="mt-5 text-sm text-primary-foreground/75">
            まず操作を確認したい方は、{" "}
            <Link
              href="/login#demo-login"
              className="font-medium underline underline-offset-4 hover:text-primary-foreground"
            >
              サンプルデータ入りのデモ
            </Link>
            をご利用いただけます。
          </p>
          </div>
        </Reveal>
      </section>

      <footer className="border-t bg-secondary/25 px-4 py-14 sm:px-6 lg:px-8">
        <div className="mx-auto max-w-7xl">
          <div className="grid gap-10 md:grid-cols-[1.5fr_1fr_1fr_1fr]">
            <div className="max-w-xs">
              <div className="flex items-center gap-2 font-bold tracking-tight text-foreground">
                <span className="flex size-9 items-center justify-center rounded-xl bg-primary text-primary-foreground">
                  <MapPinned aria-hidden="true" className="size-5" />
                </span>
                <span className="text-lg">受験マップ</span>
              </div>
              <p className="mt-4 text-sm leading-6 text-muted-foreground">
                今日の勉強を、合格までの積み上がりに。
                大学受験生のための学習時間管理アプリです。
              </p>
            </div>

            {[
              {
                heading: "プロダクト",
                links: [
                  { href: "/#features", label: "できること" },
                  { href: "/#how-it-works", label: "かんたん記録" },
                  { href: "/login#demo-login", label: "デモを見る" },
                ],
              },
              {
                heading: "はじめる",
                links: [
                  { href: "/signup", label: "無料で登録" },
                  { href: "/login", label: "ログイン" },
                ],
              },
              {
                heading: "もっと知る",
                links: [{ href: "/blog", label: "ブログ" }],
              },
            ].map((column) => (
              <nav key={column.heading} aria-label={column.heading}>
                <p className="text-sm font-semibold text-foreground">
                  {column.heading}
                </p>
                <ul className="mt-4 space-y-3 text-sm">
                  {column.links.map((link) => (
                    <li key={link.href}>
                      <Link
                        href={link.href}
                        className="text-muted-foreground transition-colors hover:text-foreground"
                      >
                        {link.label}
                      </Link>
                    </li>
                  ))}
                </ul>
              </nav>
            ))}
          </div>

          <div className="mt-12 border-t pt-6 text-xs text-muted-foreground">
            <p>&copy; {new Date().getFullYear()} 受験マップ</p>
          </div>
        </div>
      </footer>
    </main>
  );
}
