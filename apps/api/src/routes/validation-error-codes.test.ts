import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

// 400 の code（JUK-76）は、自分で書いた規則（refine・addIssue）では params で付けた名前になる。
// 付け忘れると code が "custom" になり、画面や Go 側からどの規則か区別できなくなる。
// そこで、src/shared/validations/ の規則にはすべて名前が付いていることを、ソースの文字で確かめる。
// （src/shared は画面の型チェックにも含まれ、node:fs を使えないので、テストはこちらに置く）
const dir = join(import.meta.dirname, "../../../../src/shared/validations");
const sources = readdirSync(dir)
  .filter((name) => name.endsWith(".ts") && !name.endsWith(".test.ts"))
  .map((name) => ({ name, text: readFileSync(join(dir, name), "utf8") }));

const count = (text: string, pattern: RegExp) => text.match(pattern)?.length ?? 0;

describe("入力チェックの規則の code", () => {
  it.each(sources)("$name：refine・addIssue にはすべて params の code が付いている", ({ text }) => {
    const rules = count(text, /\.refine\(|\.addIssue\(/g);
    const named = count(text, /params: \{ code: "/g);

    expect(named).toBe(rules);
  });

  it("code の名前は snake_case", () => {
    const names = sources.flatMap(({ text }) => [...text.matchAll(/params: \{ code: "([^"]+)" \}/g)].map((m) => m[1]));

    expect(names.length).toBeGreaterThan(0);
    for (const name of names) expect(name).toMatch(/^[a-z]+(_[a-z]+)*$/);
  });
});
