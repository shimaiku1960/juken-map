import { betterAuth } from "better-auth";
import { prismaAdapter } from "better-auth/adapters/prisma";
import { prisma } from "@/backend/infra/prisma";
import {
  notifyAdminOfNewUser,
  sendVerificationEmail,
  sendPasswordResetEmail,
} from "@/backend/infra/email";

// 移行期間中は Next.js 側の src/backend/infra/auth.ts と設定を二重に持つ。
// 違いは plugins: [nextCookies()] を持たないことだけで、これは Server Actions 用なので
// Fastify には不要である（このリポジトリに Server Actions は0件）。
// Next.js を削除する Step 3 で、あちらを消してこれを唯一の定義にする。
//
// セッションテーブルと BETTER_AUTH_SECRET は共通なので、Next.js が発行した Cookie を
// そのまま受理できる（Phase 0 で実証済み）。secret を変えると全ユーザーが強制ログアウトになる。
export const auth = betterAuth({
  database: prismaAdapter(prisma, {
    provider: "mysql",
  }),
  user: {
    additionalFields: {
      nickname: {
        type: "string",
        required: false,
      },
    },
  },
  databaseHooks: {
    user: {
      create: {
        after: async (user) => {
          // OAuthユーザーは作成時点でメール確認済み。メール登録は確認完了後に通知する。
          if (user.emailVerified) {
            await notifyAdminOfNewUser(user);
          }
        },
      },
    },
  },
  emailAndPassword: {
    enabled: true,
    requireEmailVerification: true,
    sendResetPassword: async ({ user, url }) => {
      await sendPasswordResetEmail(user.email, url);
    },
  },
  emailVerification: {
    sendVerificationEmail: async ({ user, url }) => {
      await sendVerificationEmail(user.email, url);
    },
    sendOnSignUp: true,
    afterEmailVerification: async (user) => {
      await notifyAdminOfNewUser(user);
    },
  },
  socialProviders: {
    google: {
      clientId: process.env.AUTH_GOOGLE_ID as string,
      clientSecret: process.env.AUTH_GOOGLE_SECRET as string,
    },
    github: {
      clientId: process.env.AUTH_GITHUB_ID as string,
      clientSecret: process.env.AUTH_GITHUB_SECRET as string,
    },
  },
  trustedOrigins: [
    "https://juken-map.com",
    "https://www.juken-map.com",
    "http://localhost:5173",
    "http://localhost:4000",
  ],
});
