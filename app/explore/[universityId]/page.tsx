import type { Metadata } from "next";
import { getCurrentSession } from "@/lib/auth-session";
import { listGoalFacultyIds } from "@/lib/services/goal-service";
import { findUniversityDetail } from "@/lib/services/university-service";
import { NOINDEX } from "@/lib/site";
import { redirect, notFound } from "next/navigation";
import Link from "next/link";
import FacultyList from "@/app/components/FacultyList";
import { DEMO_EMAIL } from "@/lib/demo";

// ログイン必須のページなので検索結果には載せない。
export const metadata: Metadata = { robots: NOINDEX };

const UniversityDetailPage = async ({
  params,
}: {
  params: Promise<{ universityId: string }>;
}) => {
  const session = await getCurrentSession();

  if (!session) {
    redirect("/login");
  }

  const { universityId } = await params;
  const id = Number(universityId);
  if (Number.isNaN(id)) {
    notFound();
  }

  const university = await findUniversityDetail(id);

  if (!university) {
    notFound();
  }

  const goals = await listGoalFacultyIds(session.user.id);
  const registeredFacultyIds = goals.map((g) => g.facultyId);

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <Link
        href="/explore"
        className="text-sm text-primary hover:underline"
      >
        ← 大学を探すに戻る
      </Link>

      <h1 className="text-3xl font-bold mt-4 mb-1">{university.name}</h1>
      <p className="text-muted-foreground mb-6">
        {university.prefecture} / {university.type}
      </p>

      <h2 className="text-xl font-bold mb-4">学部</h2>
      {university.faculties.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          この大学の学部情報はまだ準備中です。
        </p>
      ) : (
        <FacultyList
          faculties={university.faculties}
          registeredFacultyIds={registeredFacultyIds}
          readOnly={session.user.email === DEMO_EMAIL}
        />
      )}
    </main>
  );
};

export default UniversityDetailPage;
