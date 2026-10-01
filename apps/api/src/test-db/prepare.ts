// テスト用 DB（juken_map_test）を、Node のテストを流さずに用意する。
// Go の DB テスト（apps/api-go の dbtest タグ、JUK-97）が、Node のテストと同じ DB・同じ権限のユーザーを使うため。
// 中身は vitest の globalSetup と同じ（DB とユーザーを作り、マイグレーションを最新まで当てる）。
import setup from "./global-setup.ts";

await setup();
