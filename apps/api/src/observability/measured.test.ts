import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { measured } from "@/api/observability/measured";
import { takeLogLines } from "@/api/test-support";

describe("measured", () => {
  beforeEach(() => {
    takeLogLines();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("成功時の実行時間をログに書き出す", async () => {
    vi.spyOn(performance, "now")
      .mockReturnValueOnce(100)
      .mockReturnValueOnce(112.5);

    await expect(measured("example.success", async () => 42)).resolves.toBe(42);

    expect(takeLogLines()).toEqual([
      expect.objectContaining({
        operation: "example.success",
        duration_ms: 12.5,
        success: true,
      }),
    ]);
  });

  it("失敗時も実行時間をログに書き出して元のエラーを投げる", async () => {
    vi.spyOn(performance, "now")
      .mockReturnValueOnce(200)
      .mockReturnValueOnce(207.5);
    const error = new Error("boom");

    await expect(
      measured("example.failure", async () => {
        throw error;
      })
    ).rejects.toBe(error);

    expect(takeLogLines()).toEqual([
      expect.objectContaining({
        operation: "example.failure",
        duration_ms: 7.5,
        success: false,
      }),
    ]);
  });
});
