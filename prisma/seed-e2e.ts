// E2E 用の通常ユーザーを投入する tsx スクリプト（Playwright の globalSetup から呼ぶ）。
// アプリの接続プール（ESM）を読み込むので、Playwright 本体ではなく tsx の子プロセスで実行する。
import { E2E_EMAIL, E2E_PASSWORD } from "../e2e/credentials";
import {
  execute,
  runSeed,
  select,
  setCredentialPassword,
  upsertVerifiedUser,
} from "./seed-helpers";

runSeed(async () => {
  // メール確認必須化後も E2E ログインが通るよう、既存行にも確認済みを保証する
  const userId = await upsertVerifiedUser({
    email: E2E_EMAIL,
    name: "E2Eユーザー",
    nickname: "E2E",
  });
  await setCredentialPassword(userId, E2E_PASSWORD);

  // Textbook の UNIQUE は (userId, name) だけなので、ON DUPLICATE KEY で安全に upsert できる
  const now = new Date();
  await execute(
    `INSERT INTO Textbook (userId, name, subject, rangeUnit, createdAt, updatedAt)
     VALUES (?, 'E2E英語教材', 'english', 'page', ?, ?) AS new
     ON DUPLICATE KEY UPDATE subject = new.subject, rangeUnit = new.rangeUnit, updatedAt = new.updatedAt`,
    [userId, now, now]
  );

  // LINE 連携済みの状態も作る。プロフィール画面が連携状態を正しく描けるかを
  // E2E で確認するため（連携状態の取得が遅いと未連携が焼き付く不具合があった）。
  // LineConnection は userId と lineUserId の2つが UNIQUE なので ON DUPLICATE KEY は使わない。
  const [connection] = await select<{ id: number }>(
    "SELECT id FROM LineConnection WHERE userId = ?",
    [userId]
  );
  if (!connection) {
    await execute(
      `INSERT INTO LineConnection (userId, lineUserId, linkedAt, createdAt, updatedAt)
       VALUES (?, 'U-e2e-test', ?, ?, ?)`,
      [userId, now, now, now]
    );
  }

  console.log(`E2Eユーザーを投入: ${E2E_EMAIL}`);
});
