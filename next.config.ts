import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // standalone は Docker ビルド時のみ生成する（本番 EC2 の pm2 は next start を使い
  // standalone を必要としない。生成すると node_modules を丸ごとコピーしてディスクを圧迫する）
  output: process.env.BUILD_STANDALONE === "true" ? "standalone" : undefined,
  images: {
    remotePatterns: [new URL("https://images.microcms-assets.io/assets/**")],
  },
};

export default nextConfig;
