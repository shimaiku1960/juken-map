import { permanentRedirect } from "next/navigation";

// LP はトップ `/` に統合済み。重複コンテンツを避けるため、
// 旧 URL `/lp` は恒久リダイレクト（308）で `/` に集約する。
export default function LpRedirect() {
  permanentRedirect("/");
}
