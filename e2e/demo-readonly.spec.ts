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

test("スマホの上部ユーザーメニューからプロフィールを開ける", async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await loginAsDemo(page);

  const mobileNav = page.getByRole("navigation", {
    name: "モバイルナビゲーション",
  });
  const navBox = await mobileNav.boundingBox();
  expect(navBox).not.toBeNull();
  expect(navBox!.y).toBeGreaterThan(700);
  expect(navBox!.y + navBox!.height).toBeGreaterThanOrEqual(840);
  await expect(mobileNav.getByRole("link")).toHaveCount(3);
  await expect(mobileNav.getByRole("button", { name: "メニュー" })).toHaveCount(0);

  await page.getByRole("button", { name: "ユーザーメニュー" }).click();
  await page
    .getByRole("link", { name: "プロフィール", exact: true })
    .click();

  await expect(page).toHaveURL(/\/profile$/);
  await expect(
    page.getByRole("heading", { name: "プロフィール" })
  ).toBeVisible();
});
