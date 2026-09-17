import {
  createClient,
  isMicroCMSRequestError,
  type MicroCMSImage,
} from "microcms-js-sdk";
import { abortAfter } from "@/api/infra/timeout";

const client = createClient({
  serviceDomain: process.env.MICROCMS_SERVICE_DOMAIN!,
  apiKey: process.env.MICROCMS_API_KEY!,
});

// createClient には fetch の設定を渡せないので、呼び出しごとに signal を添える。
// client をそのまま公開せず下の2つだけを出すのは、呼び出し側が上限を付け忘れないようにするため。
//
// 既定（5秒）より短くしているのは、記事の取得が HTML を返す途中で走るため
// （apps/api/src/seo.ts の metaForPath）。ここで長く待つと、利用者は真っ白な画面を
// その間ずっと見ることになる。記事が取れなくても画面は SPA が描けるので、早く諦めてよい。
const MICROCMS_TIMEOUT_MS = 3_000;

function requestInit() {
  return { customRequestInit: { signal: abortAfter(MICROCMS_TIMEOUT_MS) } };
}

/** 記事を1件取る。存在しない contentId のときは SDK が例外を投げる。 */
export function getBlog(contentId: string) {
  return client.get<Blog>({ endpoint: "blogs", contentId, ...requestInit() });
}

/**
 * その失敗が「記事が存在しない」かどうか。
 *
 * microCMS は存在しない contentId でも例外を投げるので、呼び出し側が例外を一括で
 * 404 に読み替えると、タイムアウトや microCMS の障害まで「記事が無い」ことにしてしまう。
 */
export function isBlogNotFound(error: unknown) {
  return isMicroCMSRequestError(error) && error.status === 404;
}

/** 記事の一覧を取る。 */
export function listBlogs(queries?: { fields?: string; limit?: number }) {
  return client.getList<Blog>({ endpoint: "blogs", queries, ...requestInit() });
}

export type Blog = {
  id: string;
  title: string;
  description?: string;
  content: string;
  eyecatch?: MicroCMSImage;
  createdAt: string;
  updatedAt: string;
};
