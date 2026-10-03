import { betterAuth } from "better-auth";
import { APIError, createAuthMiddleware, isAPIError } from "better-auth/api";
import { twoFactor } from "better-auth/plugins";
import { pool, select } from "@/api/infra/db";
import {
  notifyAdminOfNewUser,
  sendPasswordChangedNotice,
  sendVerificationEmail,
  sendPasswordResetEmail,
} from "@/api/infra/email";
import { clearSignInAttempts, recordSignInAttempt } from "@/api/sign-in-throttle";
import { DEMO_EMAIL } from "@/shared/demo";

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
  session: {
    additionalFields: {
      // 2段階認証を通して作られたセッションか（下の session.create.before で付ける）。
      // twoFactor はメール＋パスワードのログインにしかかからず、Google / GitHub は素通りする。
      // 利用者の設定（twoFactorEnabled）ではなくセッションで見るのはそのため。
      twoFactorVerified: {
        type: "boolean",
        required: false,
        defaultValue: false,
        input: false,
      },
    },
  },
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
        before: async (session, context) => {
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
          // 認証コード・予備コードを確かめた直後に作るセッションにだけ印を付ける。
          // セッションを作り直すとき（有効化の確認など）は前のセッションの値が引き継がれて
          // くるので、ここで毎回上書きする。管理 API はこの印を見る（context.ts の requireAdmin）。
          return {
            data: { ...session, twoFactorVerified: isTwoFactorVerification(context?.path) },
          };
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
    // 再設定したら、その人のセッションを全部消す（乗っ取った人の画面も落ちる。セキュリティ基準 B6）。
    // 再設定の画面はログインしていない状態で使うので、消して困るセッションは無い。
    revokeSessionsOnPasswordReset: true,
    onPasswordReset: async ({ user }) => {
      await sendPasswordChangedNotice(user.email);
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
  advanced: {
    // Cookie 付きの書き込みは、Origin（無ければ Referer）が trustedOrigins に無ければ 403 にする（セキュリティ基準 D3、CSRF）。
    // Better Auth の既定は「NODE_ENV=test のときだけ確かめない」で、本番は確かめるがテストでは素通りになる。
    // テストでも本番と同じ判定を通すため、どの環境でも確かめると明示する（auth.origin-check.test.ts）。
    disableOriginCheck: false,
    disableCSRFCheck: false,
    // 確認メール・再設定メールを送り終えるのを待たずに応答する（セキュリティ基準 B4）。
    // 待つと、登録済みのメールアドレスだけ Resend の分だけ遅く返り、応答時間で登録の有無が分かる。
    // 送れなかったときに画面へ伝えないのは今までと同じ（Better Auth は待っていたときも
    // 失敗をログに残して握りつぶしていた）。渡される promise は Better Auth が catch 済み。
    backgroundTasks: {
      handler: (promise) => {
        void promise;
      },
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
  hooks: {
    before: createAuthMiddleware(async (ctx) => {
      // アカウント単位の回数制限（セキュリティ基準 B4。IP 単位は Better Auth の rateLimit）。
      // デモアカウントはパスワードを画面に載せていて守る意味が無く、わざと失敗させれば
      // 面接官が入れなくなるので数えない。
      if (ctx.path === "/sign-in/email") {
        const email: unknown = ctx.body?.email;
        if (typeof email === "string" && email !== DEMO_EMAIL) {
          const attempt = await recordSignInAttempt(email);
          if (!attempt.allowed) {
            throw new APIError(
              "TOO_MANY_REQUESTS",
              {
                code: "TOO_MANY_SIGN_IN_ATTEMPTS",
                message: "ログインの試行が多すぎます。しばらく待ってから、もう一度お試しください。",
              },
              { "X-Retry-After": String(attempt.retryAfterSeconds) }
            );
          }
        }
      }
      // パスワードを変えたら、ほかのセッションを必ず消す（セキュリティ基準 B6）。Better Auth は
      // 本文の revokeOtherSessions に任せていて既定は消さないので、何が送られてきても true にする。
      // 消したあと今の端末には新しいセッションが発行されるので、変更した本人はログインしたまま。
      // ただし新しいセッションには2段階認証の印が付かないので、管理者は入り直すことになる。
      if (ctx.path === "/change-password") {
        return { context: { body: { ...ctx.body, revokeOtherSessions: true } } };
      }
    }),
    after: createAuthMiddleware(async (ctx) => {
      // パスワードが合っていたら数を消す。メール未確認（EMAIL_NOT_VERIFIED）はパスワードを
      // 確かめたあとに断られるので、これも合っていた扱いにする。
      if (ctx.path === "/sign-in/email") {
        const returned = ctx.context.returned;
        const email: unknown = ctx.body?.email;
        const passwordMatched =
          !isAPIError(returned) || returned.body?.code === "EMAIL_NOT_VERIFIED";
        if (passwordMatched && typeof email === "string") await clearSignInAttempts(email);
      }
      // 変更できたときだけ知らせる（今のパスワードが違うなどで断ったときは returned が APIError）。
      if (ctx.path === "/change-password") {
        const returned = ctx.context.returned as { user?: { email?: string } } | undefined;
        if (!isAPIError(returned) && returned?.user?.email) {
          await sendPasswordChangedNotice(returned.user.email);
        }
      }
    }),
  },
  plugins: [
    // 管理者の2段階認証（認証アプリの TOTP＋予備コード）。有効にできるのは誰でもだが、
    // 求めるのは管理 API だけ。信頼済みの端末（trustDevice）で省いたログインは
    // /sign-in/email のままセッションができるので印が付かず、管理 API は通らない。
    twoFactor({ issuer: "受験マップ" }),
  ],
  trustedOrigins: [
    "https://juken-map.com",
    "https://www.juken-map.com",
    "http://localhost:5173",
    "http://localhost:4000",
  ],
});

/** 認証コード（/two-factor/verify-totp）・予備コード（/two-factor/verify-backup-code）の確認か。 */
export function isTwoFactorVerification(path: string | undefined) {
  return path?.startsWith("/two-factor/verify-") ?? false;
}
