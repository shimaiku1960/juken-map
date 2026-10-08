// ログインの E2E（e2e/auth.spec.ts）の下ごしらえをする tsx スクリプト。
// メールのリンクに載るトークンは、ログイン（Go）と同じ作り方にするため scripts/go-devtool.sh email-token が発行する（JUK-143）。
//
//   tsx db/e2e-auth.ts user <メール> <パスワード>   確認済みの利用者を作る
//   tsx db/e2e-auth.ts admin <メール>               管理者にする
//   tsx db/e2e-auth.ts cleanup <メール>             利用者を消す
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
    case "cleanup":
      await execute("DELETE FROM `user` WHERE email = ?", [email]);
      return;
    default:
      throw new Error(`知らない操作: ${command}`);
  }
});
