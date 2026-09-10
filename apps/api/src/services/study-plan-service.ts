import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

export function listStudyPlans(userId: string) {
  return measured("studyPlan.list", () =>
    prisma.studyPlan.findMany({
      where: { userId },
      orderBy: { date: "asc" },
      include: { textbook: true, studyLog: { select: { id: true } } },
    })
  );
}
