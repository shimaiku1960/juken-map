import prisma from "@/lib/prisma";
import { measured } from "@/lib/observability/measured";

export function listTextbookMasters() {
  return measured("textbookMaster.list", () =>
    prisma.textbookMaster.findMany({
      include: { metrics: { orderBy: { id: "asc" } } },
      orderBy: { id: "asc" },
    })
  );
}
