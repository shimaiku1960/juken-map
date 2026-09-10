import prisma from "@/api/infra/prisma";
import { measured } from "@/api/observability/measured";

export function listTextbookMasters() {
  return measured("textbookMaster.list", () =>
    prisma.textbookMaster.findMany({
      include: { metrics: { orderBy: { id: "asc" } } },
      orderBy: { id: "asc" },
    })
  );
}

// 志望校ページの「対策科目」チップ用。集計に必要な subject だけを引く。
export function listTextbookSubjects(userId: string) {
  return measured("textbook.listSubjects", () =>
    prisma.textbook.findMany({
      where: { userId },
      select: { subject: true },
    })
  );
}
