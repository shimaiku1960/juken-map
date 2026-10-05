import { execFileSync } from "node:child_process";
import { createHmac } from "node:crypto";
import { expect, test, type Page } from "@playwright/test";
import { login } from "./utils";

// ログイン（Go の /api/auth/*、JUK-115）の画面の流れ。API の細かい判定（期限・回数制限・取り消しなど）は
// apps/api-go の auth_db_test.go が見る。ここでは、画面とサーバーがつながっていることを確かめる。
// メールは送られないので、メールのリンクに載るトークンは db/e2e-auth.ts で発行する。

const PASSWORD = "e2e passphrase for sign up";
const emails: string[] = [];

function helper(...args: string[]) {
  const out = execFileSync("pnpm", ["exec", "tsx", "--env-file-if-exists=.env", "db/e2e-auth.ts", ...args], {
    encoding: "utf8",
    env: process.env,
  });
  return out.trim().split("\n").pop() ?? "";
}

function newEmail() {
  const email = `e2e-auth-${Date.now()}-${Math.random().toString(36).slice(2, 8)}@example.test`;
  emails.push(email);
  return email;
}

test.afterAll(() => {
  for (const email of emails) helper("cleanup", email);
});

/** RFC 6238（SHA-1・30秒・6桁）。認証アプリの代わり。 */
function totp(secretBase32: string, at = Date.now()) {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const c of secretBase32.replace(/=+$/, "")) bits += alphabet.indexOf(c).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)));
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / 30)));
  const mac = createHmac("sha1", key).update(counter).digest();
  const offset = mac[mac.length - 1] & 0x0f;
  return String((mac.readUInt32BE(offset) & 0x7fffffff) % 1_000_000).padStart(6, "0");
}

async function fillLogin(page: Page, email: string, password: string) {
  await page.goto("/login");
  await page.getByLabel("メールアドレス").fill(email);
  await page.getByLabel("パスワード", { exact: true }).fill(password);
  await page.getByRole("button", { name: "ログイン", exact: true }).click();
}

test("登録して、メールのリンクから確認し、ログインできる", async ({ page }) => {
  const email = newEmail();
  await page.goto("/signup");
  await page.getByLabel("メールアドレス").fill(email);
  await page.getByLabel("パスワード", { exact: true }).fill(PASSWORD);
  await page.getByRole("button", { name: "新規登録" }).click();
  await expect(page).toHaveURL(/\/verify-email$/);

  // 確認の前はログインできない。
  await fillLogin(page, email, PASSWORD);
  await expect(page.getByText("メールアドレスの確認が完了していません")).toBeVisible();

  // リンクを開いただけでは確認されない。開いたら URL からトークンが消える（D3）。
  const token = helper("token", email, "verify-email");
  await page.goto(`/verify-email/confirm?token=${token}`);
  await expect(page.getByRole("button", { name: "メールアドレスを確認する" })).toBeVisible();
  await expect(page).toHaveURL(/\/verify-email\/confirm$/);
  await page.getByRole("button", { name: "メールアドレスを確認する" }).click();
  await expect(page.getByRole("heading", { name: "メールアドレスを確認しました" })).toBeVisible();

  await login(page, email, PASSWORD);
});

test("パスワードを忘れたら、メールのリンクから決め直せる", async ({ page }) => {
  const email = newEmail();
  helper("user", email, PASSWORD);

  await page.goto("/forgot-password");
  await page.getByLabel("メールアドレス").fill(email);
  await page.getByRole("button", { name: "再設定リンクを送信" }).click();
  await expect(page.getByText("再設定用のリンクを送りました")).toBeVisible();

  const token = helper("token", email, "password-reset");
  await page.goto(`/reset-password?token=${token}`);
  await expect(page).toHaveURL(/\/reset-password$/);
  await page.getByLabel("新しいパスワード").fill("a brand new e2e passphrase");
  await page.getByRole("button", { name: "パスワードを再設定する" }).click();
  await expect(page).toHaveURL(/\/login$/);

  await fillLogin(page, email, PASSWORD);
  await expect(page.getByText("メールアドレスまたはパスワードが違います")).toBeVisible();
  await login(page, email, "a brand new e2e passphrase");
});

test("管理者は2段階認証を設定してから管理画面に入り、次からはコードを求められる", async ({ page }) => {
  const email = newEmail();
  helper("user", email, PASSWORD);
  helper("admin", email);
  await login(page, email, PASSWORD);

  await page.goto("/admin");
  await page.getByLabel("パスワード", { exact: true }).fill(PASSWORD);
  await page.getByRole("button", { name: "設定を始める" }).click();
  const secret = (await page.locator("code").first().textContent())!.trim();
  const backupCode = (await page.locator("ul li").first().textContent())!.trim();
  await page.getByLabel("認証アプリの6桁のコード").fill(totp(secret));
  await page.getByRole("button", { name: "確認して有効にする" }).click();
  await expect(page.getByRole("heading", { name: "利用状況" })).toBeVisible();

  // ログインし直すと、パスワードのあとにコードを求められる。予備コードでも入れる。
  await page.context().clearCookies();
  await fillLogin(page, email, PASSWORD);
  await expect(page.getByRole("heading", { name: "2段階認証" })).toBeVisible();
  await page.getByRole("button", { name: "予備コードを使う" }).click();
  await page.getByLabel("予備コード").fill(backupCode);
  await page.getByRole("button", { name: "確認してログイン" }).click();
  await expect(page.getByRole("heading", { name: "今日の学習を始めよう" })).toBeVisible();
  await page.goto("/admin");
  await expect(page.getByRole("heading", { name: "利用状況" })).toBeVisible();
});

test("プロフィールから退会すると、データが消えてログインできなくなる（06 G3）", async ({ page }) => {
  const email = newEmail();
  helper("user", email, PASSWORD);
  await login(page, email, PASSWORD);

  await page.goto("/profile");
  await page.getByRole("button", { name: "退会する" }).click();
  await page.getByLabel("確認のため、パスワードを入力してください").fill("not my passphrase!!");
  await page.getByRole("button", { name: "すべて削除して退会する" }).click();
  await expect(page.getByText("今のパスワードが違います")).toBeVisible();

  await page.getByLabel("確認のため、パスワードを入力してください").fill(PASSWORD);
  await page.getByRole("button", { name: "すべて削除して退会する" }).click();
  await expect(page).toHaveURL(/\/\?deleted=1$/);

  await fillLogin(page, email, PASSWORD);
  await expect(page.getByText("メールアドレスまたはパスワードが違います")).toBeVisible();
});
