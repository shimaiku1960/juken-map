import { betterAuth } from "better-auth";
import { APIError } from "better-auth/api";
import { pool, select } from "@/api/infra/db";
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
//
// DB にはアプリと同じ mysql2 のプールを渡す。Better Auth は getConnection を持つ
// オブジェクトを MySQL と判断し、内部の Kysely（SQL を組み立てるライブラリ）で
// user / session / account / verification を読み書きする。
// 日時は Date のままドライバへ渡るので、プールの timezone: "Z"（UTC）がそのまま効き、
// Prisma 時代に保存したセッションの期限とも同じ時刻として比べられる。
export const auth = betterAuth({
  database: pool,
  user: {
    additionalFields: {
      nickname: {
        type: "string",
        required: false,
      },
      // 管理者ページの権限。セッションに載せて API と画面の両方から読めるようにする。
      // input: false が無いと、サインアップや update-user の本文に role: "admin" を
      // 混ぜるだけで誰でも管理者になれてしまう。付け替えは pnpm admin:grant だけで行う。
      role: {
        type: "string",
        required: false,
        defaultValue: "user",
        input: false,
      },
      // 管理者に停止された日時。セッションに載せておくと、requireSession（context.ts）が
      // DB を引き直さずに停止中を弾ける。role と同じく input: false で利用者からは書けない。
      bannedAt: {
        type: "date",
        required: false,
        input: false,
      },
    },
  },
  databaseHooks: {
    session: {
      // 停止された利用者のログインを断る。ここはメール＋パスワードも Google / GitHub も
      // 必ず通る一本道（sign-in も OAuth の callback も最後はセッションを1行作る）なので、
      // 入口ごとに同じ判定を書かずに済む。
      //
      // 停止のときに既存の session は消しているが（admin-service.ts）、それだけでは
      // ログインし直せてしまう。新しいセッションを作らせないのがこの関数の役目。
      create: {
        before: async (session) => {
          const [user] = await select<{ bannedAt: Date | null }>(
            "SELECT bannedAt FROM `user` WHERE id = ?",
            [session.userId]
          );
          if (user?.bannedAt) {
            // false を返してもログインは止まるが、画面には「Failed to create session」しか
            // 出ない。理由が利用者に伝わるよう、文言を持った APIError を投げる。
            throw new APIError("FORBIDDEN", {
              message: "このアカウントは利用を停止されています。",
            });
          }
        },
      },
    },
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
