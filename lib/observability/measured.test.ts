import { afterEach, describe, expect, it, vi } from "vitest";
import { measured } from "@/lib/observability/measured";

describe("measured", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("成功時の実行時間をJSONで記録する", async () => {
    vi.spyOn(performance, "now")
      .mockReturnValueOnce(100)
      .mockReturnValueOnce(112.5);
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);

    await expect(measured("example.success", async () => 42)).resolves.toBe(42);

    expect(log).toHaveBeenCalledWith(
      JSON.stringify({
        operation: "example.success",
        duration_ms: 12.5,
        success: true,
      })
    );
  });

  it("失敗時も実行時間をJSONで記録して元のエラーを投げる", async () => {
    vi.spyOn(performance, "now")
      .mockReturnValueOnce(200)
      .mockReturnValueOnce(207.5);
    const log = vi.spyOn(console, "log").mockImplementation(() => undefined);
    const error = new Error("boom");

    await expect(
      measured("example.failure", async () => {
        throw error;
      })
    ).rejects.toBe(error);

    expect(log).toHaveBeenCalledWith(
      JSON.stringify({
        operation: "example.failure",
        duration_ms: 7.5,
        success: false,
      })
    );
  });
});
