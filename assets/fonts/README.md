# OGP 画像用フォント

`app/opengraph-image.tsx` が `ImageResponse`（内部は satori）で画像を組み立てるときに使う。
satori はフォントを埋め込まないと日本語が豆腐（□）になるため、リポジトリに置いている。

- 書体: Noto Sans JP（Regular 400 / Bold 700）
- ライセンス: SIL Open Font License 1.1（`OFL.txt`）
- 出どころ: Google Fonts

## サブセットについて

フル版は 1 ウェイト数 MB あるので、**必要な文字だけ**に絞ってある（各 130KB 程度）。
含まれるのは ASCII・ひらがな全部・カタカナ全部・主な約物と、下記の漢字のみ。

```
受験今日勉強合格積重学習記録時間予定実績科目志望校大学入試毎分析振返計画自成長無料始開一二三年月週回見直進続終力英語国数理社会文本番過去問模試点目標
```

**ここに無い漢字を OGP の文言に使うと、その字だけ表示されない。**
（実際、初回は「開」が抜けていて `学習の開始・…` が欠けた。文言を変えたら必ず下の検証を通すこと）

文言を変えて字が足りなくなったら、下のコマンドで取り直す（`kanji` に文字を足す）。

```bash
python3 - <<'PY'
import re, urllib.parse, urllib.request

ascii_printable = "".join(chr(c) for c in range(0x20, 0x7F))
hiragana = "".join(chr(c) for c in range(0x3041, 0x3097))
katakana = "".join(chr(c) for c in range(0x30A1, 0x30FB))
punctuation = "、。・ー「」『』（）〜！？：；／…—　"
kanji = "受験今日勉強合格積重学習記録時間予定実績科目志望校大学入試毎分析振返計画自成長無料始開一二三年月週回見直進続終力英語国数理社会文本番過去問模試点目標"
chars = ascii_printable + hiragana + katakana + punctuation + kanji

for weight, name in (("400", "Regular"), ("700", "Bold")):
    query = urllib.parse.urlencode({"family": f"Noto Sans JP:{weight}", "text": chars})
    req = urllib.request.Request(
        f"https://fonts.googleapis.com/css?{query}", headers={"User-Agent": ""}
    )
    css = urllib.request.urlopen(req).read().decode()
    # text= 指定だと URL が l/font?kit=... 形式になり拡張子が付かない
    url = re.search(r"url\((https://[^)]+)\)\s*format\('truetype'\)", css).group(1)
    data = urllib.request.urlopen(url).read()
    with open(f"assets/fonts/NotoSansJP-{name}-subset.ttf", "wb") as fh:
        fh.write(data)
    print(f"{name}: {len(data):,} bytes")
PY
```

> User-Agent を空にするのが肝。ブラウザの UA を送ると woff2 が返り、satori が読めない。

## 文言を変えたら必ず流す検証

OGP に出す文字がフォントに揃っているかを確かめる。`欠け: なし` なら OK。

```bash
python3 - <<'PY'
from fontTools.ttLib import TTFont

# app/opengraph-image.tsx に書いた文字列と揃えること
texts = [
    "受験マップ",
    "今日の勉強を、",
    "合格までの積み重ねに。",
    "学習の開始・記録・振り返りをひとつに。",
    "juken-map.com",
]
for path in (
    "assets/fonts/NotoSansJP-Regular-subset.ttf",
    "assets/fonts/NotoSansJP-Bold-subset.ttf",
):
    cmap = TTFont(path).getBestCmap()
    missing = sorted({ch for t in texts for ch in t if ord(ch) not in cmap})
    print(path.split("/")[-1], "→ 欠け:", missing if missing else "なし")
PY
```
