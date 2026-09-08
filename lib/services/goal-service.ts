import prisma from "@/lib/prisma";
import { measured } from "@/lib/observability/measured";

export function listGoals(userId: string) {
  return measured("goal.list", () =>
    prisma.finalGoal.findMany({
      where: { userId },
      orderBy: { createdAt: "asc" },
      include: {
        faculty: {
          include: { university: true, tags: true },
        },
      },
    })
  );
}
