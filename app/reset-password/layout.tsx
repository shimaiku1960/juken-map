import type { Metadata } from "next";

import { NOINDEX } from "@/lib/site";

// page.tsx が "use client" のため、そちらから metadata を export できない。
// 検索結果に出す必要のないページなので、この layout で noindex だけを付ける。
export const metadata: Metadata = { robots: NOINDEX };

const ResetPasswordLayout = ({ children }: { children: React.ReactNode }) => children;

export default ResetPasswordLayout;
