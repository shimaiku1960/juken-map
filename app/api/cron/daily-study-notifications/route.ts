import { NextResponse } from "next/server";
import { z } from "zod";
import { sendDailyNotifications } from "@/lib/sendDailyNotifications";

const bodySchema = z.object({ slot: z.enum(["morning", "evening"]) });

function isAuthorized(request: Request) {
  const secret = process.env.DAILY_NOTIFICATION_SECRET;
  return (
    Boolean(secret) && request.headers.get("authorization") === `Bearer ${secret}`
  );
}

export async function POST(request: Request) {
  if (!isAuthorized(request)) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const result = bodySchema.safeParse(await request.json());
  if (!result.success) {
    return NextResponse.json({ error: "Invalid slot" }, { status: 400 });
  }

  return NextResponse.json(await sendDailyNotifications(result.data.slot));
}
