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

/** 自分の参考書の一覧。 */
export function listTextbooks(userId: string) {
  return measured("textbook.list", () =>
    prisma.textbook.findMany({ where: { userId }, orderBy: { name: "asc" } })
  );
}

/** マスター登録から作るときの元データ。総量の候補（metrics）も一緒に引く。 */
export function findTextbookMaster(id: number) {
  return measured("textbookMaster.find", () =>
    prisma.textbookMaster.findUnique({ where: { id }, include: { metrics: true } })
  );
}

/** 参考書を登録する。同名の重複は DB の一意制約が弾く（P2002）。 */
export function createTextbook(data: {
  name: string;
  userId: string;
  masterId?: number;
  totalAmount?: number;
  rangeUnit?: string;
  subject?: string | null;
}) {
  return measured("textbook.create", () => prisma.textbook.create({ data }));
}

/** 逆算設定を更新する。送られてきた項目だけ変える。 */
export function updateTextbookProgress(
  id: number,
  data: {
    totalAmount?: number;
    rangeUnit?: string;
    targetDate?: string | null;
    subject?: string | null;
  }
) {
  return measured("textbook.updateProgress", () =>
    prisma.textbook.update({
      where: { id },
      data: {
        ...(data.totalAmount !== undefined && { totalAmount: data.totalAmount }),
        ...(data.rangeUnit !== undefined && { rangeUnit: data.rangeUnit }),
        ...(data.targetDate !== undefined && {
          targetDate:
            data.targetDate == null
              ? null
              : new Date(`${data.targetDate}T00:00:00.000Z`),
        }),
        ...(data.subject !== undefined && { subject: data.subject }),
      },
    })
  );
}
