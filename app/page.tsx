import type { Metadata } from "next";
import { getCurrentSession } from "@/lib/infra/auth-session";
import { toStudyPlanDTO } from "@/lib/dto/study";
import { findFirstChoiceGoal } from "@/lib/services/goal-service";
import { listStudyPlans } from "@/lib/services/study-plan-service";
import Link from "next/link";
import LandingPage from "@/app/components/LandingPage";
import StudySessionManager from "@/app/components/StudySessionManager";
import type { StudyPlan } from "@/app/hooks/useStudyPlans";
import { ymdLocal, todayYmd } from "@/lib/domain/date";
import { DEMO_EMAIL } from "@/lib/demo/constants";
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/ui/utils";

// トップは未ログイン訪問者（Googlebot 含む）には LP を返す検索流入の入口。
// ログイン後ページと認証ページは各ルート側で個別に noindex を指定する。
export const metadata: Metadata = {
  title: "今日の勉強を、合格までの積み重ねに。｜受験マップ",
  description:
    "学習の開始から時間記録、予定と実績の確認、科目別の振り返りまでをひとつにつなぐ、大学受験生向け学習管理アプリです。",
  openGraph: {
    title: "今日の勉強を、合格までの積み重ねに。｜受験マップ",
    description:
      "学習の開始から時間記録、予定と実績の確認、科目別の振り返りまでをひとつにつなぐ、大学受験生向け学習管理アプリです。",
    type: "website",
  },
};

const Home = async () => {
  const session = await getCurrentSession();
  // 未ログインはログイン画面へ飛ばさず、トップで LP を見せる。
  if (!session) return <LandingPage />;

  const todayStr = todayYmd();
  const [firstChoiceGoal, plans] = await Promise.all([
    findFirstChoiceGoal(session.user.id),
    listStudyPlans(session.user.id),
  ]);

  const initialPlans: StudyPlan[] = plans.map(toStudyPlanDTO);

  const heroFirstChoice = firstChoiceGoal
    ? {
        name: `${firstChoiceGoal.faculty.university.name} ${firstChoiceGoal.faculty.name}`,
        examDate: firstChoiceGoal.faculty.examDate,
      }
    : null;
  const daysToExam = heroFirstChoice
    ? Math.ceil(
        (new Date(ymdLocal(heroFirstChoice.examDate)).getTime() -
          new Date(todayStr).getTime()) /
          86_400_000
      )
    : null;

  return (
    <main className="mx-auto flex min-h-[calc(100dvh-4rem)] w-full max-w-3xl flex-col items-center justify-center gap-6 px-4 py-8 text-center sm:px-8">
      {heroFirstChoice && daysToExam != null && daysToExam >= 0 && (
        <p className="text-sm text-muted-foreground">
          {heroFirstChoice.name} まで
          <span className="mx-1 text-lg font-bold text-foreground">あと {daysToExam} 日</span>
        </p>
      )}
      <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">今日の学習を始めよう</h1>
      <div id="study-start" className="scroll-mt-24">
        <StudySessionManager
          initialPlans={initialPlans}
          userId={session.user.id}
          readOnly={session.user.email === DEMO_EMAIL}
          variant="hero"
        />
      </div>
      <Link
        href="/dashboard"
        className={cn(buttonVariants({ variant: "link", size: "lg" }), "mt-4 h-11")}
      >
        今日の予定・記録を見る →
      </Link>
    </main>
  );
};

export default Home;
