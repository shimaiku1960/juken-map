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
