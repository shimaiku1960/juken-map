import { expect, test, type Page } from "@playwright/test";

// 本番で、新しい版が動いていて、ログイン後の主な画面が開けるかを確かめる（JUK-101）。
//
// 書き込みはしない。デモは閲覧専用で、書き込むと 403 になり、403 の急増のアラート（JUK-98）に数えられる。
// 本番の数字を汚さないよう、アクセス解析（GA）と画面のエラーの送信（Faro）には送らない。

const expectedCommit = process.env.EXPECTED_COMMIT;

test("Node と Go の /api/health が、今回デプロイしたコミットを返す", async ({ request }) => {
  // /api/health は Node、/api/health/go は nginx が Go の /api/health へ渡す。
  for (const path of ["/api/health", "/api/health/go"]) {
    const response = await request.get(path);
    expect(response.status(), path).toBe(200);
    const body = (await response.json()) as { ok: boolean; commit: string | null };
    expect(body.ok, path).toBe(true);
    if (expectedCommit) expect(body.commit, path).toBe(expectedCommit);
  }
});

const pages = [
  { path: "/dashboard", heading: "記録・予定" },
  { path: "/goals", heading: "志望校" },
  { path: "/explore", heading: "大学を探す" },
  { path: "/profile", heading: "プロフィール" },
];

/** GA と Faro への送信を止める。本番の利用者の数字に E2E の訪問を混ぜないため。 */
async function blockAnalytics(page: Page) {
  await page.route(/googletagmanager\.com|google-analytics\.com|grafana\.net/, (route) => route.abort());
}

test("デモでログインし、主な画面が API のエラーも画面の例外も無く開ける", async ({ page }) => {
  await blockAnalytics(page);

  const apiErrors: string[] = [];
  page.on("response", (response) => {
    const url = new URL(response.url());
    if (url.pathname.startsWith("/api/") && response.status() >= 400) {
      apiErrors.push(`${response.status()} ${response.request().method()} ${url.pathname}`);
    }
  });
  const pageErrors: string[] = [];
  page.on("pageerror", (error) => pageErrors.push(error.message));

  await page.goto("/login");
  await page.getByRole("button", { name: "デモを見る" }).click();
  // ログイン後のトップへの移動は、最初の1回だけ10秒を超えることがあった（手元から本番、2026-10-02）。
  // CI は毎回まっさらな状態から始まるので、ここだけ長めに待つ。
  await expect(page.getByRole("heading", { name: "今日の学習を始めよう" })).toBeVisible({ timeout: 20_000 });

  for (const { path, heading } of pages) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1, name: heading }), path).toBeVisible();
    // 画面が出たあとに続けて読む API も待ってから、次の画面へ移る。
    await page.waitForLoadState("networkidle");
  }

  expect(apiErrors).toEqual([]);
  expect(pageErrors).toEqual([]);
});
