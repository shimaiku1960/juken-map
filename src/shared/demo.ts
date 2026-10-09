// デモアカウント（面接官向け・閲覧専用）のメールアドレス。
//
// 画面が「今デモで見ているか」の判定（編集の入口を隠す）に使う。
// 書き込みを 403 で断るのは Go の apps/api/internal/httpx/router.go の DemoEmail で、値をそろえておく。
export const DEMO_EMAIL = "demo@juken-map.com";
