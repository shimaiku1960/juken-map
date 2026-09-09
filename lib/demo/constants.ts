// デモアカウント（面接官向け・閲覧専用）のメールアドレス。
//
// 画面側（Server Component 5ファイル）が「今デモで見ているか」の判定に使い、
// サーバー側の guard.ts が 403 判定に使う。guard.ts は next/server に依存する
// サーバー専用モジュールなので、定数だけをここへ分けて画面から参照できるようにした。
export const DEMO_EMAIL = "demo@juken-map.com";
