import { createHash, randomBytes } from "node:crypto";
import prisma from "../lib/prisma";

async function main() {
  const email = process.argv[2]?.trim().toLowerCase();
  const guardianConsentConfirmed = process.argv.includes(
    "--guardian-consent-confirmed"
  );

  if (!email || !email.includes("@")) {
    throw new Error(
      "Usage: npm run support:invite -- user@example.com [--guardian-consent-confirmed]"
    );
  }

  const token = randomBytes(32).toString("base64url");
  const tokenHash = createHash("sha256").update(token).digest("hex");
  const expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);
  const invitation = await prisma.supportCheckoutInvitation.create({
    data: {
      email,
      tokenHash,
      expiresAt,
      guardianConsentConfirmedAt: guardianConsentConfirmed ? new Date() : null,
    },
  });
  const appUrl = process.env.BETTER_AUTH_URL || "http://localhost:3000";

  console.log(`Invitation expires: ${invitation.expiresAt.toISOString()}`);
  console.log(`${appUrl}/support/apply?invitation=${encodeURIComponent(token)}`);
}

main()
  .catch((error: unknown) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  })
  .finally(async () => {
    await prisma.$disconnect();
  });
