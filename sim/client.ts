// シミュレータの HTTP クライアント。画面（ブラウザ）が送るのと同じ形で API を叩く。
//
// ブラウザがやってくれていることを、ここで代わりにやる：
//   - Cookie を覚えて次のリクエストに付ける（Node の fetch は覚えない）
//   - Origin を付ける（Cookie 付きの /api/auth への POST は Origin が無いと 403）
// シミュレータだけの約束事：
//   - X-Sim-Run ヘッダー＝ API のログに sim:true が付き、Grafana で実ユーザーと分けて読める
//   - ログイン・登録の間隔を空ける（Better Auth の組み込み制限が 10 秒 3 回）
//   - 429 は指定秒だけ待って1回だけやり直す

export class HttpError extends Error {
  constructor(
    readonly method: string,
    readonly path: string,
    readonly status: number,
    readonly body: string
  ) {
    super(`${method} ${path} → ${status} ${body.slice(0, 200)}`);
  }
}

export type ClientOptions = {
  baseUrl: string;
  /** Origin に付ける画面の住所。ブラウザは API ではなく画面のオリジンを送る */
  origin: string;
  runId: string;
  /** /api/auth/sign-in と sign-up の最低間隔（ミリ秒） */
  authIntervalMs: number;
  /** その他すべてのリクエストの最低間隔（ミリ秒） */
  requestIntervalMs: number;
};

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

/** 1人分のブラウザ。Cookie はこのインスタンスが持つ。 */
export class Browser {
  cookie: string | undefined;

  constructor(
    private readonly shared: SharedPacer,
    initialCookie?: string
  ) {
    this.cookie = initialCookie;
  }

  async request<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
    const response = await this.raw(method, path, body);
    const text = await response.text();
    if (!response.ok) throw new HttpError(method, path, response.status, text);
    return (text ? JSON.parse(text) : undefined) as T;
  }

  /** 応答をそのまま返す（ステータスで分岐したいとき用）。 */
  async raw(method: string, path: string, body?: unknown, retried = false): Promise<Response> {
    const { options } = this.shared;
    await this.shared.wait(path.startsWith("/api/auth/sign-") ? "auth" : "request");

    const headers: Record<string, string> = {
      Origin: options.origin,
      "User-Agent": "juken-map-sim/1",
      "X-Sim-Run": options.runId,
    };
    if (body !== undefined) headers["Content-Type"] = "application/json";
    if (this.cookie) headers.Cookie = this.cookie;

    const response = await fetch(new URL(path, options.baseUrl), {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
      redirect: "manual",
    });
    this.shared.count(response.status);

    // Better Auth は Set-Cookie でセッションを渡す。名前（本番は __Secure- 付き）に
    // 依存しないよう、返ってきたものを名前ごとそのまま覚える。
    const setCookies = response.headers.getSetCookie();
    if (setCookies.length > 0) this.cookie = mergeCookies(this.cookie, setCookies);

    if (response.status === 429 && !retried) {
      const retryAfter = Number(response.headers.get("X-Retry-After") ?? "10");
      await response.body?.cancel();
      await sleep((retryAfter + 1) * 1000);
      return this.raw(method, path, body, true);
    }
    return response;
  }
}

/** Cookie ヘッダーの文字列に、Set-Cookie の値を上書き・追加する。Max-Age=0 は消す。 */
export function mergeCookies(current: string | undefined, setCookies: string[]) {
  const jar = new Map<string, string>();
  for (const pair of (current ?? "").split(";")) {
    const [name, ...rest] = pair.trim().split("=");
    if (name) jar.set(name, rest.join("="));
  }
  for (const header of setCookies) {
    const [pair, ...attributes] = header.split(";");
    const [name, ...rest] = pair.trim().split("=");
    const expired = attributes.some((a) => a.trim().toLowerCase() === "max-age=0");
    if (expired) jar.delete(name);
    else jar.set(name, rest.join("="));
  }
  const value = [...jar].map(([name, v]) => `${name}=${v}`).join("; ");
  return value || undefined;
}

/**
 * 全員で共有する間隔の管理とステータスの集計。
 * 同じランナー（同じ IP）から出るので、ログインの制限は全員の合計に掛かる。
 */
export class SharedPacer {
  private lastAuthAt = 0;
  private lastRequestAt = 0;
  readonly statusCounts = new Map<number, number>();

  constructor(readonly options: ClientOptions) {}

  async wait(kind: "auth" | "request") {
    const now = Date.now();
    const gap =
      kind === "auth"
        ? Math.max(this.options.authIntervalMs - (now - this.lastAuthAt), 0)
        : 0;
    const requestGap = Math.max(this.options.requestIntervalMs - (now - this.lastRequestAt), 0);
    const delay = Math.max(gap, requestGap);
    if (delay > 0) await sleep(delay);
    const at = Date.now();
    this.lastRequestAt = at;
    if (kind === "auth") this.lastAuthAt = at;
  }

  count(status: number) {
    this.statusCounts.set(status, (this.statusCounts.get(status) ?? 0) + 1);
  }
}

/** シミュレーション専用 API（/api/sim/*）。共有シークレットで認証する。 */
export class SimApi {
  constructor(
    private readonly baseUrl: string,
    private readonly secret: string
  ) {}

  private async call<T>(method: string, path: string, body?: unknown): Promise<T> {
    const response = await fetch(new URL(path, this.baseUrl), {
      method,
      headers: {
        Authorization: `Bearer ${this.secret}`,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    if (!response.ok) throw new HttpError(method, path, response.status, text);
    return (text ? JSON.parse(text) : undefined) as T;
  }

  state() {
    return this.call<{
      nextSeq: number;
      users: {
        seq: number;
        email: string;
        cohort: string;
        createdAt: string;
        dormantFrom: string | null;
        lastActedOn: string | null;
      }[];
    }>("GET", "/api/sim/state");
  }

  markUser(email: string, data: { seq: number; cohort: string }) {
    return this.call<void>("POST", "/api/sim/users", { email, ...data });
  }

  updateUser(seq: number, data: { lastActedOn?: string; dormantFrom?: string }) {
    return this.call<void>("PATCH", `/api/sim/users/${seq}`, data);
  }
}
