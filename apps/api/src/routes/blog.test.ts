import { beforeEach, describe, expect, it, vi } from "vitest";
import { buildTestApp, request } from "@/api/test-support";

// microCMS の SDK ごと差し替える。ここで見たいのは「相手が失敗したとき何を返すか」で、
// 実際に microCMS を叩く必要はない（API キーも要らなくなる）。
const getBlog = vi.fn();
const listBlogs = vi.fn();
const isBlogNotFound = vi.fn();

vi.mock("@/api/infra/microcms", () => ({
  getBlog: (...args: unknown[]) => getBlog(...args),
  listBlogs: (...args: unknown[]) => listBlogs(...args),
  isBlogNotFound: (...args: unknown[]) => isBlogNotFound(...args),
}));

const { registerBlogRoutes } = await import("@/api/routes/blog");

function app() {
  return buildTestApp((app) => registerBlogRoutes(app));
}

describe("GET /api/blog/:id", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    isBlogNotFound.mockReturnValue(false);
  });

  it("記事が取れたらそのまま返す", async () => {
    getBlog.mockResolvedValue({ id: "abc", title: "記事" });

    const res = await request(app(), "GET", "/api/blog/abc");

    expect(res.statusCode).toBe(200);
    expect(res.json()).toMatchObject({ id: "abc" });
    expect(getBlog).toHaveBeenCalledWith("abc");
  });

  it("記事が存在しないときだけ 404 を返す", async () => {
    getBlog.mockRejectedValue(new Error("not found"));
    isBlogNotFound.mockReturnValue(true);

    const res = await request(app(), "GET", "/api/blog/missing");

    expect(res.statusCode).toBe(404);
  });

  it("タイムアウトは 404 ではなく 502 を返す（記事が無いことにしない）", async () => {
    const timeout = Object.assign(new Error("The operation was aborted due to timeout"), {
      name: "TimeoutError",
    });
    getBlog.mockRejectedValue(timeout);

    const res = await request(app(), "GET", "/api/blog/abc");

    expect(res.statusCode).toBe(502);
    expect(res.json()).toEqual({ error: "Bad Gateway" });
  });
});

describe("GET /api/blog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    isBlogNotFound.mockReturnValue(false);
  });

  it("一覧を返す", async () => {
    listBlogs.mockResolvedValue({ contents: [], totalCount: 0 });

    const res = await request(app(), "GET", "/api/blog");

    expect(res.statusCode).toBe(200);
  });

  it("microCMS が落ちていたら 500 ではなく 502 を返す", async () => {
    listBlogs.mockRejectedValue(new Error("upstream down"));

    const res = await request(app(), "GET", "/api/blog");

    expect(res.statusCode).toBe(502);
  });
});
