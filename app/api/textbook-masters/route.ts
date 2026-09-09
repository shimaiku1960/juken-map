import { NextResponse } from "next/server";
import { headers } from "next/headers";
import { auth } from "@/backend/infra/auth";
import { listTextbookMasters } from "@/backend/services/textbook-service";

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const masters = await listTextbookMasters();

  return NextResponse.json(masters);
}
