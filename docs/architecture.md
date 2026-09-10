# アーキテクチャ

受験マップは **React + Vite の SPA（`apps/web`）** と **Fastify の API（`apps/api`）** の
2つのアプリからなる。共有するコードは `src/` に置き、責務で `backend` と `shared` に分かれる。

もとは Next.js のモジュラーモノリスだった。2026-09-10 に分離へ切り替え、Next.js は削除した。
経緯と手順は `docs/split-migration-plan.md`、性能まわりの計測は `docs/performance.md` にある。

## ディレクトリ

```
apps/web/               画面（React + Vite の SPA）
├ src/pages/              ルートに対応する画面
├ src/components/         画面部品（ui/ は shadcn/ui）
├ src/hooks/              TanStack Query のサーバー状態フック
├ src/lib/                utils(cn), analytics, auth-client, browser, demo-client,
│                         prefectures, studySession, studyLog, studyPlan, examSchedule
└ public/                 favicon, PWA アイコン, manifest, robots.txt, LP 素材

apps/api/               HTTP の入口（Fastify）
├ src/server.ts           サーバー組み立て。圧縮・JSON 解析・SPA 配信もここ
├ src/auth.ts             Better Auth の唯一の定義
├ src/context.ts          requireSession / denyDemoWrite（門番）
├ src/seo.ts              robots / sitemap / ページ別 meta の生成
└ src/routes/             エンドポイント定義（認証・検証・ステータスコードのみ）

src/backend/            データアクセス・外部連携・ドメインロジック
├ infra/                  prisma, email, resend, microcms, line, lineLogin
├ services/               ユースケース（goal / study-log / study-plan / textbook /
│                         university / notification / sendDailyNotifications）
├ dto/study-mapper.ts     Prisma の戻り値 → 共有 DTO への変換
├ domain/dailyNotification.ts  通知本文の組み立て
└ observability/measured.ts    サービスの所要時間計測

src/shared/             外部依存のない純粋関数・型・定数データのみ
├ date.ts, subjects.ts, studyStats.ts, site.ts, demo.ts
├ validations/            Zod スキーマ（リクエストの契約）
└ dto/study.ts            画面↔API で受け渡す形の型定義
```

## 依存の向き

```
apps/web  ──────────────→  shared
apps/api  ──→  backend  ──→  shared
```

- **shared は何にも依存しない。** DB・HTTP・ブラウザ API に依存するものは置かない。
  ここが崩れると、画面と API が同じものを別々に持つ状態に戻る。
- **apps/web は backend を呼べない。** これは約束ではなく**物理的に不可能**である。
  `apps/web/tsconfig.json` の `paths` に `@/backend` が無く、そもそも解決できない。
  データが必要なら HTTP を通すしかない。
- **apps/api は薄い。** 認証・入力検証・ステータスコードだけを持ち、処理は backend の
  サービス層に置く。

### Next.js のときとの違い

以前は `app/` が frontend と backend の両方を import できる唯一の入口だった。同じプロセスに
同居していたので、境界は**フォルダと ESLint による約束**にすぎなかった。

プロセスを分けたことで、その約束は**型解決の仕組みそのもの**に置き換わった。画面から
サービス層を直接呼ぶコードは、書いても import が解決しない。守り方としてはこちらが強い。

## 判断とその理由

実装は git 履歴を見れば分かるが、選ばなかった選択肢とその理由はコードに残らないため、
ここに書く。

### Fastify が API と SPA の両方を配る

nginx に静的ファイルを配らせ、`/api` だけ Fastify へ振る構成も検討した（当初の計画はこちら）。
採らなかった理由は**性能ではなくリスクの形**である。

デプロイはコンテナ単位でスモークテストと自動ロールバックが組まれている
（`.github/scripts/deploy-ec2.sh`）。静的ファイルをホストに置くと、戻す対象が「コンテナ」と
「静的ファイル」の2系統に割れ、既存の安全網が片方しか守らなくなる。

Fastify を 3000 番で待ち受けさせれば、nginx は
`location / { proxy_pass http://localhost:3000; }` のままでよい。結果として：

- **切り替えで本番ホストに一切触らずに済んだ**（SSM での手作業がゼロ）
- 既存のスモークテストと自動ロールバックがそのまま効く
- SPA と API のバージョンがずれることが原理的に起きない

代償は、静的配信を nginx ではなく Node が担うこと。この規模では実測で問題にならない。
必要になれば nginx に `location /api` を足すだけで移せる（アプリ側は無変更）。

### SPA 化で失う SEO を、サーバー側で作り直した

SPA は誰が来ても同じ `index.html` を返す。JS を実行する前の HTML しか読まないクローラーと
SNS には、中身が空に見える。Next.js が黙って担っていた分を `apps/api/src/seo.ts` で作り直した。

- `robots.txt` と OGP 画像は `apps/web/public` の実ファイル
- `sitemap.xml` は記事一覧から作るので Fastify のルート（microCMS 障害時も固定ページ分は返す）
- `index.html` を返すときに head を差し込む。`/articles/:id` は microCMS からタイトル・説明・
  アイキャッチを引く。ログイン後ページと認証フローには `noindex` を付ける

