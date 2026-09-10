// macOS で依存を足すと、Linux 用のオプショナル依存が package-lock.json から
// 抜けることがある（npm の既知の挙動）。CI と本番イメージは Linux なので、
// 抜けたまま push すると npm ci が落ちる。
//
// lockfile に darwin 用のネイティブパッケージがあるのに、対応する linux-x64-gnu が
// 無い場合を検出する。パッケージ名を決め打ちしないので、依存が入れ替わっても腐らない。
import { readFileSync } from "node:fs";

const lockfiles = process.argv.slice(2);
if (lockfiles.length === 0) {
  console.error("使い方: node scripts/check-linux-native-deps.mjs <package-lock.json...>");
  process.exit(2);
}

let failed = false;

for (const lockfile of lockfiles) {
  const packages = Object.keys(JSON.parse(readFileSync(lockfile, "utf-8")).packages ?? {});
  const names = new Set(packages);

  // linux 側の名前はパッケージによって違う（-linux-x64-gnu / -linux-x64 / -linuxmusl-x64 …）。
  // 決め打ちすると誤検知するので、「同じ接頭辞の linux 版が1つでもあるか」で判定する。
  const missing = packages
    .filter((name) => /-darwin-(arm64|x64)$/.test(name))
    .map((name) => name.replace(/-darwin-(arm64|x64)$/, "-linux"))
    .filter((prefix) => ![...names].some((name) => name.startsWith(prefix)));

  if (missing.length > 0) {
    failed = true;
    for (const name of new Set(missing)) {
      console.error(
        `::error::${lockfile} に ${name.replace("node_modules/", "")}* に相当する Linux 用ネイティブ依存がありません。` +
          `ローカルで 'npm run lock:fix' を実行し、更新された lockfile をコミットしてください。`
      );
    }
  } else {
    console.log(`${lockfile}: Linux 用ネイティブ依存は揃っています（darwin 用 ${packages.filter((n) => /-darwin-/.test(n)).length} 件を確認）`);
  }
}

process.exit(failed ? 1 : 0);
