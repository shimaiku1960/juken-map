import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";
import { redirect } from "next/navigation";
import UniversitySearch from "@/app/components/UniversitySearch";
import PageShell from "@/app/components/layout/PageShell";
import PageHeader from "@/app/components/layout/PageHeader";

const ExplorePage = async () => {
  const session = await auth.api.getSession({
    headers: await headers(),
  });

  if (!session) {
    redirect("/login");
  }

  const universitiesRaw = await prisma.university.findMany({
    orderBy: { name: "asc" },
    select: {
      id: true,
      name: true,
      prefecture: true,
      type: true,
      faculties: { select: { tags: { select: { name: true } } } },
    },
  });

  // 学部系統(タグ)で絞り込めるよう、大学ごとに学部数とタグ名を集約
  const universities = universitiesRaw.map((u) => ({
    id: u.id,
    name: u.name,
    prefecture: u.prefecture,
    type: u.type,
    facultyCount: u.faculties.length,
    tagNames: [
      ...new Set(u.faculties.flatMap((f) => f.tags.map((t) => t.name))),
    ],
  }));

  return (
    <PageShell>
      <PageHeader title="大学を探す" description="地域や国公立・私立、学部系統から志望校候補を絞り込めます。" />
      <UniversitySearch universities={universities} />
    </PageShell>
  );
};

export default ExplorePage;
