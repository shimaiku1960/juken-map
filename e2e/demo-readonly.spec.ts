import { test, expect } from "@playwright/test";
import { loginAsDemo } from "./utils";

// デモアカウントは閲覧専用：タイマー開始前に理由を示して無効化する。
test("デモアカウントでは学習タイマーを開始できない", async ({ page }) => {
  await loginAsDemo(page);
  await expect(page.getByRole("button", { name: "学習を始める" })).toBeDisabled();
  await expect(page.getByText("デモアカウントでは計測できません")).toBeVisible();
});
