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

/**
 * 指定した参考書のうち、自分が所有しているものの件数を返す。
 * 呼び出し元は「渡した件数と一致するか」で他人の参考書混入を弾く。
 */
export function countOwnedTextbooks(ids: number[], userId: string) {
  return measured("textbook.countOwned", () =>
    prisma.textbook.count({ where: { id: { in: ids }, userId } })
  );
}

/** 1件だけの所有確認。範囲や単位の検証にも使うので行ごと返す。 */
export function findOwnedTextbook(id: number, userId: string) {
  return measured("textbook.findOwned", () =>
    prisma.textbook.findFirst({ where: { id, userId } })
  );
}
