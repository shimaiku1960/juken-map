import { type Page, expect } from "@playwright/test";

// メール＋パスワードでログインし、ダッシュボード表示まで待つ
export async function login(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByPlaceholder("メールアドレス").fill(email);
  await page.getByPlaceholder("パスワード").fill(password);
  await page.getByRole("button", { name: "ログイン", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "ダッシュボード" })
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

// 「デモでログイン」ボタンでログインし、ダッシュボード表示まで待つ
export async function loginAsDemo(page: Page) {
  await page.goto("/login");
  await page.getByRole("button", { name: /デモでログイン/ }).click();
  await expect(
    page.getByRole("heading", { name: "ダッシュボード" })
  ).toBeVisible();
}
