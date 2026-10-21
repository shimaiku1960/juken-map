import { describe, expect, it } from "vitest";
import { ApiError } from "./api-client";
import { createIdempotencyKeyHolder } from "./idempotencyKey";

function holder() {
  let n = 0;
  return createIdempotencyKeyHolder(() => `key-${++n}`);
}

const apiError = (status: number) => new ApiError("失敗", status, { error: "失敗" });

describe("createIdempotencyKeyHolder", () => {
  it("結果を受け取るまでは同じキーを返す", () => {
    const key = holder();
    expect(key.current()).toBe("key-1");
    expect(key.current()).toBe("key-1");
  });

  it("成功したら次の操作は新しいキーにする", () => {
    const key = holder();
    key.current();
    key.settle();
    expect(key.current()).toBe("key-2");
  });

  it("422（このキーで保存済み）なら新しいキーにする", () => {
    const key = holder();
    key.current();
    key.settle(apiError(422));
    expect(key.current()).toBe("key-2");
  });

  it.each([
    ["通信の失敗", new TypeError("Failed to fetch")],
    ["500", apiError(500)],
    ["502", apiError(502)],
    ["400", apiError(400)],
    ["409", apiError(409)],
  ])("%s なら同じキーのまま送り直す", (_, error) => {
    const key = holder();
    key.current();
    key.settle(error);
    expect(key.current()).toBe("key-1");
  });
});
