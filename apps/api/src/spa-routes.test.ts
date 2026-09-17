import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SPA_ROUTES } from "@/shared/routes";

// 未知のパスを 404 にする判定（server.ts の setNotFoundHandler）は SPA_ROUTES を正とするので、
// App.tsx との二重管理がずれると、足したばかりのページが本番で 404 になる。
// 型では守れないため、ここで App.tsx の <Route path> と機械的に突き合わせる。
// ルートの定義は画面側にあるが、ずれて困るのは API なので、テストは API 側に置く。
const appSource = readFileSync(
  fileURLToPath(new URL("../../web/src/App.tsx", import.meta.url)),
  "utf-8"
);

describe("SPA_ROUTES", () => {
  it("App.tsx の <Route path> と一致する", () => {
    const paths = [...appSource.matchAll(/path="([^"]+)"/g)]
      .map((match) => match[1]!)
      // 受け皿の "*" は NotFoundPage 用なので対象外。
      .filter((path) => path !== "*");

    expect(paths.toSorted()).toEqual([...SPA_ROUTES].toSorted());
  });
});
