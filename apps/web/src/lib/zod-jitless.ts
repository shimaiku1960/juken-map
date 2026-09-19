// Zod 4 は最初の検証で new Function を試し、使えれば検証を速くする（JIT）。使えなくても
// 例外を握りつぶして普通の検証に戻るので害は無いが、CSP はこの試しを違反として報告する
// （本番のダッシュボードで実際に報告された）。画面の検証はフォームの数件だけで速さは要らない
// ので、Zod 自身が用意している jitless で試しをやめる。
//
// z.config({ jitless: true }) を呼ぶと、そのために最初の JS へ Zod 全体（gzip 約3.4KB）が入る。
// Zod は設定を globalThis.__zod_globalConfig から読む（あれば使い、無ければ作る）ので、
// Zod が読み込まれる前にここへ置いておく。内部の名前に頼るため、zod-jitless.test.ts で
// Zod が実際にこの値を読むことを確かめている（Zod を上げて名前が変わればテストが落ちる）。
const target = globalThis as { __zod_globalConfig?: { jitless?: boolean } };
target.__zod_globalConfig = { ...target.__zod_globalConfig, jitless: true };

// 副作用だけのファイルだが、型検査でモジュールとして扱わせる（テストから import するため）。
export {};
