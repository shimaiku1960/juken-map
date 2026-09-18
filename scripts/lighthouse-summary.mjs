// Lighthouse CI の結果（manifest.json）から、URL ごとの表を Markdown で出す。
// 中央値の回（isRepresentativeRun）の値と、全回の幅を並べる。
// Lighthouse はフォントの取得などで回ごとに大きく揺れるので、1回の値だけでは判断しない。
//
// 使い方: node scripts/lighthouse-summary.mjs <resultsPath> [links.json の中身]
import { readFileSync } from "node:fs";
import path from "node:path";

const [resultsPath, linksJson] = process.argv.slice(2);
if (!resultsPath) throw new Error("結果のフォルダを渡してください");

const manifest = JSON.parse(readFileSync(path.join(resultsPath, "manifest.json"), "utf-8"));
const links = linksJson ? JSON.parse(linksJson) : {};

const METRICS = [
  { label: "スコア", get: (lhr) => lhr.categories.performance.score * 100, format: (v) => v.toFixed(0) },
  { label: "FCP", get: (lhr) => lhr.audits["first-contentful-paint"].numericValue, format: seconds },
  { label: "LCP", get: (lhr) => lhr.audits["largest-contentful-paint"].numericValue, format: seconds },
  { label: "TBT", get: (lhr) => lhr.audits["total-blocking-time"].numericValue, format: (v) => `${v.toFixed(0)}ms` },
  { label: "CLS", get: (lhr) => lhr.audits["cumulative-layout-shift"].numericValue, format: (v) => v.toFixed(3) },
  { label: "転送量", get: (lhr) => lhr.audits["total-byte-weight"].numericValue, format: (v) => `${(v / 1024).toFixed(0)}KB` },
];

function seconds(ms) {
  return `${(ms / 1000).toFixed(1)}秒`;
}

const byUrl = new Map();
for (const run of manifest) {
  const lhr = JSON.parse(readFileSync(run.jsonPath, "utf-8"));
  const entry = byUrl.get(run.url) ?? { runs: [], representative: undefined };
  entry.runs.push(lhr);
  if (run.isRepresentativeRun) entry.representative = lhr;
  byUrl.set(run.url, entry);
}

const lines = [
  "| URL | 回数 | " + METRICS.map((m) => m.label).join(" | ") + " |",
  "|---|---|" + METRICS.map(() => "---").join("|") + "|",
];
for (const [url, { runs, representative }] of byUrl) {
  const cells = METRICS.map((metric) => {
    const values = runs.map(metric.get);
    const median = metric.format(metric.get(representative ?? runs[0]));
    if (runs.length === 1) return median;
    return `**${median}**<br>(${metric.format(Math.min(...values))}〜${metric.format(Math.max(...values))})`;
  });
  const name = new URL(url).pathname;
  const label = links[url] ? `[${name}](${links[url]})` : name;
  lines.push(`| ${label} | ${runs.length} | ${cells.join(" | ")} |`);
}

console.log(lines.join("\n"));
console.log(
  "\n太字は中央値の回、かっこ内は全回の幅。モバイル想定（回線・CPUを絞った条件）。URL のリンクは詳しいレポート（7日で消える）。"
);
