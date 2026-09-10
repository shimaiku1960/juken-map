import { betterAuth } from "better-auth";
import { prismaAdapter } from "better-auth/adapters/prisma";
import { prisma } from "@/api/infra/prisma";
import {
  notifyAdminOfNewUser,
  sendVerificationEmail,
  sendPasswordResetEmail,
} from "@/api/infra/email";

// アプリ唯一の Better Auth 定義。Next.js 側にあった同等の定義は削除済み。
//
// セッションテーブルと BETTER_AUTH_SECRET は Next.js 時代から引き継いでいるので、
// あの頃に発行された Cookie もそのまま受理できる。secret を変えると全ユーザーが
// 強制ログアウトになるので触らないこと。
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
