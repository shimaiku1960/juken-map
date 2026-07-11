import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/auth";
import prisma from "@/lib/prisma";

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const masters = await prisma.textbookMaster.findMany({
    include: { metrics: { orderBy: { id: "asc" } } },
    orderBy: { id: "asc" },
  });

  return NextResponse.json(masters);
}
