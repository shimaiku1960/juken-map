import { describe, expect, it } from "vitest";
import { readCollectorUrl, redactSecrets } from "@/web/lib/faro";

describe("redactSecrets", () => {
  it("URL のトークンを伏せ、他のクエリは残す", () => {
    expect(
      redactSecrets("https://juken-map.com/reset-password?token=abc.def-123&from=mail")
    ).toBe("https://juken-map.com/reset-password?token=[REDACTED]&from=mail");
    expect(redactSecrets("/line/link?linkToken=xyz")).toBe(
      "/line/link?linkToken=[REDACTED]"
    );
    expect(redactSecrets("/profile?line=linked")).toBe("/profile?line=linked");
  });

  it("パスに載った Better Auth の再設定トークンも伏せる", () => {
    expect(
      redactSecrets("https://juken-map.com/api/auth/reset-password/tok123?callbackURL=%2Freset-password")
    ).toBe(
      "https://juken-map.com/api/auth/reset-password/[REDACTED]?callbackURL=%2Freset-password"
    );
  });

  it("送信データの入れ子（meta・スタックトレース・配列）まで伏せ、元は書き換えない", () => {
    const item = {
      type: "exception",
      payload: {
        value: "Failed: /api/auth/verify-email?token=jwt.value",
        stacktrace: { frames: [{ filename: "https://juken-map.com/assets/a.js", lineno: 1 }] },
      },
      meta: { page: { url: "https://juken-map.com/reset-password?token=secret" } },
    };

    const redacted = redactSecrets(item);

    expect(redacted.payload.value).toBe(
      "Failed: /api/auth/verify-email?token=[REDACTED]"
    );
    expect(redacted.meta.page.url).toBe(
      "https://juken-map.com/reset-password?token=[REDACTED]"
    );
    expect(redacted.payload.stacktrace.frames[0]).toEqual({
      filename: "https://juken-map.com/assets/a.js",
      lineno: 1,
    });
    expect(item.meta.page.url).toContain("token=secret");
  });
});

describe("readCollectorUrl", () => {
  const docWith = (content: string | null) =>
    ({
      querySelector: () =>
        content === null ? null : { getAttribute: () => content },
    }) as unknown as Document;

  it("meta があればその URL を返す", () => {
    expect(readCollectorUrl(docWith("https://faro.example/collect/k"))).toBe(
      "https://faro.example/collect/k"
    );
  });

  it("meta が無い・空なら送らない（null）", () => {
    expect(readCollectorUrl(docWith(null))).toBeNull();
    expect(readCollectorUrl(docWith(""))).toBeNull();
  });
});
