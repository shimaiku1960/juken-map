import Image, { type StaticImageData } from "next/image";
import Link from "next/link";
import {
  ArrowRight,
  BarChart3,
  CalendarDays,
  Check,
  Clock3,
  type LucideIcon,
  MapPinned,
  Play,
  Target,
  TimerReset,
} from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  AmbientOrb,
  HeroReveal,
  Reveal,
} from "@/app/components/landing/LandingMotion";
// 画像は public/ ではなく import で読み込む。ビルド時にファイル名へコンテンツ
// ハッシュが付くため、中身を差し替えれば URL が変わりキャッシュが自動で外れる。
// width/height も import した値から自動で決まる。
import studyCalendarImage from "@/app/components/landing/images/study-calendar.png";
import studyStartDialogImage from "@/app/components/landing/images/study-start-dialog.png";
import subjectStudyTimeImage from "@/app/components/landing/images/subject-study-time.png";

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
        <div className="relative flex flex-wrap items-end justify-between gap-3">
          <div>
            <p className="text-sm font-semibold text-primary">
              ひとつながりの学習記録
            </p>
            <h3 className="mt-1 text-lg font-bold sm:text-xl">
              始めてから、振り返るまで。
            </h3>
          </div>
          <span className="rounded-full border bg-card px-3 py-1 text-xs font-medium text-muted-foreground">
            約18秒
          </span>
        </div>

        <div className="relative mt-8 grid items-stretch gap-8 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.2fr)] lg:gap-12">
          <figure className="mx-auto w-full max-w-[18rem]">
            <div className="overflow-hidden rounded-[2.3rem] border-[6px] border-foreground/90 bg-foreground shadow-2xl shadow-primary/20">
              <video
                className="block aspect-[606/1048] w-full bg-background object-contain"
                autoPlay
                controls
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

          <ol className="grid gap-3 lg:grid-rows-4">
              {[
                {
                  number: "01",
                  label: "内容を選ぶ",
                  description: "今日の予定や参考書から選択",
                  icon: Clock3,
                },
                {
                  number: "02",
                  label: "時間を計る",
                  description: "タイマーで学習時間を計測",
                  icon: TimerReset,
                },
                {
                  number: "03",
                  label: "そのまま保存",
                  description: "内容と時間を確認して実績へ",
                  icon: Check,
                },
                {
                  number: "04",
                  label: "記録を振り返る",
                  description: "カレンダーで予定と実績を確認",
                  icon: CalendarDays,
                },
              ].map(({ number, label, description, icon: Icon }) => (
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
                    <p className="mt-1 text-xs text-muted-foreground">
                      {description}
                    </p>
                  </div>
                </li>
              ))}
          </ol>
        </div>
      </div>
    </div>
  </div>
);

// 機能セクションの説明側。points は隣のスクリーンショットの「読み方」を示す
// 短い補足で、画像を見ただけでは伝わらない部分を言葉で補う。
const FeatureText = ({
  icon: Icon,
  title,
  description,
  points,
}: {
  icon: LucideIcon;
  title: string;
  description: string;
  points: string[];
}) => (
  <div>
    <div className="flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary">
      <Icon aria-hidden="true" className="size-5" />
    </div>
    <h3 className="mt-5 text-2xl font-bold">{title}</h3>
    <p className="mt-4 leading-7 text-muted-foreground">{description}</p>
    <ul className="mt-6 space-y-3">
      {points.map((point) => (
        <li key={point} className="flex items-start gap-3">
          <span className="mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
            <Check aria-hidden="true" className="size-3" />
          </span>
          <span className="text-sm leading-6">{point}</span>
        </li>
      ))}
    </ul>
  </div>
);

// 機能セクションの実画面。デバイスの枠は見せず、該当箇所だけを切り抜いた
// スクリーンショットを同じカードで並べて、3つの見せ方を揃える。
const ScreenshotCard = ({
  src,
  alt,
}: {
  src: StaticImageData;
  alt: string;
}) => (
  <figure className="overflow-hidden rounded-xl border bg-card shadow-[0_24px_60px_-36px_color-mix(in_oklch,var(--primary)_60%,transparent)]">
    <Image
      src={src}
      alt={alt}
      sizes="(min-width: 1024px) 50vw, 100vw"
      className="block h-auto w-full"
    />
  </figure>
);

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
              <span className="text-primary">合格までの積み重ねに。</span>
            </h1>
            <p className="mx-auto mt-7 max-w-xl text-base leading-8 text-muted-foreground sm:text-lg lg:mx-0">
              学習を始めるときにタイマーを押すだけ。
              勉強時間や予定、毎日の積み重ねをひとつのカレンダーで振り返れます。
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
              登録は無料。すぐに始められます
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
            title="勉強したはずなのに、積み重ねが見えない。"
          />
          <div className="mt-12 grid gap-4 md:grid-cols-3">
            {[
              "勉強時間を後から思い出して記録するのが面倒",
              "予定を立てても、そのとおりにできたか分からない",
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
              受験マップは、勉強を始めた瞬間から記録に残ります。
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
            <FeatureText
              icon={Clock3}
              title="すぐに始められる"
              description="今日の予定や学習内容を選んで、そのままタイマーを開始。過去の学習も時間を選ぶだけで、すばやく記録できます。"
              points={[
                "「その他の学習」で予定外の勉強も記録",
                "計測を止めたら、そのまま実績として保存",
                "あとから記録するときは時間を選ぶだけ",
              ]}
            />
            <ScreenshotCard
              src={studyStartDialogImage}
              alt="今日の予定「日本史 近現代 通史」を選んで計測を開始する実際の画面"
            />
          </Reveal>

          <Reveal className="mt-28 grid items-center gap-10 lg:grid-cols-2 lg:gap-20">
            <div className="order-2 lg:order-1">
              <ScreenshotCard
                src={studyCalendarImage}
                alt="学習カレンダーで日ごとの学習時間と科目の内訳、選んだ日の学習予定を確認する実際の画面"
              />
            </div>
            <div className="order-1 lg:order-2">
              <FeatureText
                icon={CalendarDays}
                title="予定と実績がつながる"
                description="何を勉強する予定だったか、実際にどれだけ取り組んだかを、ひとつのカレンダーで確認できます。"
                points={[
                  "色が濃い日ほど、その日の学習時間が長い",
                  "日ごとのバーで科目の内訳がわかる",
                  "予定だけの日と実績のある日を見分けられる",
                ]}
              />
            </div>
          </Reveal>

          <Reveal className="mt-28 grid items-center gap-10 lg:grid-cols-2 lg:gap-20">
            <FeatureText
              icon={BarChart3}
              title="科目ごとの積み重ねが見える"
              description="学習時間、継続日数、科目ごとの配分を振り返って、次に何をやるかを決められます。"
              points={[
                "直近7日間の合計を科目ごとに比較",
                "記録がない科目は「—」ですぐ気づける",
                "偏りを見て、次にやる科目を決められる",
              ]}
            />
            <ScreenshotCard
              src={subjectStudyTimeImage}
              alt="直近7日間の科目別学習時間。英語3時間30分、数学4時間、国語2時間45分の実際の集計画面"
            />
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
              今日の勉強が何につながるのかを、見失わずに済みます。
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
              ["01", "無料で登録", "メールアドレスか、Google・GitHubのアカウントで登録できます。"],
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
            学習を始めるところから、毎日の積み重ねを振り返るところまで。
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
            をご覧ください。
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
                今日の勉強を、合格までの積み重ねに。
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
