import { test, expect } from "@playwright/test";
import { login, displayedMinutes, settledMinutes } from "./utils";
import { E2E_EMAIL, E2E_PASSWORD } from "./credentials";

// 毎日ループの正常系：開始 → 計測 → 終了 → 実績保存 → 可視化更新。
test("タイマーで学習した実績がダッシュボードに反映される", async ({ page }) => {
  await login(page, E2E_EMAIL, E2E_PASSWORD);

  await page.getByRole("link", { name: "記録・予定" }).click();
  const todayMinutes = page.getByText(/今日の学習時間：/);
  const beforeMinutes = await settledMinutes(todayMinutes);

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

// 実績の一覧は全期間ではなく期間を指定して取る（応答が大きくなりすぎないように）。
// カレンダーは表示中の月しか持たないので、隣の月は裏で先読みしておく。
test("カレンダーは表示中の月だけを取り、前の月は先読みしておく", async ({ page }) => {
  // 実績を取りにいった期間を全部控える
  const queries: string[] = [];
  page.on("request", (req) => {
    const url = new URL(req.url());
    if (url.pathname.startsWith("/api/study-logs")) queries.push(url.search);
  });

  await login(page, E2E_EMAIL, E2E_PASSWORD);
  await page.getByRole("link", { name: "記録・予定" }).click();
  await settledMinutes(page.getByText(/今日の学習時間：/));

  const today = new Date();
  const previous = new Date(today.getFullYear(), today.getMonth() - 1, 1);
  const prevFrom = `from=${previous.getFullYear()}-${String(previous.getMonth() + 1).padStart(2, "0")}-01`;
  const prevLabel = `${previous.getFullYear()}年${previous.getMonth() + 1}月`;

  // 表示する前に、前の月を裏で取り終えている
  await expect
    .poll(() => queries.filter((q) => q.includes(prevFrom)).length)
    .toBe(1);

  await page.getByRole("button", { name: "前の月" }).click();
  await expect(page.getByText(prevLabel)).toBeVisible();

  // 先読み済みなので、その月をもう一度取りにはいかない（さらに前の月の先読みは起きる）
  expect(queries.filter((q) => q.includes(prevFrom))).toHaveLength(1);

  // どの取得も期間を指定している＝全期間を返していた頃には戻っていない
  expect(queries.every((q) => q.includes("from="))).toBe(true);

  // 「今日」で当月へ戻れる
  await page.getByRole("button", { name: "今日", exact: true }).click();
  await expect(page.getByRole("heading", { name: "今日の予定と実績" })).toBeVisible();
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
