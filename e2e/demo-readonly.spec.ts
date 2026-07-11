import { test, expect } from "@playwright/test";
import { loginAsDemo, fillMinutes } from "./utils";

// デモアカウントは閲覧専用：記録しようとすると 403 → 専用トーストが出る。
test("デモアカウントでは実績を記録できない", async ({ page }) => {
  await loginAsDemo(page);

  await fillMinutes(page, "30");
  await page.getByRole("button", { name: "記録する" }).click();

  // demoReadOnlyGuard の 403 メッセージがトーストで出る
  await expect(page.getByText("デモアカウントは閲覧専用です")).toBeVisible();

  // 成功トーストは出ない
  await expect(page.getByText("学習実績を記録しました")).toHaveCount(0);
});
