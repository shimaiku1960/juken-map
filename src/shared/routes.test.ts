import { describe, expect, it } from "vitest";
import { isKnownSpaRoute } from "./routes";

describe("isKnownSpaRoute", () => {
  it("App.tsx が持つパスは描ける", () => {
    expect(isKnownSpaRoute("/")).toBe(true);
    expect(isKnownSpaRoute("/dashboard")).toBe(true);
    expect(isKnownSpaRoute("/line/link")).toBe(true);
  });

  it("可変部分は1区切りだけに一致する", () => {
    expect(isKnownSpaRoute("/articles/abc123")).toBe(true);
    expect(isKnownSpaRoute("/explore/1")).toBe(true);
    // さらに下の階層は App.tsx に無い。
    expect(isKnownSpaRoute("/articles/abc123/edit")).toBe(false);
    expect(isKnownSpaRoute("/explore/1/2")).toBe(false);
  });

  it("末尾の / は同じ画面として扱う", () => {
    expect(isKnownSpaRoute("/dashboard/")).toBe(true);
    expect(isKnownSpaRoute("/")).toBe(true);
  });

  it("本番ログに来ていたボットのスキャンは描けない", () => {
    for (const path of [
      "/wp-admin/install.php",
      "/wp-login.php",
      "/alive.php",
      "/ab2g",
      "/teorema505",
      "/AGENTS.md",
      "/fapi/v1/depth",
      "/ws/btcusdt@bestBidAsk",
    ]) {
      expect(isKnownSpaRoute(path)).toBe(false);
    }
  });

  it("似ているだけのパスは描けない", () => {
    expect(isKnownSpaRoute("/dashboards")).toBe(false);
    expect(isKnownSpaRoute("/dashboard/extra")).toBe(false);
    expect(isKnownSpaRoute("/articles")).toBe(false);
  });
});

