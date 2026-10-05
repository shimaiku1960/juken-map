import { defineConfig, globalIgnores } from "eslint/config";
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import globals from "globals";
import { sqlInjectionRules } from "./eslint.sql.config.mjs";

// レイヤー境界（docs/architecture.md）を ESLint で強制する。
//
//   apps/web  ──→  shared
//   db        ──→  shared（seed が日付の関数を使う）
//
// shared は画面と seed が共有する最下層で、何にも依存しない。ここが崩れると、
// 画面と API が同じものを別々に持つ状態へ戻る。
//
// 注意: apps/ は独自の tsconfig を持つ別パッケージなので lint 対象から外している。
// アプリ同士（apps/web → apps/api-go の中身など）を直接参照する経路は、そもそも
// tsconfig の paths に無いので物理的に解決できない。ここで守るのは shared の純度だけ。
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
              group: ["@/api/*", "@/api", "@/web/*", "@/web", "@/apps/*"],
              message:
                "shared は何にも依存しない層です。apps 配下のコードを import しないでください。" +
                "両方のアプリで使いたいものは shared の中に置き、片方でしか使わないものは" +
                "そのアプリへ移してください。",
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
  // SQL を文字列の連結・埋め込みで組み立てない（dev-standards 06 D1、JUK-104）。
  { files: ["db/**/*.ts"], rules: sqlInjectionRules },
  globalIgnores([
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
