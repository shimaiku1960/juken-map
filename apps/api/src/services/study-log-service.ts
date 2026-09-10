import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

export function listStudyLogs(userId: string) {
  return measured("studyLog.list", () =>
    prisma.studyLog.findMany({
      where: { userId },
      orderBy: { date: "desc" },
      include: { textbook: true },
    })
  );
}
