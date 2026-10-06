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

  it("別の URL のクエリにエンコードされて入ったトークンも伏せる（GA4 への送信の URL など）", () => {
    expect(
      redactSecrets(
        "https://www.google-analytics.com/g/collect?v=2&dl=https%3A%2F%2Fjuken-map.com%2Fline%2Flink%3FlinkToken%3Dabc123%26x%3D1&dt=t"
      )
    ).toBe(
      "https://www.google-analytics.com/g/collect?v=2&dl=https%3A%2F%2Fjuken-map.com%2Fline%2Flink%3FlinkToken%3D[REDACTED]%26x%3D1&dt=t"
    );
    expect(redactSecrets("/login?callbackURL=%2Freset-password%3Ftoken%3Dabc.def")).toBe(
      "/login?callbackURL=%2Freset-password%3Ftoken%3D[REDACTED]"
    );
  });

  it("パスに載った Better Auth の再設定トークンも伏せる", () => {
    expect(
      redactSecrets("https://juken-map.com/api/auth/reset-password/tok123?callbackURL=%2Freset-password")
    ).toBe(
      "https://juken-map.com/api/auth/reset-password/[REDACTED]?callbackURL=%2Freset-password"
    );
  });

  it("メールアドレスを伏せる（エンコードされたもの・エラーの文に入ったものも）", () => {
    expect(redactSecrets("Failed: taro.yamada+1@example.co.jp は登録済みです")).toBe(
      "Failed: [REDACTED] は登録済みです"
    );
    expect(redactSecrets("/login?email=taro%40example.com&from=mail")).toBe(
      "/login?email=[REDACTED]&from=mail"
    );
    expect(
      redactSecrets("https://juken-map.com/node_modules/@grafana/faro-web-sdk/index.js")
    ).toBe("https://juken-map.com/node_modules/@grafana/faro-web-sdk/index.js");
  });

  it("送信データの入れ子（meta・スタックトレース・配列）まで伏せ、元は書き換えない", () => {
    const item = {
      type: "exception",
      payload: {
        value: "Failed: /api/auth/verify-email?token=jwt.value",
        stacktrace: { frames: [{ filename: "https://juken-map.com/assets/a.js", lineno: 1 }] },
      },
      meta: {
        page: { url: "https://juken-map.com/reset-password?token=secret" },
        user: { attributes: { contact: "hanako@example.com" } },
      },
    };

    const redacted = redactSecrets(item);

    expect(redacted.payload.value).toBe(
      "Failed: /api/auth/verify-email?token=[REDACTED]"
    );
    expect(redacted.meta.page.url).toBe(
      "https://juken-map.com/reset-password?token=[REDACTED]"
    );
    expect(redacted.meta.user.attributes.contact).toBe("[REDACTED]");
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
