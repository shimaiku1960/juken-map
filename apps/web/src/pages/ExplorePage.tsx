import UniversitySearch from "@/web/components/UniversitySearch";
import PageShell from "@/web/components/layout/PageShell";
import PageHeader from "@/web/components/layout/PageHeader";
import { useUniversities } from "@/web/hooks/useUniversities";

export default function ExplorePage() {
  const { data: raw = [], isPending } = useUniversities();

  // 学部系統(タグ)で絞り込めるよう、大学ごとに学部数とタグ名を集約
  const universities = raw.map((u) => ({
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
      <PageHeader
        title="大学を探す"
        description="地域や国公立・私立、学部系統から志望校候補を絞り込めます。"
      />
      {isPending ? (
        <p className="text-sm text-muted-foreground">読み込み中…</p>
      ) : (
        <UniversitySearch universities={universities} />
      )}
    </PageShell>
  );
}
