// デモユーザー「だけ」を投入する一回きりスクリプト（本番/ローカル共通）。
// 大学マスターには一切触れず、既存の学部を参照して志望校・学習予定・学習実績を作る。
// 本番へは EC2 経由のポートフォワーディング（SSM）で DATABASE_URL を向けて実行する想定。
//   例) DATABASE_URL="mysql://user:pass@127.0.0.1:3307/db" pnpm exec tsx db/seed-demo.ts
import { seedDemoUser } from "./demo-user";
import { runSeed } from "./seed-helpers";

runSeed(() => seedDemoUser({ withLogs: true }));
