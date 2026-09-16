import { afterEach, describe, expect, it, vi } from "vitest";
import { createGoal, fetchGoals } from "./useGoals";

// フック本体（useMutation の配線）ではなく、通信と分岐を持つ関数を直接試す。
// fetchGoals と同じく、フックの外に出してある部分が対象。
function stubFetch(response: Response) {
  const fetchMock = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createGoal", () => {
  it("候補（candidate）として追加する", async () => {
    const fetchMock = stubFetch(json({ id: 1 }, 201));

    const result = await createGoal(7);

    expect(result).toEqual({ duplicated: false });
    expect(fetchMock).toHaveBeenCalledWith("/api/goals", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      // 探す画面からの追加は必ず candidate。受験校の確定は志望校ページで行う。
      body: JSON.stringify({ facultyId: 7, status: "candidate" }),
    });
  });

  it("409 は失敗にせず duplicated として返す", async () => {
    stubFetch(json({ error: "この学部はすでに登録されています" }, 409));

    await expect(createGoal(7)).resolves.toEqual({ duplicated: true });
  });

  it("409 以外のエラーはそのまま投げる", async () => {
    stubFetch(json({ error: "不正な学部です" }, 400));

    await expect(createGoal(7)).rejects.toThrow("不正な学部です");
  });

  it("本文が読めないエラーは既定の文言で投げる", async () => {
    stubFetch(new Response("<html>502</html>", { status: 502 }));

    await expect(createGoal(7)).rejects.toThrow("追加に失敗しました");
  });

  it("通信自体の失敗は duplicated に化けさせない", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("Failed to fetch")));

    await expect(createGoal(7)).rejects.toBeInstanceOf(TypeError);
  });
});

describe("fetchGoals", () => {
  it("一覧をそのまま返す", async () => {
    stubFetch(json([{ id: 1, status: "candidate" }]));

    await expect(fetchGoals()).resolves.toEqual([
      { id: 1, status: "candidate" },
    ]);
  });

  it("失敗したら取得用の文言で投げる", async () => {
    stubFetch(new Response(null, { status: 500 }));

    await expect(fetchGoals()).rejects.toThrow("目標の取得に失敗しました");
  });
});
