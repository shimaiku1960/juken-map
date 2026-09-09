import { headers } from "next/headers";
import { NextResponse } from "next/server";
import { auth } from "@/backend/infra/auth";
import { SITE_URL } from "@/shared/site";

const NOTIFICATION_SETTINGS_PATH = "/profile#notification-settings";

export async function GET() {
  const session = await auth.api.getSession({ headers: await headers() });
  if (session) {
    return NextResponse.redirect(new URL(NOTIFICATION_SETTINGS_PATH, SITE_URL));
  }

  const loginUrl = new URL("/login", SITE_URL);
  loginUrl.searchParams.set("callbackURL", NOTIFICATION_SETTINGS_PATH);
  return NextResponse.redirect(loginUrl);
}
