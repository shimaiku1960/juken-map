import { test, expect } from "@playwright/test";
import { login } from "./utils";
import { E2E_EMAIL, E2E_PASSWORD } from "./credentials";

const displayedMinutes = (text: string | null) => {
  const hours = Number(text?.match(/(\d+)時間/)?.[1] ?? 0);
  const minutes = Number(text?.match(/(\d+)分/)?.[1] ?? 0);
  return hours * 60 + minutes;
};

// 毎日ループの正常系：ログイン → 実績を記録 → その場で可視化が更新される。
test("実績を記録するとダッシュボードに反映される", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);
  const todayMinutes = page.getByText(/今日の学習時間：/);
  const beforeMinutes = displayedMinutes(await todayMinutes.textContent());

  // ダッシュボード上部のPrimary CTAから、英語・45分を最小入力で記録
  await page
    .getByRole("button", { name: "学習を記録", exact: true })
    .first()
    .click();
  const dialog = page.getByRole("dialog", { name: "学習を記録" });
  await expect(dialog).toBeVisible();

  const minutesInput = dialog.getByPlaceholder("分", { exact: true });
  await expect(async () => {
    await minutesInput.fill("45");
    await expect(minutesInput).toHaveValue("45", { timeout: 1000 });
  }).toPass({ timeout: 15_000 });

  const subjectSelect = dialog
    .locator("select")
    .filter({ hasText: "科目なし" })
    .first();
  await subjectSelect.selectOption({ label: "英語" });

  await dialog.getByRole("button", { name: "記録する" }).click();

  // 成功トースト
  await expect(page.getByText("学習実績を記録しました")).toBeVisible();

  // 「最近の記録」に 英語・45分 が現れる
  await expect(page.getByText("英語・45分").first()).toBeVisible();

  // 今日の合計も、今回記録した45分ぶん増える
  await expect
    .poll(async () => displayedMinutes(await todayMinutes.textContent()))
    .toBe(beforeMinutes + 45);
});
