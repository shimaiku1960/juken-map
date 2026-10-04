// ログインの E2E（e2e/auth.spec.ts）の下ごしらえをする tsx スクリプト。メールは送られないので、
// メールのリンクに載るはずのトークンをここで発行して表示する（Go の issueToken と同じ形）。
//
//   tsx db/e2e-auth.ts user <メール> <パスワード>   確認済みの利用者を作る
//   tsx db/e2e-auth.ts admin <メール>               管理者にする
//   tsx db/e2e-auth.ts token <メール> <用途>        verify-email か password-reset のトークンを表示する
//   tsx db/e2e-auth.ts cleanup <メール>             利用者を消す
import { createHash, randomBytes } from "node:crypto";
import { execute, runSeed, select, setCredentialPassword, upsertVerifiedUser } from "./seed-helpers";

const [command, email, arg] = process.argv.slice(2);

async function userId() {
  const [user] = await select<{ id: string }>("SELECT id FROM `user` WHERE email = ?", [email]);
  if (!user) throw new Error(`${email} がいません`);
  return user.id;
}

runSeed(async () => {
  switch (command) {
    case "user": {
      const id = await upsertVerifiedUser({ email, name: email, nickname: "E2E" });
      await setCredentialPassword(id, arg);
      return;
    }
    case "admin":
      await execute("UPDATE `user` SET role = 'admin' WHERE id = ?", [await userId()]);
      return;
    case "token": {
      const id = await userId();
      const token = randomBytes(32).toString("base64url");
      const now = new Date();
      await execute("DELETE FROM AuthToken WHERE userId = ? AND purpose = ?", [id, arg]);
      await execute(
        "INSERT INTO AuthToken (tokenHash, purpose, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?, ?)",
        [createHash("sha256").update(token).digest(), arg, id, now, new Date(now.getTime() + 3_600_000)]
      );
      console.log(token);
      return;
    }
    case "cleanup":
      await execute("DELETE FROM `user` WHERE email = ?", [email]);
      return;
    default:
      throw new Error(`知らない操作: ${command}`);
  }
});
