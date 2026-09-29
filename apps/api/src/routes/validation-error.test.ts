import { describe, expect, it } from "vitest";
import { z } from "zod";
import { validationErrorBody } from "./validation-error.ts";

function firstError(schema: z.ZodType, input: unknown) {
  const parsed = schema.safeParse(input);
  if (parsed.success) throw new Error("通ってしまった");
  return validationErrorBody(parsed.error);
}

describe("validationErrorBody", () => {
  it("自分で書いた規則は params の code を使い、項目はドットでつなぐ", () => {
    const schema = z.object({
      items: z.array(
        z.object({ unit: z.string() }).superRefine((_, ctx) => {
          ctx.addIssue({ code: "custom", message: "単位を選んでください", path: ["unit"], params: { code: "invalid_range_unit" } });
        })
      ),
    });

    expect(firstError(schema, { items: [{ unit: "x" }] })).toEqual({
      error: "単位を選んでください",
      code: "invalid_range_unit",
      field: "items.0.unit",
    });
  });

  it("組み込みのチェックは Zod の code をそのまま使う", () => {
    const schema = z.object({ minutes: z.number().positive("1分以上を入力してください") });

    expect(firstError(schema, { minutes: 0 })).toEqual({
      error: "1分以上を入力してください",
      code: "too_small",
      field: "minutes",
    });
  });

  it("params の code が無い規則は custom、項目に結びつかなければ field は null", () => {
    const schema = z.string().refine(() => false, "だめです");

    expect(firstError(schema, "a")).toEqual({ error: "だめです", code: "custom", field: null });
  });

  it("issue が複数あっても、最初の1件だけを返す（画面が出すのは1行）", () => {
    const schema = z.object({ a: z.string(), b: z.string() });

    expect(firstError(schema, {})).toMatchObject({ field: "a" });
  });
});
