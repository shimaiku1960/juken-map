import { defineConfig, globalIgnores } from "eslint/config";
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import globals from "globals";

// レイヤー境界（docs/architecture.md）を ESLint で強制する。
//
//   apps/web  ──→  shared
//   apps/api  ──→  backend  ──→  shared
//
// apps/web（画面）と apps/api（HTTP の入口）が上の層。backend は DB や外部連携を持ち、
// shared は何にも依存しない。この向きが崩れると、フロントとバックが同じものを
// 別々に持つ状態へ戻る。
//
// 注意: apps/ は独自の tsconfig を持つ別パッケージなので lint 対象から外している。
// ここで守れるのは src/ 側の依存の向きだけで、apps/web から backend を呼ぶ経路は
// そもそも解決できない（tsconfig の paths に無い）ため物理的に不可能になっている。
const layerBoundaries = [
  {
    // shared は最下層。誰にも依存してはいけない。
    files: ["src/shared/**/*.ts"],
    rules: {
      "no-restricted-imports": [
        "error",
        {
          patterns: [
            {
              group: ["@/backend/*", "@/backend"],
              message:
                "shared は何にも依存しない層です。backend のコードを import しないでください。" +
                "両方で使いたいものは shared の中に置き、片方でしか使わないものはその層へ移してください。",
            },
          ],
        },
      ],
    },
  },
];

export default defineConfig([
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...layerBoundaries,
  {
    // このリポジトリのコードはすべて Node で動く（サーバー・シード・スクリプト・テスト）。
    // ブラウザ側は apps/web が持ち、そちらは対象外。
    languageOptions: { globals: globals.node },
    // テストは vitest のグローバルを使わず import しているので追加設定は不要。
    rules: {
      // 型で表現しきれない箇所で any を使っている既存コードがあるため警告に留める。
      "@typescript-eslint/no-explicit-any": "warn",
    },
  },
  globalIgnores([
    // Prisma の生成物。自動生成コードは対象外。
    "src/backend/generated/**",
    // apps/ は独自の tsconfig と依存を持つ別パッケージ。
    "apps/**",
    // 実行成果物
    "playwright-report/**",
    "test-results/**",
    "terraform/**",
    // k6 のスクリプト。Node でもブラウザでもない実行環境（__ENV 等）なので対象外。
    "load-tests/**",
  ]),
]);
