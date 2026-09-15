import { describe, expect, it } from "vitest";
import type { tracing } from "@opentelemetry/sdk-node";
import {
  RedactingSpanExporter,
  redactPath,
  redactUrlAttributes,
} from "@/api/observability/redact";

describe("redactPath", () => {
  it("? 以降（トークンや OAuth の code）を落とす", () => {
    expect(redactPath("/api/auth/verify-email?token=secret&callbackURL=%2F")).toBe(
      "/api/auth/verify-email"
    );
    expect(redactPath("/api/line/oauth/callback?state=s&code=c")).toBe(
      "/api/line/oauth/callback"
    );
  });

  it("パスに入るパスワード再設定のトークンを伏せる", () => {
    expect(redactPath("/api/auth/reset-password/secret-token?callbackURL=%2F")).toBe(
      "/api/auth/reset-password/:token"
    );
  });

  it("トークンを含まないパスはそのまま残す", () => {
    expect(redactPath("/api/study-logs/12")).toBe("/api/study-logs/12");
  });
});

describe("redactUrlAttributes", () => {
  it("HTTP と Fastify の計測が付ける URL の属性から、トークンを取り除く", () => {
    const redacted = redactUrlAttributes({
      // Fastify の計測は url.path に ? 以降まで入れる。
      "url.path": "/api/auth/reset-password/secret-token?callbackURL=%2F",
      "url.query": "token=secret",
      "url.full": "http://localhost:4000/api/auth/verify-email?token=secret",
      "http.target": "/api/auth/verify-email?token=secret",
      "http.request.method": "GET",
    });

    expect(redacted).toEqual({
      "url.path": "/api/auth/reset-password/:token",
      "url.full": "http://localhost:4000/api/auth/verify-email",
      "http.target": "/api/auth/verify-email",
      "http.request.method": "GET",
    });
    expect(JSON.stringify(redacted)).not.toContain("secret");
  });
});

describe("RedactingSpanExporter", () => {
  it("属性だけ差し替えた写しを渡し、元のスパンのメソッドはそのまま使える", () => {
    const received: tracing.ReadableSpan[] = [];
    const inner: tracing.SpanExporter = {
      export: (spans, done) => {
        received.push(...spans);
        done({ code: 0 });
      },
      shutdown: async () => {},
    };
    const span = {
      name: "GET",
      attributes: { "url.query": "token=secret", "url.path": "/api/x" },
      spanContext: () => ({ traceId: "t", spanId: "s", traceFlags: 1 }),
    } as unknown as tracing.ReadableSpan;

    new RedactingSpanExporter(inner).export([span], () => {});

    expect(received[0].attributes).toEqual({ "url.path": "/api/x" });
    expect(received[0].name).toBe("GET");
    expect(received[0].spanContext().traceId).toBe("t");
    // 元のスパンは書き換えない。
    expect(span.attributes["url.query"]).toBe("token=secret");
  });
});
