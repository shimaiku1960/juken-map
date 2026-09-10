import { test, expect } from "@playwright/test";
import { login } from "./utils";
import { E2E_EMAIL, E2E_PASSWORD } from "./credentials";

// LINE 連携済みなのにプロフィールが「未連携」を出す不具合の再発防止。
//
// NotificationPreferenceForm は initial* をマウント時に useState へ取り込むため、
// 連携状態の取得が終わる前に描くと false が焼き付いて後から直らない。
// ローカルは応答が速く素通りするので、本番の遅延を模して意図的に遅らせる。
test("連携状態の取得が遅くてもプロフィールは連携済みとして描く", async ({ page }) => {
  await page.route("**/api/line/connection", async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 800));
    await route.continue();
  });

  await login(page, E2E_EMAIL, E2E_PASSWORD);
  await page.goto("/profile");

  // 連携済みユーザーなので、連携を促す文言は出ない。
  await expect(page.getByRole("heading", { name: "プロフィール" })).toBeVisible();
  await expect(page.getByText("LINEと連携する")).toHaveCount(0);
});
