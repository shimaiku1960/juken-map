import { defineConfig } from "eslint/config";
import tseslint from "typescript-eslint";

// SQL を文字列の連結・埋め込みで組み立てている箇所を見つける（セキュリティ基準 06 の D1、JUK-104）。
// `pnpm lint:sql` で、本体の eslint.config.mjs とは別に流す。本体の設定は apps/ を対象から外していて
// （全体にかけるのは JUK-53）、ここでは SQL の規則だけを apps/api と db/ にかける。
// Go（apps/api-go）は gosec の G201・G202 が同じことを見る（ci.yml の go のジョブ）。
//
// 値は必ずプレースホルダー（?）で渡す。プレースホルダーにできない部分（固定の列名の並び・? を件数ぶん
// 並べる・定数のテーブル名）だけを埋め込んでよく、その行には `eslint-disable-next-line no-restricted-syntax
// -- <なぜ安全か>` を書く。理由の無い許可や、使われていない許可は --report-unused-disable-directives で落ちる。
const SQL = String.raw`/\b(SELECT|INSERT INTO|UPDATE|DELETE FROM|REPLACE INTO)\b/`;
const message =
  "SQL を文字列の連結・埋め込みで組み立てています。値はプレースホルダー（?）で渡してください。" +
  "固定の列名など、どうしても埋め込む場合は、その行に eslint-disable-next-line で理由を書いてください（JUK-104）。";

// 本体の eslint.config.mjs も db/ にこの規則をかける（db/ は本体の対象なので、かけないと許可のコメントが
// 「使われていない」と警告される）。規則の中身はここ1か所に置く。
export const sqlInjectionRules = {
  "no-restricted-syntax": [
    "error",
    // `SELECT ... ${x}` のような、値を埋め込んだテンプレート文字列
    { selector: `TemplateLiteral[expressions.length>0]:has(TemplateElement[value.raw=${SQL}])`, message },
    // "SELECT ... " + x のような連結
    { selector: `BinaryExpression[operator='+']:has(Literal[value=${SQL}])`, message },
    { selector: `BinaryExpression[operator='+']:has(TemplateElement[value.raw=${SQL}])`, message },
  ],
};

export default defineConfig([
  {
    files: ["apps/api/src/**/*.ts", "db/**/*.ts"],
    // テストは外から呼ばれない。テスト用 DB の名前など、決まった値を埋め込んでいる。
    ignores: ["**/*.test.ts"],
    languageOptions: { parser: tseslint.parser },
    linterOptions: { reportUnusedDisableDirectives: "error" },
    rules: sqlInjectionRules,
  },
]);
