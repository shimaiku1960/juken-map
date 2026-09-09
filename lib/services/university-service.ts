import prisma from "@/lib/prisma";
import { measured } from "@/lib/observability/measured";

export function listUniversitiesForExplore() {
  return measured("university.listForExplore", () =>
    prisma.university.findMany({
      orderBy: { name: "asc" },
      select: {
        id: true,
        name: true,
        prefecture: true,
        type: true,
        faculties: { select: { tags: { select: { name: true } } } },
      },
    })
  );
}

// 大学詳細ページ用。学部と、絞り込みに使うタグまで一度に引く。
export function findUniversityDetail(id: number) {
  return measured("university.findDetail", () =>
    prisma.university.findUnique({
      where: { id },
      include: {
        faculties: {
          include: { tags: true },
          orderBy: { id: "asc" },
        },
      },
    })
  );
}
