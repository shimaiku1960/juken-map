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

// LINE のトークに届くリンクの linkToken を、GA4・Faro へ送るページの URL やログインの戻り先に
// 載せない（JUK-124）。URL からはすぐ消し、ログインをはさんでも同じタブの中で持ち回る。
test("連携のリンクの linkToken は URL から消え、ログインをはさんでも使える", async ({ page }) => {
  let sentLinkToken: string | undefined;
  await page.route("**/api/line/account-link", async (route) => {
    sentLinkToken = (route.request().postDataJSON() as { linkToken?: string }).linkToken;
    await route.fulfill({ json: { redirectUrl: "/profile" } });
  });

  const response = await page.goto("/line/link?linkToken=e2e-link-token");
  expect(response?.headers()["referrer-policy"]).toBe("no-referrer");
  await expect(page).toHaveURL(/\/line\/link$/);

  const loginLink = page.getByRole("link", { name: "ログインして連携を続ける" });
  await expect(loginLink).toHaveAttribute("href", "/login?callbackURL=%2Fline%2Flink");
  await loginLink.click();

  await page.getByLabel("メールアドレス").fill(E2E_EMAIL);
  await page.getByLabel("パスワード", { exact: true }).fill(E2E_PASSWORD);
  await page.getByRole("button", { name: "ログイン", exact: true }).click();

  await expect(page).toHaveURL(/\/line\/link$/);
  await page.getByRole("button", { name: "このアカウントと連携する" }).click();
  await expect(page).toHaveURL(/\/profile$/);
  expect(sentLinkToken).toBe("e2e-link-token");
});
