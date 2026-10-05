// 管理 API が「管理者だが2段階認証を通したセッションではない」ときに 403 の本文へ入れる印。
// サーバー（apps/api-go/router.go の管理者の確認）が付け、画面（管理ページ）が見て、
// 権限が無いのではなく2段階認証の設定・ログインし直しが要ることを案内する。
export const TWO_FACTOR_REQUIRED = "TWO_FACTOR_REQUIRED";
