import { lazy } from "react";
import { Link } from "react-router";
import { ymdLocal, todayYmd } from "@/shared/date";
import { DEMO_EMAIL } from "@/shared/demo";
import { buttonVariants } from "@/web/components/ui/button";
import { cn } from "@/web/lib/utils";
import { useSession } from "@/web/lib/auth-client";
import { useFirstChoiceGoal } from "@/web/hooks/useFirstChoiceGoal";

// 未ログインは LP、ログイン済みは学習開始だけを使う。どちらか片方しか要らないので分けて読む。
// （LP のアニメーション用ライブラリをログイン後の人に、学習記録のフォームを LP の人に配らない）
const loadLandingPage = () => import("@/web/components/LandingPage");
const LandingPage = lazy(loadLandingPage);
// トップを直接開いたときは、ログイン判定の完了を待たずに LP の取得を始める。
// 待つと「判定 → LP の取得」が直列になり、初めて来た人ほど表示が遅れる。
// ビルド時の HTML 生成（entry-server.tsx）は Node で読み込むので window が無い。
if (typeof window !== "undefined" && window.location.pathname === "/") void loadLandingPage();
const StudySessionManager = lazy(
  () => import("@/web/components/StudySessionManager")
);

export default function HomePage() {
  const { data: session, isPending } = useSession();
  const { data: firstChoiceGoal } = useFirstChoiceGoal(Boolean(session));

  if (isPending) return null;
  // 未ログインはログイン画面へ飛ばさず、トップで LP を見せる。
  if (!session) return <LandingPage />;

  const todayStr = todayYmd();
  const heroFirstChoice = firstChoiceGoal
    ? {
        name: `${firstChoiceGoal.faculty.university.name} ${firstChoiceGoal.faculty.name}`,
        // API は JSON なので Date ではなく文字列で届く。
        examDate: new Date(firstChoiceGoal.faculty.examDate),
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
          <span className="mx-1 text-lg font-bold text-foreground">
            あと {daysToExam} 日
          </span>
        </p>
      )}
      <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
        今日の学習を始めよう
      </h1>
      <div id="study-start" className="scroll-mt-24">
        <StudySessionManager
          userId={session.user.id}
          readOnly={session.user.email === DEMO_EMAIL}
          variant="hero"
        />
      </div>
      <Link
        to="/dashboard"
        className={cn(
          buttonVariants({ variant: "link", size: "lg" }),
          "mt-4 h-11"
        )}
      >
        今日の予定・記録を見る →
      </Link>
    </main>
  );
}
