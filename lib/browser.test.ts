import { describe, expect, it } from "vitest";
import { isLineInAppBrowser, safeCallbackURL } from "./browser";

describe("isLineInAppBrowser", () => {
  it("LINEアプリ内ブラウザを判定する", () => {
    expect(
      isLineInAppBrowser(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Line/15.0.0"
      )
    ).toBe(true);
    expect(isLineInAppBrowser("Mozilla/5.0 LIFF/2.27.0")).toBe(true);
  });

  it("通常のブラウザをLINEと誤判定しない", () => {
    expect(
      isLineInAppBrowser(
        "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1"
      )
    ).toBe(false);
    expect(
      isLineInAppBrowser(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/140.0.0.0 Safari/537.36"
      )
    ).toBe(false);
  });
});

describe("safeCallbackURL", () => {
  it("アプリ内の戻り先とクエリを保持する", () => {
    expect(
      safeCallbackURL(
        "?callbackURL=%2Fline%2Flink%3FlinkToken%3Dtoken",
        "/"
      )
    ).toBe("/line/link?linkToken=token");
  });

  it("外部URLとプロトコル相対URLを拒否する", () => {
    expect(safeCallbackURL("?callbackURL=https%3A%2F%2Fevil.example", "/")).toBe("/");
    expect(safeCallbackURL("?callbackURL=%2F%2Fevil.example", "/dashboard")).toBe("/dashboard");
  });
});
