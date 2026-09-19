import { expect, it } from "vitest";

it("Zod より先に読み込めば、Zod が jitless を使う", async () => {
  await import("@/web/lib/zod-jitless");
  const { z } = await import("zod");

  expect(z.config().jitless).toBe(true);
});
