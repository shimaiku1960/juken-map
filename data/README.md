# 大学マスターデータ

## 出所

ASTI アマノ技研「国内大学の位置データ」
<https://amano-tec.com/data/univs.html>

- ライセンス: 研究利用・商用利用とも無料（制限なし）
- 取得日: 2026-06-23

## ファイル構成

- `raw/asti-datr0805uj/r0501univs_utf8.csv` … 公式配布のフル版CSV（大学名・住所など）。同意フォーム経由で手動ダウンロード
- `raw/asti-datr0805uj/univs_map.tsv` … 地図用エンドポイント（`https://amano-tec.com/_userdata/astiunivs.php`）の出力。設置区分（type列: 1国立 / 2公立 / 3私立 / 4短大 / 5大学校）を含む
- `clean/universities.json` … 上記2ファイルを `scripts/transform-universities.ts` で整形した投入用データ（大学のみ823校、`{ name, prefecture, type }`）

## 更新手順

1. アマノ技研からフル版CSVを再取得し `raw/` を差し替え
2. `curl https://amano-tec.com/_userdata/astiunivs.php -o data/raw/asti-datr0805uj/univs_map.tsv`
3. `pnpm exec tsx scripts/transform-universities.ts` で `clean/universities.json` を再生成
4. `pnpm exec prisma db seed` でDBへ upsert
