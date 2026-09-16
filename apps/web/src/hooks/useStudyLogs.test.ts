import { afterEach, describe, expect, it, vi } from "vitest";
import { saveStudySession } from "./useStudyLogs";

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

const timerInput = {
  planId: null,
  data: {
    date: "2026-09-16",
    minutes: 45,
    subject: "english",
    textbookId: 3,
    rangeStart: 10,
    rangeEnd: 20,
    rangeUnit: "page",
    memo: "長文3題",
  },
} as const;

const planInput = {
  planId: 12,
  data: {
    minutes: 45,
    rangeStart: 10,
    rangeEnd: 20,
    rangeUnit: "page",
    memo: "長文3題",
  },
} as const;

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("saveStudySession の呼び先の出し分け", () => {
  it("予定が無いときは学習実績として記録する", async () => {
    const fetchMock = stubFetch(json({ isFirstStudyLog: false }, 201));

    await saveStudySession(timerInput);

    expect(fetchMock.mock.calls[0][0]).toBe("/api/study-logs");
    expect(fetchMock.mock.calls[0][1].body).toBe(
      JSON.stringify(timerInput.data)
    );
  });

  it("予定があるときはその予定の完了として記録する", async () => {
    const fetchMock = stubFetch(json({ isFirstStudyLog: false }, 201));

    await saveStudySession(planInput);

    expect(fetchMock.mock.calls[0][0]).toBe("/api/study-plans/12/complete");
    // 日付・科目・参考書は予定が持っているのでサーバーが引き継ぐ。送らない。
    expect(fetchMock.mock.calls[0][1].body).toBe(JSON.stringify(planInput.data));
  });
});

describe("saveStudySession が返す計測用の値", () => {
  it("予定なしは timer、予定ありは plan として数える", async () => {
    stubFetch(json({ isFirstStudyLog: false }, 201));
    await expect(saveStudySession(timerInput)).resolves.toMatchObject({
      recordMethod: "timer",
    });

    stubFetch(json({ isFirstStudyLog: false }, 201));
    await expect(saveStudySession(planInput)).resolves.toMatchObject({
      recordMethod: "plan",
    });
  });

  it("初回の実績かどうかを応答から受け取る", async () => {
    stubFetch(json({ isFirstStudyLog: true }, 201));

    await expect(saveStudySession(timerInput)).resolves.toMatchObject({
      isFirstStudyLog: true,
    });
  });

  it("応答に isFirstStudyLog が無いときは false に寄せる", async () => {
    stubFetch(json({ log: { id: 1 } }, 201));

    await expect(saveStudySession(timerInput)).resolves.toMatchObject({
      isFirstStudyLog: false,
    });
  });
});

describe("saveStudySession の失敗", () => {
  it("すでに記録済みの予定（409）はサーバーの文言で投げる", async () => {
    stubFetch(json({ error: "この予定の実績はすでに記録されています" }, 409));

    await expect(saveStudySession(planInput)).rejects.toThrow(
      "この予定の実績はすでに記録されています"
    );
  });

  it("入力エラー（Zod の issue 配列）も1行にして投げる", async () => {
    stubFetch(
      json({ error: [{ path: ["minutes"], message: "1分以上を入力してください" }] }, 400)
    );

    await expect(saveStudySession(timerInput)).rejects.toThrow(
      "1分以上を入力してください"
    );
  });

  it("本文が読めないときは保存用の既定の文言で投げる", async () => {
    stubFetch(new Response(null, { status: 500 }));

    await expect(saveStudySession(timerInput)).rejects.toThrow(
      "実績を保存できませんでした"
    );
  });
});
