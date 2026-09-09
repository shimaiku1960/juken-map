import { Link, useParams } from "react-router";
import FacultyList from "@/web/components/FacultyList";
import { DEMO_EMAIL } from "@/shared/demo";
import { useUniversityDetail } from "@/web/hooks/useUniversities";
import { useSession } from "@/web/lib/auth-client";
import NotFoundPage from "@/web/pages/NotFoundPage";

export default function UniversityDetailPage() {
  const { universityId } = useParams();
  const id = Number(universityId);
  const { data: session } = useSession();
  const { data, isPending, isError } = useUniversityDetail(id);

  if (!Number.isInteger(id) || isError) return <NotFoundPage />;
  if (isPending || !data) {
    return (
      <main className="w-full mx-auto max-w-3xl p-8">
        <p className="text-sm text-muted-foreground">読み込み中…</p>
      </main>
    );
  }

  const { university, registeredFacultyIds } = data;

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
      <Link to="/explore" className="text-sm text-primary hover:underline">
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
          // API は JSON なので examDate が文字列で届く。FacultyList は Date を
          // 期待しているため、境界のここで戻す（分離で初めて必要になった変換）。
          faculties={university.faculties.map((f) => ({
            ...f,
            examDate: new Date(f.examDate),
          }))}
          registeredFacultyIds={registeredFacultyIds}
          readOnly={session?.user.email === DEMO_EMAIL}
        />
      )}
    </main>
  );
}
