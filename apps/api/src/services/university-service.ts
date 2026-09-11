import { select } from "@/api/infra/db";
import type { FacultyRow, TagRow, UniversityRow } from "@/api/infra/tables";
import { measured } from "@/api/observability/measured";

// 大学 → 学部 → タグは一本道の「1対多の連なり」なので、LEFT JOIN 1本で取れる。
// 行は（大学 × 学部 × タグ）の数だけ並ぶので、上から順に見て入れ子へ詰め直す。
//
// Prisma の include / select はこれを大学・学部・タグ（中間テーブル込み）の
// 3本に分けて取り、入れ子の組み立ても自動でやっていた。

type ExploreRow = Pick<UniversityRow, "id" | "name" | "prefecture" | "type"> & {
  facultyId: number | null;
  tagName: string | null;
};

export function listUniversitiesForExplore() {
  return measured("university.listForExplore", async () => {
    const rows = await select<ExploreRow>(
      `SELECT u.id, u.name, u.prefecture, u.type, f.id AS facultyId, t.name AS tagName
       FROM University AS u
       LEFT JOIN Faculty AS f ON f.universityId = u.id
       LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
       LEFT JOIN Tag AS t ON t.id = ft.B
       ORDER BY u.name ASC, u.id ASC, f.id ASC, t.id ASC`
    );

    // ORDER BY で同じ大学・同じ学部の行が隣り合うので、直前の要素と比べるだけで束ねられる。
    const universities: {
      id: number;
      name: string;
      prefecture: string;
      type: string;
      faculties: { tags: { name: string }[] }[];
    }[] = [];
    let lastFacultyId: number | null = null;
    for (const row of rows) {
      let university = universities.at(-1);
      if (university?.id !== row.id) {
        university = { id: row.id, name: row.name, prefecture: row.prefecture, type: row.type, faculties: [] };
        universities.push(university);
        lastFacultyId = null;
      }
      // 学部が1つも無い大学は、学部の列が NULL の行が1行だけ来る
      if (row.facultyId === null) continue;
      if (row.facultyId !== lastFacultyId) {
        university.faculties.push({ tags: [] });
        lastFacultyId = row.facultyId;
      }
      if (row.tagName !== null) university.faculties.at(-1)!.tags.push({ name: row.tagName });
    }
    return universities;
  });
}

type DetailRow = UniversityRow & {
  f_id: number | null;
  f_name: string | null;
  f_examDate: Date | null;
  f_createdAt: Date | null;
  f_universityId: number | null;
  t_id: number | null;
  t_name: string | null;
  t_createdAt: Date | null;
};

// 大学詳細ページ用。学部と、絞り込みに使うタグまで一度に引く。
export function findUniversityDetail(id: number) {
  return measured("university.findDetail", async () => {
    const rows = await select<DetailRow>(
      `SELECT u.id, u.name, u.prefecture, u.type, u.createdAt,
              f.id AS f_id, f.name AS f_name, f.examDate AS f_examDate,
              f.createdAt AS f_createdAt, f.universityId AS f_universityId,
              t.id AS t_id, t.name AS t_name, t.createdAt AS t_createdAt
       FROM University AS u
       LEFT JOIN Faculty AS f ON f.universityId = u.id
       LEFT JOIN _FacultyToTag AS ft ON ft.A = f.id
       LEFT JOIN Tag AS t ON t.id = ft.B
       WHERE u.id = ?
       ORDER BY f.id ASC, t.id ASC`,
      [id]
    );
    if (rows.length === 0) return null;

    const [first] = rows;
    const faculties: (FacultyRow & { tags: TagRow[] })[] = [];
    for (const row of rows) {
      if (row.f_id === null) continue;
      let faculty = faculties.at(-1);
      if (faculty?.id !== row.f_id) {
        faculty = {
          id: row.f_id,
          name: row.f_name!,
          examDate: row.f_examDate!,
          createdAt: row.f_createdAt!,
          universityId: row.f_universityId!,
          tags: [],
        };
        faculties.push(faculty);
      }
      if (row.t_id !== null) {
        faculty.tags.push({ id: row.t_id, name: row.t_name!, createdAt: row.t_createdAt! });
      }
    }

    return {
      id: first.id,
      name: first.name,
      prefecture: first.prefecture,
      type: first.type,
      createdAt: first.createdAt,
      faculties,
    };
  });
}
