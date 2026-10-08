import { execFile } from "node:child_process";
import { randomBytes } from "node:crypto";
import { fileURLToPath } from "node:url";

// seed で作る利用者のパスワードのハッシュと ID（JUK-115）。
//
// ハッシュはログイン（Go）の本物の関数で作る（scripts/go-devtool.sh hash-password、JUK-143）。
// 前はここで同じ形・同じ強さをなぞっていたが、Go 側を変えたときの直し忘れで seed の利用者が
// ログインできなくなるので、二重に書くのをやめた。初回は Go のビルドで数秒かかる。

const DEVTOOL = fileURLToPath(new URL("../scripts/go-devtool.sh", import.meta.url));

export function hashPassword(password: string) {
  return new Promise<string>((resolve, reject) => {
    // パスワードはコマンドラインに出さず、標準入力で渡す。
    const child = execFile("bash", [DEVTOOL, "hash-password"], { encoding: "utf8" }, (error, stdout, stderr) => {
      if (error) {
        reject(new Error(`パスワードのハッシュを作れません（Go）: ${stderr.trim() || error.message}`));
        return;
      }
      resolve(stdout.trim());
    });
    child.stdin?.end(password);
  });
}

/**
 * 利用者の ID。Go の account.NewUserID と同じ形（16 バイトの乱数の hex、32 文字）。
 * ログインは ID の形を見ないので、ずれても壊れない。合成データで何万人分も作るため、Go を呼ばずにここで作る。
 */
export function newUserId() {
  return randomBytes(16).toString("hex");
}
