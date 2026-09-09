import prisma from "@/lib/infra/prisma";
import { measured } from "@/lib/observability/measured";

export function listStudyLogs(userId: string) {
  return measured("studyLog.list", () =>
    prisma.studyLog.findMany({
      where: { userId },
      orderBy: { date: "desc" },
      include: { textbook: true },
    })
  );
}
