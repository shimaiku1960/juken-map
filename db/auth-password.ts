import * as nodeCrypto from "node:crypto";
import { promisify } from "node:util";

// seed で作る利用者のパスワードのハッシュと ID（JUK-115）。ログインを確かめるのは Go
// （apps/api の auth_password.go）なので、同じ形・同じ強さで作る。
//
//   形：PHC 文字列 $argon2id$v=19$m=19456,t=2,p=1$塩$ハッシュ（塩とハッシュは = の無い base64）
//   入力：NFKC で正規化してからハッシュにする
//
// Argon2id は Node 24.7 から node:crypto に入っている（依存を足さずに済む）。

const MEMORY_KIB = 19 * 1024;
const PASSES = 2;
const PARALLELISM = 1;

// ルートの @types/node は 20 で argon2 の型を持たないので、使う形だけをここで書く（実行は Node 24）。
type Argon2 = (
  algorithm: "argon2id",
  parameters: { message: string; nonce: Buffer; parallelism: number; tagLength: number; memory: number; passes: number },
  callback: (error: Error | null, key: Buffer) => void
) => void;
const argon2Async = promisify((nodeCrypto as unknown as { argon2: Argon2 }).argon2);

export async function hashPassword(password: string) {
  const nonce = nodeCrypto.randomBytes(16);
  const key = await argon2Async("argon2id", {
    message: password.normalize("NFKC"),
    nonce,
    parallelism: PARALLELISM,
    tagLength: 32,
    memory: MEMORY_KIB,
    passes: PASSES,
  });
  const b64 = (buf: Buffer) => buf.toString("base64").replace(/=+$/, "");
  return `$argon2id$v=19$m=${MEMORY_KIB},t=${PASSES},p=${PARALLELISM}$${b64(nonce)}$${b64(key)}`;
}

/** 利用者の ID。Go の newUserID と同じ形（16 バイトの乱数の hex、32 文字）。 */
export function newUserId() {
  return nodeCrypto.randomBytes(16).toString("hex");
}
