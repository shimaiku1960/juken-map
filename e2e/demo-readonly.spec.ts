import { test, expect } from "@playwright/test";
import { loginAsDemo } from "./utils";

// デモアカウントは閲覧専用：記録しようとすると 403 → 専用トーストが出る。
test("デモアカウントでは実績を記録できない", async ({ page }) => {
  await loginAsDemo(page);

  await page
    .getByRole("button", { name: "学習を記録", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: "学習を記録" });
  await dialog.getByRole("button", { name: "30分" }).click();
  await dialog.getByRole("button", { name: "記録する" }).click();

  // demoReadOnlyGuard の 403 メッセージがトーストで出る
  await expect(page.getByText("デモアカウントは閲覧専用です")).toBeVisible();

  // 成功トーストは出ない
  await expect(page.getByText("学習実績を記録しました")).toHaveCount(0);
});
