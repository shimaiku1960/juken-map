import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

// レイヤー境界（docs/architecture.md）を ESLint で強制する。
//
//   app/  ──→  frontend  ──→  shared
//     └─────→  backend   ──→  shared
//
// app/ は両方を呼べる唯一の入口。frontend と backend は互いを知らず、
// shared は何にも依存しない。この向きが崩れると、フロントとバックが
// 同じものを別々に持つ状態へ戻る。
const PRISMA_CLIENT = "@/app/generated/prisma/client";

const layerBoundaries = [
  {
    // shared は最下層。誰にも依存してはいけない。
    files: ["src/shared/**/*.ts", "src/shared/**/*.tsx"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [
          {
            group: ["@/frontend/*", "@/backend/*", "@/app/*", "@/frontend", "@/backend"],
            message:
              "shared は何にも依存しない層です。frontend・backend・app のコードを import しないでください。" +
              "両方で使いたいものは shared の中に置き、片方でしか使わないものはその層へ移してください。",
          },
        ],
      }],
    },
  },
  {
    // frontend は UI の責務。DB や外部連携には触れない。
    files: ["src/frontend/**/*.ts", "src/frontend/**/*.tsx"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [
          {
            group: ["@/backend/*", "@/backend"],
            message:
              "frontend から backend を直接呼ばないでください。データ取得は入口（app/ の page.tsx / layout.tsx）で行い、" +
              "結果を props で渡してください。両方で使う純粋な処理は @/shared へ置きます。",
          },
          {
            group: [PRISMA_CLIENT, `${PRISMA_CLIENT}/*`],
            message: "UI 層から DB に直接アクセスしないでください。@/backend/services 経由で呼びます。",
          },
        ],
      }],
    },
  },
  {
    // backend が UI を参照すると、サーバー処理が画面都合に引きずられる。
    files: ["src/backend/**/*.ts", "src/backend/**/*.tsx"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [
          {
            group: ["@/frontend/*", "@/frontend"],
            message:
              "backend から frontend を import しないでください。両方で使う型や純粋関数は @/shared に置きます。",
          },
        ],
      }],
    },
  },
  {
    // app/ の画面側は UI の組み立てだけ。DB へは触らない。
    // API の入口（app/api/**/route.ts）は Prisma のエラー型などを扱うため対象外。
    files: ["app/**/page.tsx", "app/**/layout.tsx", "app/**/template.tsx"],
    rules: {
      "no-restricted-imports": ["error", {
        patterns: [
          {
            group: [PRISMA_CLIENT, `${PRISMA_CLIENT}/*`, "@/backend/infra/prisma"],
            message:
              "UI 層から DB に直接アクセスしないでください。@/backend/services のサービス関数を呼びます。" +
              "適切な関数が無ければ、サービス層に追加してから使ってください。",
          },
        ],
      }],
    },
  },
];

const eslintConfig = defineConfig([
  ...nextVitals,
  ...nextTs,
  ...layerBoundaries,
  // Override default ignores of eslint-config-next.
  globalIgnores([
    // Default ignores of eslint-config-next:
    ".next/**",
    "out/**",
    "build/**",
    "next-env.d.ts",
    // Prisma の生成物。自動生成コードは対象外。
    "app/generated/**",
  ]),
]);

export default eslintConfig;
