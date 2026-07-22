import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { redirect, notFound } from "next/navigation";
import Link from "next/link";
import FacultyList from "@/app/components/FacultyList";
import { DEMO_EMAIL } from "@/lib/demo";

const UniversityDetailPage = async ({
  params,
}: {
  params: Promise<{ universityId: string }>;
}) => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  const { universityId } = await params;
  const id = Number(universityId);
  if (Number.isNaN(id)) {
    notFound();
  }

  const university = await prisma.university.findUnique({
    where: { id },
    include: {
      faculties: {
        include: { tags: true },
        orderBy: { id: "asc" },
      },
    },
  });

  if (!university) {
    notFound();
  }

  const goals = await prisma.finalGoal.findMany({
    where: { userId: session.user.id },
    select: { facultyId: true },
  });
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
