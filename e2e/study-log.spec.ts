import { test, expect } from "@playwright/test";
import { login } from "./utils";
import { E2E_EMAIL, E2E_PASSWORD } from "./credentials";

const displayedMinutes = (text: string | null) => {
  const hours = Number(text?.match(/(\d+)時間/)?.[1] ?? 0);
  const minutes = Number(text?.match(/(\d+)分/)?.[1] ?? 0);
  return hours * 60 + minutes;
};

// 毎日ループの正常系：開始 → 計測 → 終了 → 実績保存 → 可視化更新。
test("タイマーで学習した実績がダッシュボードに反映される", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);
  const todayMinutes = page.getByText(/今日の学習時間：/);
  const beforeMinutes = displayedMinutes(await todayMinutes.textContent());

  await page.getByRole("button", { name: "学習を始める" }).click();
  const picker = page.getByRole("dialog", { name: "何を勉強しますか？" });
  await picker.getByLabel("予定外の学習").check();
  const subjectSelect = picker
    .locator("select")
    .filter({ hasText: "科目なし" })
    .first();
  await subjectSelect.selectOption({ label: "英語" });
  await picker.getByRole("button", { name: "計測を開始" }).click();

  await expect(page.getByText("● 計測中")).toBeVisible();
  await page.getByRole("button", { name: "学習を終了" }).click();

  const review = page.getByRole("dialog", { name: "おつかれさまでした" });
  await review.getByLabel("学習時間（分）").fill("45");
  await review.getByRole("button", { name: "実績を保存" }).click();

  await expect(page.getByText("学習実績を保存しました")).toBeVisible();

  // 「最近の記録」に 英語・45分 が現れる
  await expect(page.getByText("英語・45分").first()).toBeVisible();

  // 今日の合計も、今回記録した45分ぶん増える
  await expect
    .poll(async () => displayedMinutes(await todayMinutes.textContent()))
    .toBe(beforeMinutes + 45);
});

test("計測中に再読み込みしてもタイマーを復元できる", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);
  await page.getByRole("button", { name: "学習を始める" }).click();
  const picker = page.getByRole("dialog", { name: "何を勉強しますか？" });
  await picker.getByLabel("予定外の学習").check();
  await picker.getByRole("button", { name: "計測を開始" }).click();
  await expect(page.getByText("● 計測中")).toBeVisible();

  await page.reload();
  await expect(page.getByText("● 計測中")).toBeVisible();
  await page.getByRole("button", { name: "学習を終了" }).click();
  const review = page.getByRole("dialog", { name: "おつかれさまでした" });
  await review.getByRole("button", { name: "保存せず終了" }).click();
  await review.getByRole("button", { name: "破棄する" }).click();
  await expect(page.getByRole("button", { name: "学習を始める" })).toBeVisible();
});
