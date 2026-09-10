import { type Locator, type Page, expect } from "@playwright/test";

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

// 「デモを見る」ボタンでログインし、学習開始画面の表示まで待つ
export async function loginAsDemo(page: Page) {
  await page.goto("/login");
  await page.getByRole("button", { name: "デモを見る" }).click();
  await expect(
    page.getByRole("heading", { name: "今日の学習を始めよう" })
  ).toBeVisible();
}

// 集計表示（例: 「今日の学習時間：2時間15分」）から分を読む。
export function displayedMinutes(text: string | null) {
  const hours = Number(text?.match(/(\d+)時間/)?.[1] ?? 0);
  const minutes = Number(text?.match(/(\d+)分/)?.[1] ?? 0);
  return hours * 60 + minutes;
}

// 集計の基準値を、実績の取得が終わってから読む。
//
// ダッシュボードは取得中 logs=[] で描くため、待たずに読むと 0 分を掴む。
// SSR だった Next.js 版では起きなかったが、SPA の本番ビルドは初回描画が
// フェッチより速いので露出する（dev サーバーではモジュール読み込みが遅く隠れていた）。
// 「読み込み中」を示す DOM が無いので、値が2回続けて同じになったら確定とみなす。
export async function settledMinutes(locator: Locator) {
  let previous: number | null = null;
  let current = 0;

  await expect
    .poll(
      async () => {
        current = displayedMinutes(await locator.textContent());
        const settled = previous === current;
        previous = current;
        return settled;
      },
      { intervals: [250, 250, 250, 250, 250], timeout: 10_000 }
    )
    .toBe(true);

  return current;
}
