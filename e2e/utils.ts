import { type Page, expect } from "@playwright/test";

// メール＋パスワードでログインし、学習開始画面の表示まで待つ
export async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("メールアドレス").fill(email);
  await page.getByLabel("パスワード", { exact: true }).fill(password);
  await page.getByRole("button", { name: "ログイン", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "今日の学習を始めよう" })
  ).toBeVisible();
}

// 学習時間（分）を入力する。クライアント component の hydration 前に fill すると
// React 側の値がリセットされることがあるため、値が定着するまで fill を再試行する。
export async function fillMinutes(page: Page, value: string) {
  const input = page.getByPlaceholder("分", { exact: true });
  await expect(async () => {
    await input.fill(value);
    await expect(input).toHaveValue(value, { timeout: 1000 });
  }).toPass({ timeout: 15_000 });
}

// 「デモで試す」ボタンでログインし、学習開始画面の表示まで待つ
export async function loginAsDemo(page: Page) {
  await page.goto("/login");
  await page.getByRole("button", { name: "デモで試す" }).click();
  await expect(
    page.getByRole("heading", { name: "今日の学習を始めよう" })
  ).toBeVisible();
}
