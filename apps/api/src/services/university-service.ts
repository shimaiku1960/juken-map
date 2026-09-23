import { createHash } from "node:crypto";
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

// 大学一覧は全員に同じものを返し、変わるのは管理画面でマスターを編集したときだけ。
// 毎回 DB を引くと一番重い API（大学 823 件 × LEFT JOIN 4本）になるので、JSON にした
// 状態でメモリに持つ。圧縮後は 8KB ほどしかなく、形を削っても 1 割も減らない。
// 効くのは DB を引かないことと、変わっていなければ 304 で中身を送らないこと。
//
// 管理画面の編集は master-service が invalidate を呼んで捨てる。seed の直接投入や、
// 無停止デプロイで新旧のプロセスが並ぶ間の編集はそれでは届かないので、期限でも捨てる。
const EXPLORE_CACHE_TTL_MS = 10 * 60 * 1000;

type ExploreSnapshot = { json: string; etag: string; expiresAt: number };

let exploreSnapshot: ExploreSnapshot | null = null;
let exploreLoading: Promise<ExploreSnapshot> | null = null;
// 読み込み中に invalidate されたら、その読み込み結果は古いかもしれないので置かない。
let exploreGeneration = 0;

/** 大学一覧を JSON 文字列と ETag で返す。キャッシュが生きていれば DB を引かない。 */
export async function getUniversitiesForExplore(): Promise<{ json: string; etag: string }> {
  if (exploreSnapshot && exploreSnapshot.expiresAt > Date.now()) return exploreSnapshot;

  // 期限切れの直後に同時に来たリクエストは、1回の読み込みを待ち合わせる。
  if (exploreLoading) return exploreLoading;

  const generation = exploreGeneration;
  const loading = (async () => {
    const json = JSON.stringify(await listUniversitiesForExplore());
    const snapshot = {
      json,
      etag: `"${createHash("sha1").update(json).digest("base64url")}"`,
      expiresAt: Date.now() + EXPLORE_CACHE_TTL_MS,
    };
    if (generation === exploreGeneration) exploreSnapshot = snapshot;
    return snapshot;
  })();
  exploreLoading = loading;
  // 途中で invalidate されて次の読み込みが始まっていたら、そちらを消さない。
  const settle = () => {
    if (exploreLoading === loading) exploreLoading = null;
  };
  loading.then(settle, settle);
  return loading;
}

/** 大学・学部・タグのつながりを変えたら呼ぶ。次のリクエストで DB から作り直す。 */
export function invalidateUniversitiesForExplore() {
  exploreGeneration++;
  exploreSnapshot = null;
  exploreLoading = null;
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
