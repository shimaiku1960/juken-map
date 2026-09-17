import { describe, expect, it } from "vitest";
import { abortAfter, withDeadline, EXTERNAL_TIMEOUT_MS } from "@/api/infra/timeout";

describe("abortAfter", () => {
  it("決めた時間で中断する", async () => {
    const signal = abortAfter(20);
    expect(signal.aborted).toBe(false);
    await new Promise((resolve) => setTimeout(resolve, 40));
    expect(signal.aborted).toBe(true);
    expect((signal.reason as Error).name).toBe("TimeoutError");
  });

  it("呼び出し側の signal が先に止まれば、そちらで中断する", () => {
    const caller = new AbortController();
    const signal = abortAfter(EXTERNAL_TIMEOUT_MS, caller.signal);

    caller.abort(new Error("caller stopped"));

    expect(signal.aborted).toBe(true);
    expect((signal.reason as Error).message).toBe("caller stopped");
  });

  it("呼び出し側の signal が無くても使える（null を渡しても落ちない）", () => {
    expect(abortAfter(1_000, null).aborted).toBe(false);
  });
});

describe("withDeadline", () => {
  it("時間内に終われば、その結果を返す", async () => {
    await expect(withDeadline(Promise.resolve("ok"), "test", 1_000)).resolves.toBe("ok");
  });

  it("時間を超えたら、どこで諦めたか分かる形で失敗する", async () => {
    const stuck = new Promise<string>(() => {});
    await expect(withDeadline(stuck, "resend.test", 20)).rejects.toThrow(
      "resend.test timed out after 20ms"
    );
  });

  it("元の失敗はそのまま伝える", async () => {
    const failing = Promise.reject(new Error("upstream failed"));
    await expect(withDeadline(failing, "test", 1_000)).rejects.toThrow("upstream failed");
  });

  it("先に終わったらタイマーを残さない（プロセスが上限まで待たされない）", async () => {
    const before = process.getActiveResourcesInfo().filter((r) => r === "Timeout").length;
    await withDeadline(Promise.resolve("ok"), "test", 60_000);
    const after = process.getActiveResourcesInfo().filter((r) => r === "Timeout").length;
    expect(after).toBe(before);
  });
});
