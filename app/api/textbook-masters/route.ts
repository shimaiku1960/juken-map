import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/lib/infra/auth";
import { listTextbookMasters } from "@/lib/services/textbook-service";

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const masters = await listTextbookMasters();

  return NextResponse.json(masters);
}
