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

  await page.getByRole("link", { name: "記録・予定" }).click();
  const todayMinutes = page.getByText(/今日の学習時間：/);
  const beforeMinutes = displayedMinutes(await todayMinutes.textContent());

  await page.getByRole("link", { name: "学習を始める" }).click();

  await page.getByRole("button", { name: "学習を始める" }).click();
  const chooser = page.getByRole("dialog", { name: "何を勉強しますか？" });
  await chooser.getByRole("button", { name: /自由に入力する/ }).click();
  const picker = page.getByRole("dialog", { name: "自由に入力する" });
  await picker.getByLabel("学習内容").fill("過去問演習");
  await picker.getByRole("button", { name: "英語" }).click();
  await picker.getByRole("button", { name: "計測を開始" }).click();

  await expect(page.getByText("● 計測中")).toBeVisible();
  await page.getByRole("button", { name: "学習を終了" }).click();

  const review = page.getByRole("dialog", { name: "おつかれさまでした" });
  await review.getByLabel("学習時間（分）").fill("45");
  await review.getByRole("button", { name: "実績を保存" }).click();

  await expect(page.getByText("学習実績を保存しました")).toBeVisible();

  await page.getByRole("link", { name: "記録・予定" }).click();

  // その日の実績リストに「英語・45分」の記録が現れる（科目と時間は別要素で描画される）
  const savedLog = page
    .getByRole("listitem")
    .filter({ hasText: "英語" })
    .filter({ hasText: "45分" });
  await expect(savedLog.first()).toBeVisible();

  // 今日の合計も、今回記録した45分ぶん増える
  await expect
    .poll(async () => displayedMinutes(await todayMinutes.textContent()))
    .toBe(beforeMinutes + 45);
});

test("計測中に再読み込みしてもタイマーを復元できる", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);
  await page.getByRole("button", { name: "学習を始める" }).click();
  const chooser = page.getByRole("dialog", { name: "何を勉強しますか？" });
  await chooser.getByRole("button", { name: /自由に入力する/ }).click();
  const picker = page.getByRole("dialog", { name: "自由に入力する" });
  await picker.getByLabel("学習内容").fill("復習");
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

test("予定外の学習方法を選んで戻れる", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);
  await page.getByRole("button", { name: "学習を始める" }).click();

  const chooser = page.getByRole("dialog", { name: "何を勉強しますか？" });
  await expect(
    chooser.getByRole("button", { name: /参考書から選ぶ/ })
  ).toBeVisible();
  await expect(
    chooser.getByRole("button", { name: /自由に入力する/ })
  ).toBeVisible();

  await chooser.getByRole("button", { name: /自由に入力する/ }).click();
  const freeForm = page.getByRole("dialog", { name: "自由に入力する" });
  await freeForm.getByRole("button", { name: "計測を開始" }).click();
  await expect(freeForm.getByRole("alert")).toHaveText(
    "勉強する内容を入力してください"
  );

  await freeForm.getByRole("button", { name: "戻る" }).click();
  await chooser.getByRole("button", { name: /参考書から選ぶ/ }).click();
  const textbookPicker = page.getByRole("dialog", { name: "参考書から選ぶ" });
  await textbookPicker
    .getByRole("button", { name: /E2E英語教材/ })
    .click();
  await textbookPicker.getByRole("button", { name: "計測を開始" }).click();
  await expect(page.getByText("● 計測中")).toBeVisible();

  await page.getByRole("button", { name: "学習を終了" }).click();
  const review = page.getByRole("dialog", { name: "おつかれさまでした" });
  await review.getByRole("button", { name: "保存せず終了" }).click();
  await review.getByRole("button", { name: "破棄する" }).click();
});
