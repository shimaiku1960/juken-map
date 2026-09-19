/**
 * SPA が描けるパスの一覧（apps/web/src/App.tsx の <Route path> と対）。
 *
 * サーバーは誰が来ても index.html を返すため、これが無いと存在しないパスまで 200 になる
 * （本番ログで /wp-login.php のようなボットのスキャンが全部「正常」に計上されていた）。
 * ここを正として突き合わせ、一致しないパスは同じ HTML を 404 で返す
 * （apps/api/src/server.ts の setNotFoundHandler）。
 *
 * App.tsx と二重管理になるので、ずれていないことは
 * apps/web/src/App.routes.test.ts が機械的に確かめる。
 */
export const SPA_ROUTES = [
  // 公開
  "/",
  "/blog",
  "/articles/:id",
  "/terms",
  "/privacy",
  "/lp",

  // 認証フロー
  "/login",
  "/signup",
  "/forgot-password",
  "/reset-password",
  "/verify-email",

  // ログイン必須
  "/dashboard",
  "/goals",
  "/explore",
  "/explore/:universityId",
  "/profile",
  "/schedule",
  "/admin",
  "/admin/masters",

  // LINE のトークから開く
  "/line/link",
] as const;

// ":id" のような部分は「/ を含まない1区切り」として扱う。ルートの文字列に
// 正規表現の特殊文字は入らないので、そのまま組み立ててよい。
const matchers = SPA_ROUTES.map(
  (route) => new RegExp(`^${route.replaceAll(/:[^/]+/g, "[^/]+")}$`)
);

/** そのパスを SPA が描けるか（＝ index.html を 200 で返してよいか）。 */
export function isKnownSpaRoute(pathname: string): boolean {
  // react-router は "/login/" を "/login" と同じ画面として扱うので、判定も揃える。
  const normalized =
    pathname.length > 1 ? pathname.replace(/\/+$/, "") || "/" : pathname;
  return matchers.some((matcher) => matcher.test(normalized));
}
