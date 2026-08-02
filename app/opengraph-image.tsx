import { ImageResponse } from "next/og";
import { readFile } from "node:fs/promises";
import { join } from "node:path";

// SNS やチャットに URL を貼ったときに出るサムネイル画像。
// ImageResponse（内部は satori）は日本語フォントを渡さないと豆腐（□）になるので、
// assets/fonts のサブセットを読み込んで渡す。文言に使える漢字は
// assets/fonts/README.md に書いた範囲だけである点に注意。

export const alt = "受験マップ｜今日の勉強を、合格までの積み重ねに。";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

// ブランドカラー（public/brand-mark.svg・app/manifest.ts と同じ値）
const BRAND = "#007364";
const BRAND_DARK = "#00463c";
const PAPER = "#f6fbfa";

const fontPath = (file: string) => join(process.cwd(), "assets", "fonts", file);

const Image = async () => {
  const [regular, bold, mark] = await Promise.all([
    readFile(fontPath("NotoSansJP-Regular-subset.ttf")),
    readFile(fontPath("NotoSansJP-Bold-subset.ttf")),
    readFile(join(process.cwd(), "public", "brand-mark.svg")),
  ]);

  // satori は SVG ファイルを直接読めないので data URI にして img で貼る。
  const markSrc = `data:image/svg+xml;base64,${mark.toString("base64")}`;

  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "76px 84px",
          backgroundImage: `linear-gradient(135deg, ${BRAND} 0%, ${BRAND_DARK} 100%)`,
          color: PAPER,
          fontFamily: "Noto Sans JP",
        }}
      >
        {/* 右上の光。単色べた塗りを避けて奥行きを出すだけの飾り */}
        <div
          style={{
            position: "absolute",
            top: -220,
            right: -180,
            width: 620,
            height: 620,
            borderRadius: 620,
            backgroundColor: "rgba(246, 251, 250, 0.07)",
            display: "flex",
          }}
        />

        <div style={{ display: "flex", alignItems: "center" }}>
          <img src={markSrc} width={76} height={76} alt="" />
          <div style={{ fontSize: 36, fontWeight: 700, marginLeft: 22 }}>受験マップ</div>
        </div>

        <div style={{ display: "flex", flexDirection: "column" }}>
          <div style={{ fontSize: 76, fontWeight: 700, lineHeight: 1.3 }}>今日の勉強を、</div>
          <div style={{ fontSize: 76, fontWeight: 700, lineHeight: 1.3 }}>
            合格までの積み重ねに。
          </div>
        </div>

        <div
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          <div style={{ fontSize: 30, opacity: 0.85 }}>
            学習の開始・記録・振り返りをひとつに。
          </div>
          <div style={{ fontSize: 26, opacity: 0.7 }}>juken-map.com</div>
        </div>
      </div>
    ),
    {
      ...size,
      fonts: [
        { name: "Noto Sans JP", data: regular, weight: 400, style: "normal" },
        { name: "Noto Sans JP", data: bold, weight: 700, style: "normal" },
      ],
    }
  );
};

export default Image;
