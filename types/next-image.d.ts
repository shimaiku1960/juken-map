// next-env.d.ts は .gitignore 済みで、CI のクリーンチェックアウトには存在しない。
// `npm run check` は `next build` より前に `tsc --noEmit` を実行するため、
// next-env.d.ts が生成される前に画像の static import が型エラーになる。
// 画像モジュールの宣言だけはコミット済みのこのファイルから参照する。
/// <reference types="next/image-types/global" />
