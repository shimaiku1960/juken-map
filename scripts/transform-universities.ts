// アマノ技研の2ファイルを突き合わせて、アプリ用の大学マスターJSONを生成する。
//   - フル版CSV (r0501univs_utf8.csv): 大学名 + 住所（→都道府県）
//   - 地図用TSV (univs_map.tsv):       設置区分（type列: 1国立/2公立/3私立/4短大/5大学校）
// 2ファイルは同じ並び順・同じ件数なので「行の位置」で結合する。
//
// 実行: pnpm exec tsx scripts/transform-universities.ts

import { readFileSync, writeFileSync, mkdirSync } from "fs";

const CSV_PATH = "data/raw/asti-datr0805uj/r0501univs_utf8.csv";
const MAP_PATH = "data/raw/asti-datr0805uj/univs_map.tsv";
const OUT_PATH = "data/clean/universities.json";

// 都道府県（住所の先頭からこのいずれかで始まる）
const PREFECTURES = [
  "北海道", "青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県",
  "茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県",
  "新潟県", "富山県", "石川県", "福井県", "山梨県", "長野県", "岐阜県",
  "静岡県", "愛知県", "三重県", "滋賀県", "京都府", "大阪府", "兵庫県",
  "奈良県", "和歌山県", "鳥取県", "島根県", "岡山県", "広島県", "山口県",
  "徳島県", "香川県", "愛媛県", "高知県", "福岡県", "佐賀県", "長崎県",
  "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
];

const TYPE_LABEL: Record<string, string> = {
  "1": "国立",
  "2": "公立",
  "3": "私立",
};

// 都道府県を省いて市名から始まる住所のフォールバック（政令指定都市など）
const CITY_TO_PREF: Record<string, string> = {
  札幌市: "北海道", 仙台市: "宮城県", さいたま市: "埼玉県", 千葉市: "千葉県",
  横浜市: "神奈川県", 川崎市: "神奈川県", 相模原市: "神奈川県", 新潟市: "新潟県",
  静岡市: "静岡県", 浜松市: "静岡県", 名古屋市: "愛知県", 京都市: "京都府",
  大阪市: "大阪府", 堺市: "大阪府", 神戸市: "兵庫県", 岡山市: "岡山県",
  広島市: "広島県", 北九州市: "福岡県", 福岡市: "福岡県", 熊本市: "熊本県",
};

function extractPrefecture(address: string): string | null {
  const pref = PREFECTURES.find((p) => address.startsWith(p));
  if (pref) return pref;
  const city = Object.keys(CITY_TO_PREF).find((c) => address.startsWith(c));
  return city ? CITY_TO_PREF[city] : null;
}

function main() {
  const csvLines = readFileSync(CSV_PATH, "utf-8").trim().split(/\r?\n/);
  const mapLines = readFileSync(MAP_PATH, "utf-8").trim().split(/\r?\n/);

  // 先頭行はヘッダーなので除く
  const csvRows = csvLines.slice(1);
  const mapRows = mapLines.slice(1);

  if (csvRows.length !== mapRows.length) {
    throw new Error(
      `行数が一致しません: csv=${csvRows.length} map=${mapRows.length}（位置結合の前提が崩れています）`
    );
  }

  const seen = new Set<string>();
  const result: { name: string; prefecture: string; type: string }[] = [];

  for (let i = 0; i < csvRows.length; i++) {
    const csv = csvRows[i].split("\t");
    const map = mapRows[i].split("\t");

    const name = csv[3]?.trim();
    const address = csv[6]?.trim() ?? "";
    const typeCode = map[0]?.trim();

    // 大学だけ（短大4・大学校5は除外）
    const typeLabel = TYPE_LABEL[typeCode];
    if (!typeLabel) continue;

    // 同一大学名（複数キャンパス）は最初の1件に集約
    if (!name || seen.has(name)) continue;

    const prefecture = extractPrefecture(address);
    if (!prefecture) {
      console.warn(`都道府県を抽出できませんでした: ${name} / ${address}`);
      continue;
    }

    seen.add(name);
    result.push({ name, prefecture, type: typeLabel });
  }

  mkdirSync("data/clean", { recursive: true });
  writeFileSync(OUT_PATH, JSON.stringify(result, null, 2) + "\n", "utf-8");

  const byType = result.reduce<Record<string, number>>((acc, r) => {
    acc[r.type] = (acc[r.type] ?? 0) + 1;
    return acc;
  }, {});
  console.log(`生成: ${OUT_PATH}  計${result.length}校`, byType);
}

main();
