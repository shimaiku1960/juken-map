import { useQuery } from "@tanstack/react-query";

// ログインの API（/api/auth/*、apps/api-go の auth_handlers.go）を呼ぶ唯一の入口（JUK-115）。
// Better Auth のクライアントを置き換えた。呼び出し側が分岐しやすいよう、どの関数も
// { data, error } を返し、例外は投げない（通信そのものの失敗も error にする）。
//
// SPA は API と同じオリジンで配信するので、Cookie は fetch の既定（same-origin）で付く。
// セッションの Cookie は HttpOnly で、画面の JS からは読めない（認証基準 10 の D3）。
// ログインしているかは GET /api/auth/session で聞く。

export type SessionUser = {
  id: string;
  email: string;
  name: string;
  nickname: string | null;
  image: string | null;
  role: string;
  emailVerified: boolean;
  twoFactorEnabled: boolean;
  createdAt: string;
};

export type Session = {
  user: SessionUser;
  session: { id: string; expiresAt: string; twoFactorVerified: boolean };
};

export type AuthError = { status: number; code?: string; message: string };
export type AuthResult<T> = { data: T; error: null } | { data: null; error: AuthError };

async function call<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<AuthResult<T>> {
  let response: Response;
  try {
    response = await fetch(path, {
      method,
      ...(body === undefined
        ? {}
        : { headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
    });
  } catch {
    return { data: null, error: { status: 0, message: "通信に失敗しました。接続を確かめてください" } };
  }
  const text = await response.text().catch(() => "");
  let payload: unknown = undefined;
  try {
    payload = text === "" ? undefined : JSON.parse(text);
  } catch {
    // 502 の HTML など。下で既定の文言にする。
  }
  if (!response.ok) {
    const { error, code } = (payload ?? {}) as { error?: unknown; code?: unknown };
    return {
      data: null,
      error: {
        status: response.status,
        code: typeof code === "string" ? code : undefined,
        message: typeof error === "string" && error !== "" ? error : "うまくいきませんでした。時間をおいてお試しください",
      },
    };
  }
  return { data: payload as T, error: null };
}

type Ok = { ok: true };

export const authClient = {
  signUp: (input: { email: string; password: string; callbackURL?: string }) =>
    call<Ok>("POST", "/api/auth/sign-up", input),

  /** mfaRequired が true なら、まだログインしていない。2段階認証のコードを求める。 */
  signIn: (input: { email: string; password: string }) =>
    call<{ mfaRequired: boolean }>("POST", "/api/auth/sign-in", input),

  /** Google・GitHub の同意画面へ移る。移れなかったときだけ error が返る。 */
  async signInSocial(input: { provider: "google" | "github"; callbackURL: string }) {
    const result = await call<{ url: string }>("POST", `/api/auth/oauth/${input.provider}`, {
      callbackURL: input.callbackURL,
    });
    if (result.data) window.location.href = result.data.url;
    return result;
  },

  signOut: () => call<Ok>("POST", "/api/auth/sign-out", {}),

  verifyEmail: (input: { token: string }) => call<Ok>("POST", "/api/auth/verify-email", input),

  resendVerification: (input: { email: string; callbackURL?: string }) =>
    call<Ok>("POST", "/api/auth/verify-email/resend", input),

  forgotPassword: (input: { email: string }) => call<Ok>("POST", "/api/auth/password/forgot", input),

  resetPassword: (input: { token: string; password: string }) =>
    call<Ok>("POST", "/api/auth/password/reset", input),

  changePassword: (input: { currentPassword: string; newPassword: string }) =>
    call<Ok>("POST", "/api/auth/password/change", input),

  listAccounts: () => call<{ hasPassword: boolean; providers: string[] }>("GET", "/api/auth/accounts"),

  mfa: {
    setup: (input: { password: string }) =>
      call<{ totpURI: string; backupCodes: string[] }>("POST", "/api/auth/mfa/setup", input),
    confirm: (input: { code: string }) => call<Ok>("POST", "/api/auth/mfa/confirm", input),
    verify: (input: { code: string; method: "totp" | "backup" }) =>
      call<{ ok: true; backupCodesRemaining: number }>("POST", "/api/auth/mfa/verify", input),
  },
};

export const sessionQueryKey = ["auth", "session"] as const;

/**
 * 今のログイン。Better Auth の useSession と同じ形（data・isPending・refetch）にしてある。
 * ログインしていなければ data は null。
 */
export function useSession() {
  const query = useQuery({
    queryKey: sessionQueryKey,
    queryFn: async () => {
      const result = await call<Session | null>("GET", "/api/auth/session");
      if (result.error) throw new Error(result.error.message);
      return result.data ?? null;
    },
    // 画面を切り替えるたびに聞き直さない。ログイン・ログアウトはページを読み直すので、そのときに取り直す。
    staleTime: 60_000,
  });
  return { data: query.data ?? null, isPending: query.isPending, refetch: query.refetch };
}
