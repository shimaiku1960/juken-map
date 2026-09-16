import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "./api-client";

// fetch を差し替えて、渡した Response をそのまま返す。
// Node 24 の Response をそのまま使うので、本文の読み方（204・空ボディ・
// 壊れた JSON）は実物と同じ経路で確かめられる。
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

// reject された理由は unknown なので、ApiError であることを確かめてから型を絞る。
async function catchApiError(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.catch((cause: unknown) => cause);
  expect(error).toBeInstanceOf(ApiError);
  return error as ApiError;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("api の送信内容", () => {
  it("GET は Content-Type もボディも付けない", async () => {
    const fetchMock = stubFetch(json([{ id: 1 }]));

    await api.get("/api/goals");

    expect(fetchMock).toHaveBeenCalledWith("/api/goals", { method: "GET" });
  });

  it("POST はボディを JSON にして Content-Type を付ける", async () => {
    const fetchMock = stubFetch(json({ id: 1 }, 201));

    await api.post("/api/goals", { facultyId: 3, status: "candidate" });

    expect(fetchMock).toHaveBeenCalledWith("/api/goals", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ facultyId: 3, status: "candidate" }),
    });
  });

  it("DELETE はボディを付けない", async () => {
    const fetchMock = stubFetch(json({ message: "Deleted" }));

    await api.del("/api/goals/1");

    expect(fetchMock).toHaveBeenCalledWith("/api/goals/1", {
      method: "DELETE",
    });
  });

  it("PATCH と PUT もボディを JSON にする", async () => {
    const patchMock = stubFetch(json({ ok: true }));
    await api.patch("/api/goals/1", { isFirstChoice: true });
    expect(patchMock.mock.calls[0][1]).toMatchObject({ method: "PATCH" });

    const putMock = stubFetch(json({ ok: true }));
    await api.put("/api/profile", { nickname: "ぬし" });
    expect(putMock.mock.calls[0][1]).toMatchObject({
      method: "PUT",
      body: JSON.stringify({ nickname: "ぬし" }),
    });
  });

  it("signal を渡すと fetch へ引き継ぐ", async () => {
    const fetchMock = stubFetch(json([]));
    const controller = new AbortController();

    await api.get("/api/goals", { signal: controller.signal });

    expect(fetchMock.mock.calls[0][1].signal).toBe(controller.signal);
  });
});

describe("成功応答の読み取り", () => {
  it("JSON をそのまま返す", async () => {
    stubFetch(json([{ id: 1, name: "早稲田大学" }]));

    await expect(api.get("/api/universities")).resolves.toEqual([
      { id: 1, name: "早稲田大学" },
    ]);
  });

  it("204 No Content でも落ちずに undefined を返す", async () => {
    stubFetch(new Response(null, { status: 204 }));

    await expect(api.del("/api/goals/1")).resolves.toBeUndefined();
  });

  it("本文が空でも落ちずに undefined を返す", async () => {
    stubFetch(new Response("", { status: 200 }));

    await expect(api.get("/api/goals")).resolves.toBeUndefined();
  });
});

describe("エラー応答の吸収", () => {
  it("{error: string} をそのままメッセージにする", async () => {
    stubFetch(json({ error: "この学部はすでに登録されています" }, 409));

    await expect(api.post("/api/goals", {})).rejects.toThrow(
      "この学部はすでに登録されています"
    );
  });

  it("{error: ZodIssue[]} は最初の issue の message を使う", async () => {
    stubFetch(
      json(
        {
          error: [
            { path: ["minutes"], message: "1分以上で入力してください" },
            { path: ["date"], message: "日付を選んでください" },
          ],
        },
        400
      )
    );

    await expect(api.post("/api/study-logs", {})).rejects.toThrow(
      "1分以上で入力してください"
    );
  });

  it("message を持たない issue は読み飛ばす", async () => {
    stubFetch(json({ error: [{ path: ["minutes"] }, { message: "必須です" }] }, 400));

    await expect(
      api.post("/api/study-logs", {}, { fallbackMessage: "記録に失敗しました" })
    ).rejects.toThrow("必須です");
  });

  it("本文から取り出せないときは fallbackMessage を使う", async () => {
    stubFetch(new Response("<html>502 Bad Gateway</html>", { status: 502 }));

    await expect(
      api.get("/api/goals", { fallbackMessage: "目標の取得に失敗しました" })
    ).rejects.toThrow("目標の取得に失敗しました");
  });

  it("fallbackMessage が無いときは既定の文言を使う", async () => {
    stubFetch(new Response(null, { status: 500 }));

    await expect(api.get("/api/goals")).rejects.toThrow("通信に失敗しました");
  });

  it("error が空文字のときも fallbackMessage に落とす", async () => {
    stubFetch(json({ error: "" }, 400));

    await expect(
      api.post("/api/goals", {}, { fallbackMessage: "追加に失敗しました" })
    ).rejects.toThrow("追加に失敗しました");
  });
});

describe("ApiError", () => {
  it("status で呼び出し側が分岐できる（409 の重複登録）", async () => {
    stubFetch(json({ error: "この学部はすでに登録されています" }, 409));

    const error = await catchApiError(api.post("/api/goals", {}));

    expect(error.status).toBe(409);
  });

  it("Error として catch でき、name が ApiError である", async () => {
    stubFetch(json({ error: "Not found" }, 404));

    const error = await catchApiError(api.get("/api/goals/999"));

    expect(error).toBeInstanceOf(Error);
    expect(error.name).toBe("ApiError");
    expect(error.status).toBe(404);
  });

  it("body に応答本文を残すので issue 配列を後から読める", async () => {
    const issues = [{ path: ["minutes"], message: "1分以上で入力してください" }];
    stubFetch(json({ error: issues }, 400));

    const error = await catchApiError(api.post("/api/study-logs", {}));

    expect(error.body).toEqual({ error: issues });
  });

  it("通信自体の失敗は包み直さずそのまま投げる", async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));
    vi.stubGlobal("fetch", fetchMock);

    const error = await api.get("/api/goals").catch((cause: unknown) => cause);

    expect(error).toBeInstanceOf(TypeError);
    expect(error).not.toBeInstanceOf(ApiError);
  });
});
