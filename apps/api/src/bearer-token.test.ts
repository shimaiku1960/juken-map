import { describe, it, expect } from "vitest";
import type { FastifyRequest } from "fastify";
import { hasBearerToken } from "./bearer-token.ts";

const withHeader = (authorization?: string) =>
  ({ headers: authorization === undefined ? {} : { authorization } }) as FastifyRequest;

describe("hasBearerToken", () => {
  it("Bearer の値が一致すれば true", () => {
    expect(hasBearerToken(withHeader("Bearer s3cret"), "s3cret")).toBe(true);
  });

  it("値が違えば false（長さが同じでも、違っても）", () => {
    expect(hasBearerToken(withHeader("Bearer s3creT"), "s3cret")).toBe(false);
    expect(hasBearerToken(withHeader("Bearer s3cret-and-more"), "s3cret")).toBe(false);
    expect(hasBearerToken(withHeader("s3cret"), "s3cret")).toBe(false);
  });

  it("Authorization が無ければ false", () => {
    expect(hasBearerToken(withHeader(), "s3cret")).toBe(false);
  });

  // 未設定のまま `Bearer ${secret}` を組むと "Bearer undefined" や "Bearer " になり、
  // それを送れば通ってしまう。未設定なら何を送っても断る。
  it.each([undefined, ""])("秘密値が未設定（%j）なら、何を送っても false", (secret) => {
    for (const header of [undefined, "Bearer ", "Bearer undefined", "Bearer"]) {
      expect(hasBearerToken(withHeader(header), secret)).toBe(false);
    }
  });
});
