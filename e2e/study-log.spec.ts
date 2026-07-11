import { test, expect } from "@playwright/test";
import { login, fillMinutes } from "./utils";
import { E2E_EMAIL, E2E_PASSWORD } from "./credentials";

// 毎日ループの正常系：ログイン → 実績を記録 → その場で可視化が更新される。
test("実績を記録するとダッシュボードに反映される", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);

  // 「実績を記録」フォームで 英語・45分 を記録
  await fillMinutes(page, "45");

  // 科目セレクトで「英語」を選ぶ（フォーム内の最初の科目セレクト）
  const subjectSelect = page
    .locator("select")
    .filter({ hasText: "科目なし" })
    .first();
  await subjectSelect.selectOption({ label: "英語" });

  await page.getByRole("button", { name: "記録する" }).click();

  // 成功トースト
  await expect(page.getByText("学習実績を記録しました")).toBeVisible();

  // 「最近の記録」に 英語・45分 が現れる
  await expect(page.getByText("英語・45分").first()).toBeVisible();

  // 今日の学習時間が 0分 ではなくなっている（45分以上）
  await expect(page.getByText(/今日の学習時間/)).toBeVisible();
  await expect(page.getByText("今日の学習時間：0分")).toHaveCount(0);
});
