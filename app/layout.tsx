import type { Metadata } from "next";
import { Geist_Mono, Noto_Sans_JP } from "next/font/google";
import "./globals.css";
import Header from "./components/Header";
import { Toaster } from "@/components/ui/sonner";
import Providers from "./providers";
import { SITE_URL } from "@/lib/site";

const notoSansJp = Noto_Sans_JP({
  variable: "--font-noto-sans-jp",
  subsets: ["latin"],
  display: "swap",
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  // OGP 画像や canonical の相対パスを絶対 URL に解決する基準。
  // これが無いと og:image が相対 URL のままになり、SNS 側で画像を取得できない。
  metadataBase: new URL(SITE_URL),
  title: "受験マップ",
  description:
    "学習の開始から時間記録、予定と実績の確認、科目別の振り返りまでをひとつにつなぐ、大学受験生向け学習管理アプリです。",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="ja"
      className={`${notoSansJp.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
      <Toaster />
      <Header />  
      <Providers>{children}</Providers>
      </body>
    </html>
  );
}