**放置すると `/robots.txt` が 200 で HTML を返す**という、404 より質の悪い状態になっていた。

なお `fastify-static` の `index` は切ってある。切らないと `/` に `index.html` が直接返り、
一番 SEO が要るトップページだけ meta が入らない。

一覧やアプリ内の画面は今も JS 実行後にしか中身が出ない。これは SPA である限り残る弱点で、
実害と認識のうえで許容している。

### データ取得の入口は REST API に統一している

**理由: 将来スマートフォンアプリから同じサーバーを使いたいから。** Next.js 時代は Server
Actions を使わない理由として書いていたが（URL が固定されない、識別子がビルドごとに変わる、
ボディが React 独自形式）、分離した今はそもそも選択肢が存在しない。

`POST /api/study-logs` に JSON を送るだけで、Web もアプリも同じ入口を使える。
画面と API が同じ形をやり取りすることは `src/shared/dto/` の型と
`src/backend/dto/study-mapper.ts` が保証している。

### 画面から DB を触らない

**理由は「DB を隠すため」ではなく「同じクエリが増殖するのを防ぐため」。** Next.js 時代、
`app/profile/page.tsx` と `app/api/notification-preferences/route.ts` に1文字違わず同じ
`findUnique` が書かれていた（`docs/performance.md` の TASK 3）。片方だけ `select` を直せば、
画面と API で返るデータが静かにずれる。

サービス層に集約すれば直す場所は1つになる。加えて全サービスが `measured()` を通るので、
どのクエリが遅いかがログから追える。画面に直書きされたクエリはこの計測から漏れる。

分離後は、このルールは ESLint ではなく構成そのものが保証している（上記「依存の向き」）。

### リポジトリ層は導入していない

サービス層が Prisma を直接呼んでいる。`GoalRepository` のような層は挟んでいない。

**理由: 得られるものより失うものが大きいと判断したため。**

利点は「DB を差し替えられること」だが、MySQL から移る予定は無い。一方で失うものは具体的である。

- **Prisma の型推論が切れる。** `include` の内容に応じて戻り値の型が変わるのが Prisma の
  最大の利点で、`src/backend/dto/study-mapper.ts` はその型を
  `Prisma.StudyPlanGetPayload<{ include: ... }>` として受けている。schema を変えると変換側が
  型エラーになり、直し忘れに気づける。自前の型に詰め替えると、この検出が効かなくなる。
- **`select` の柔軟性が失われる。** 画面ごとに必要な列は違う。`listGoalsWithFaculty` は
  `tags` を引かず `listGoals` は引く、という使い分けをしている（`tags` は暗黙的多対多なので
  SQL が1本増える）。メソッドを固定すると、この差を表現するために際限なく増える。
- **層が1つ増える。** 個人開発の規模では、通過するだけのファイルが増える負担が、
  差し替え可能性という使わない利点を上回る。

**見直す条件**は、DB を実際に移す必要が出たとき、または Prisma を直接モックできず
テストが書けない場面が繰り返し出てきたとき。現状はルートのテストで
`vi.mock("@/backend/infra/prisma")` によりモックできている。

## 境界の強制

物理的に守れない部分だけを ESLint で見る（`eslint.config.mjs`）。

- **shared は backend を import できない。** これは同じ tsconfig の下にあるので型解決では
  防げず、ルールが要る。
- `apps/**` は独自の tsconfig と依存を持つ別パッケージなので、ルートの lint 対象から外している。
- Prisma の生成物（`src/backend/generated/**`）も対象外。

## テスト

| 対象 | 実行 | 内容 |
|---|---|---|
| `src/shared`・`src/backend` | ルートの vitest | 純粋関数、Zod スキーマ、通知本文の組み立て |
| `apps/api` | `npm run test --prefix apps/api` | エンドポイントの門番（401 / 403 / 400 / 404 / 409） |
| `apps/web` | `npm run test --prefix apps/web` | 画面まわりの純粋関数 |
| 通し | Playwright | 記録→可視化の毎日ループ、デモ閲覧専用、モバイルナビ |

`apps/api` のテストは Fastify の `inject()` を使う。ハンドラを直接呼ばないのは、
**本物のルーティングと JSON 解析を通すため**である。とくに JSON 解析は
「壊れた JSON でも 400 にせず認証チェックを先に効かせる」という細工が入っており
（`server.ts` のコメント参照）、ハンドラ直呼びではここが素通りしてしまう。

差し替えるのは DB・認証・外部サービスという境界だけで、門番のロジックは実物を動かす。

## デプロイ

`apps/web` のビルド成果物と `apps/api` を1つのイメージに入れ、EC2 上の Docker で動かす。
nginx（EC2 ホスト上）が 443 を受けて 3000 番へ流す。設定の実物は `infra/nginx/README.md`。

`Dockerfile` に2つ symlink がある。npm workspaces を使っていないため、`/app/src` や
`/app/src/backend/generated` からのモジュール解決が `apps/*/node_modules` に届かないための
橋渡しである。workspaces へ移せば不要になる。
