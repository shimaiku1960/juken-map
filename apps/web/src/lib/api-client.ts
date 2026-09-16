// フロントから apps/api を呼ぶときの唯一の入口。
//
// これを作る前は、res.ok の確認とエラーメッセージの取り出しが呼び出し側に
// 4通り散っていた（固定文言で throw / err.error ?? 既定値 / typeof ガード /
// responseError() の重複定義）。API は 400 のとき {error: string} を返す場合と
// {error: ZodIssue[]} を返す場合があり、その差をどこで吸収するかが決まって
// いなかったのが原因。吸収はこのファイルの中だけでやる。

// サーバーが返したエラー。status を持たせているのは、呼び出し側が 409 などを
// 特別扱いできるようにするため（例: 志望校の重複登録はエラーにせず「登録済み」
// として扱う）。body は Zod の issue 配列をそのまま見たいときのために残す。
export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(message: string, status: number, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

type RequestOptions = {
  // 応答本文からメッセージを取り出せなかったときに使う文言。
  // 画面ごとに「削除に失敗しました」などと出し分けたいので呼び出し側が渡す。
  fallbackMessage?: string;
  signal?: AbortSignal;
};

const DEFAULT_FALLBACK = "通信に失敗しました";

// 本文を JSON として読む。204/205 と空ボディ、JSON でない応答（502 の HTML など）
// でも落とさず undefined を返す。res.json() を直接呼ぶと、この3つで例外になる。
async function readBody(response: Response): Promise<unknown> {
  if (response.status === 204 || response.status === 205) return undefined;

  const text = await response.text().catch(() => "");
  if (text === "") return undefined;

  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

type IssueLike = { message?: unknown };

// {error: string} と {error: ZodIssue[]} の両方から、画面に出す1行を取り出す。
// どちらでもなければ呼び出し側の既定文言に落とす。
function errorMessage(body: unknown, fallback: string): string {
  const error = (body as { error?: unknown } | undefined)?.error;

  if (typeof error === "string" && error !== "") return error;

  if (Array.isArray(error)) {
    const issue = (error as IssueLike[]).find(
      (candidate) => typeof candidate?.message === "string" && candidate.message !== ""
    );
    if (issue) return issue.message as string;
  }

  return fallback;
}

async function request<T>(
  method: string,
  path: string,
  body: unknown,
  options: RequestOptions = {}
): Promise<T> {
  // 通信自体が失敗したとき（オフライン等）fetch は TypeError を投げる。
  // ここでは包み直さずそのまま通す。ApiError は「サーバーが応答を返したうえで
  // 失敗を伝えてきた」場合だけに絞り、status の有無で区別できる状態を保つ。
  const response = await fetch(path, {
    method,
    ...(body === undefined
      ? {}
      : {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }),
    ...(options.signal ? { signal: options.signal } : {}),
  });

  const payload = await readBody(response);

  if (!response.ok) {
    throw new ApiError(
      errorMessage(payload, options.fallbackMessage ?? DEFAULT_FALLBACK),
      response.status,
      payload
    );
  }

  // 204 と空ボディはここで undefined になる。戻り値を使わない呼び出し
  // （DELETE など）は T を void にして受ける。
  return payload as T;
}

export const api = {
  get: <T>(path: string, options?: RequestOptions) =>
    request<T>("GET", path, undefined, options),

  post: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>("POST", path, body, options),

  patch: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>("PATCH", path, body, options),

  put: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>("PUT", path, body, options),

  // delete は予約語なので del。呼び出し側で api.delete と書けないのは
  // このためで、意図的な命名である。
  del: <T>(path: string, options?: RequestOptions) =>
    request<T>("DELETE", path, undefined, options),
};
