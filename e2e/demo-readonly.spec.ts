import { test, expect } from "@playwright/test";
import { loginAsDemo } from "./utils";

// デモアカウントは閲覧専用：操作しようとした時点で理由と回復方法を示す。
test("デモアカウントの編集操作には閲覧専用トーストを表示する", async ({ page }) => {
  await loginAsDemo(page);
  await page.getByRole("button", { name: "学習を始める" }).click();
  await expect(page.getByText("デモアカウントは閲覧専用です")).toBeVisible();
  await expect(
    page.getByText("編集するには、ご自身のアカウントでログインしてください。")
  ).toBeVisible();
  await expect(page.getByText("デモアカウントでは計測できません")).toBeVisible();
});
