import { afterEach, describe, expect, it, vi } from "vitest";
import { injectMeta, type PageMeta } from "./seo.ts";

const html = "<html><head><title>x</title></head><body></body></html>";
const meta: PageMeta = {
  title: "受験マップ",
  description: "説明",
  ogTitle: "受験マップ",
  ogType: "website",
  ogImage: "https://juken-map.com/og.png",
  noindex: false,
};

describe("injectMeta の Faro の送り先", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it("FARO_COLLECTOR_URL があれば meta で画面へ渡す", () => {
    vi.stubEnv("FARO_COLLECTOR_URL", "https://faro.example/collect/abc");

    expect(injectMeta(html, meta)).toContain(
      '<meta name="faro-collector-url" content="https://faro.example/collect/abc"/>'
    );
  });

  it("無ければ meta を出さない（画面は送信しない）", () => {
    vi.stubEnv("FARO_COLLECTOR_URL", "");

    expect(injectMeta(html, meta)).not.toContain("faro-collector-url");
  });

  it("値は属性として安全に埋め込む", () => {
    vi.stubEnv("FARO_COLLECTOR_URL", 'https://faro.example/"><script>');

    const out = injectMeta(html, meta);
    expect(out).not.toContain('"><script>');
  });
});
