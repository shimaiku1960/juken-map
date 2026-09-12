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

apps/api/               バックエンド一式（Fastify）
└ src/
  ├ server.ts             サーバー組み立て。圧縮・JSON 解析・SPA 配信もここ
  ├ auth.ts               Better Auth の唯一の定義
  ├ context.ts            requireSession / denyDemoWrite（門番）
  ├ seo.ts                robots / sitemap / ページ別 meta の生成
  ├ routes/               HTTP の入口（認証・検証・ステータスコードのみ）
  ├ services/             ユースケース（goal / study-log / study-plan / textbook /
  │                       university / notification / sendDailyNotifications）
  ├ infra/                db（生 SQL の接続プール）, tables（テーブル1行の型）,
  │                       email, resend, microcms, line, lineLogin
  ├ test-db/              テスト用 MySQL の準備と、テストデータの作成
  ├ domain/               通知本文の組み立てなど
  ├ observability/        サービスの所要時間計測

src/shared/             2つのアプリが共有する、外部依存のない純粋関数・型のみ
├ date.ts, subjects.ts, studyStats.ts, site.ts, demo.ts
├ validations/            Zod スキーマ（リクエストの契約）
└ dto/study.ts            画面↔API で受け渡す形の型定義

db/                     テーブル定義と初期データ
├ migrations/             テーブル定義の変更（SQL）。テーブル定義の正はここ
└ seed*.ts                大学マスター・デモ・E2E・LP 撮影・負荷試験のデータ投入
```

## パッケージ管理

ルート・`apps/api`・`apps/web` をpnpm workspaceとして管理する。
`pnpm-workspace.yaml`が参加パッケージを定義し、各`package.json`に依存を宣言する。
解決結果はルートの`pnpm-lock.yaml`へ集約し、pnpmのバージョンもルートの
`packageManager`で固定する。ルートの`pnpm install`で全パッケージを導入できる。

`src/shared`は今回パッケージ化せず、既存のパス参照を維持する。そこから使うZodは
ルートに宣言する。Dockerは`node_modules/.pnpm`とAPI側の相対リンクを同じ階層で
コピーするため、手作業でルートの`node_modules`をアプリへリンクする処理は不要。
開発時は従来どおりAPIと画面を別ターミナルで起動する。

## 依存の向き

```
apps/web  ────────────────────→  shared
apps/api  routes ──→ services ──→  shared
```

- **shared は何にも依存しない。** DB・HTTP・ブラウザ API に依存するものは置かない。
  ここが崩れると、画面と API が同じものを別々に持つ状態に戻る。
- **apps/web は apps/api の中身を呼べない。** これは約束ではなく**物理的に不可能**である。
  `apps/web/tsconfig.json` の `paths` に `@/api` が無く、そもそも解決できない。
  データが必要なら HTTP を通すしかない。
- **routes は薄い。** 認証・入力検証・ステータスコードだけを持ち、処理は services に置く。
- **バックエンドのコードは `apps/api` に全部ある。** 以前は `src/backend` にも分散していたが、
  それは Next.js のモノリスを分割したときの名残で、利用者が `apps/api` だけになった時点で
  置き場所としての理由を失っていた。`src/` に残すのは 2 つのアプリが共有する `shared` だけ。

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
画面と API が同じ形をやり取りすることは `src/shared/dto/` の型が保証している。
予定と実績の一覧は、サービス（`listStudyPlans` / `listStudyLogs`）の戻り値の型をこの型にしている。

### 画面へ返す形への変換はサービスで行う（2026-09-11〜）

以前は `apps/api/src/dto/study-mapper.ts` に変換関数を置き、routes から呼んでいた。
Next.js 時代に同じ変換が画面3箇所と API に重複していたのを集めたものだった。

SPA に分けたあとは、変換を使うのが予定と実績の一覧の GET の2か所だけになった。
流れを追うときに開くファイルが1つ増えるだけだったので、ファイルを消し、
サービスが最初から画面の形で組み立てるようにした。整形は1回で済み、
routes → services → infra の3層だけで流れを追える。

**見直す条件:** cron のように、日時を `Date` のまま使いたい呼び出し元が出てきたとき。
そのときはサービスの戻り値を `Date` に戻し、変換を呼び出し元へ分ける。

### routes は DB を触らない

`apps/api/src/routes/` に `prisma` の直接呼び出しは**0件**。DB へ行くのは
`services/` だけで、routes は次の4つだけを持つ。

1. 認証（`requireSession`）
2. 権限（`denyDemoWrite`）
3. 入力検証（Zod の `safeParse`）
4. ステータスコードへの翻訳（`P2002` → 409、`null` → 404）

**この順番に意味がある。** 誰か分からない人に入力の良し悪しを教えないため、
認証 → 権限 → 入力の順で門番を並べている。

サービス層は HTTP を知らない。「見つからない」は `null`、「一意制約違反」は Prisma の
例外のまま返し、それを 404 や 409 にするかは routes が決める。こうしておくと、
cron やバッチなど HTTP 以外の入口からも同じ処理を呼べる。

所有者チェックは `findFirst({ where: { id, userId } })` の形に統一した。以前は
`findUnique` で引いてから `userId` を比べていたが、取得と判定が1回のクエリで済み、
比較の書き忘れも起きない。

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
  最大の利点で、当時の `apps/api/src/dto/study-mapper.ts` はその型を
  `Prisma.StudyPlanGetPayload<{ include: ... }>` として受けていた。schema を変えると変換側が
  型エラーになり、直し忘れに気づける。自前の型に詰め替えると、この検出が効かなくなる。
- **`select` の柔軟性が失われる。** 画面ごとに必要な列は違う。`listGoalsWithFaculty` は
  `tags` を引かず `listGoals` は引く、という使い分けをしている（`tags` は暗黙的多対多なので
  SQL が1本増える）。メソッドを固定すると、この差を表現するために際限なく増える。
- **層が1つ増える。** 個人開発の規模では、通過するだけのファイルが増える負担が、
  差し替え可能性という使わない利点を上回る。

**見直す条件**は、DB を実際に移す必要が出たとき、または Prisma を直接モックできず
テストが書けない場面が繰り返し出てきたとき。

### ORM をやめて生 SQL へ（2026-09-11 完了）

Prisma を段階的に外し、`mysql2` で SQL を直接書く形へ移した。
クエリ・Better Auth・マイグレーション・seed のすべてから Prisma を無くした。

**理由: 「ORM があると処理が追いにくい」ため。学習目的も兼ねる。** 1回の呼び出しの裏で
何本の SQL が流れるか（`include` は JOIN ではなく `IN (...)` の別クエリになる）、
`updateMany` の条件付き更新がどんな SQL か、が API の書き方に隠れていた。

`services/` はすべて移行済み（study-plan・study-log・textbook・university・user・notification・sendDailyNotifications・goal・line-connection）。
予定と実績で共通の列と、JOIN の結果を入れ子に戻す関数は `services/study-columns.ts` にある。
Better Auth（`auth.ts`）も同じ mysql2 のプールを使う（内部の Kysely で読み書きする）。
アプリの実行時も seed（`db/seed*.ts`）もマイグレーションの適用も Prisma を使っていない。
seed は `db/seed-helpers.ts` 経由でアプリと同じ接続プールを使い、日時の扱い（UTC）もアプリと揃えている。
`schema.prisma`・生成コード・Prisma の依存パッケージも消した（2026-09-11）。テーブル定義の正は
`db/migrations` の SQL、行の型は `infra/tables.ts`。

### マイグレーション（テーブル定義の変更）

`prisma migrate deploy` の代わりに `apps/api/src/infra/migrations.ts` が当てる。
本番はコンテナの起動時（`docker-entrypoint.sh`）、ローカルは `pnpm dev` / `pnpm run db:migrate`、
CI は E2E の前、テストは globalSetup で流す。

- `db/migrations/<名前>/migration.sql` を名前順に見て、まだ当てていないものだけを流す。
  既存の22本はそのまま使う。**各マイグレーションのディレクトリ名は変えない。** 名前が
  `_prisma_migrations` の `migration_name` として本番の DB に記録されており、変えると
  当て直しになる（親ディレクトリは `prisma/` から `db/` へ改名済み。こちらは DB に記録が
  無いので影響しない）。
- 当てた記録は、Prisma が使っていた表 `_prisma_migrations` にそのまま書く。本番の DB に残る
  Prisma の記録を引き継げるので、移し替えは要らない。checksum も Prisma と同じ「ファイルの SHA-256」。
  Prisma で当てた DB と、この仕組みで当てた DB のテーブル定義・記録が一致することを確かめてある。
- MySQL の CREATE / ALTER はトランザクションで取り消せない。途中で失敗したら「失敗した」記録を
  残して止まり、人が DB を直して記録の `rolled_back_at`（やり直す）か `finished_at`（手で当て終えた）
  を埋めるまで、次の実行も止まる（Prisma と同じ振る舞い）。本番では起動が止まるので、
  デプロイのスモークテストが落ちて前のイメージに戻る。
- `GET_LOCK` で、同時に2つ動いても二重に当てない。
- **新しいマイグレーションは SQL を手で書く。** ORM がスキーマの差分から作ってくれることはもう無い。
  `db/migrations/<YYYYMMDDHHMMSS>_<内容>/migration.sql` を足し、テーブル1行の型
  （`infra/tables.ts`）も合わせて直す。当てたあとの migration.sql は書き換えない（変えても DB には
  反映されず、警告だけが出る）。直すときは新しいマイグレーションを足す。

ORM を外すと、次のことを自分で持つことになる。どれも `infra/db.ts` とテストで押さえている。

- **日時の時間帯。** MySQL の `DATETIME` は時間帯を持たない。Prisma は UTC として読み書き
  していたが、ドライバの既定はプロセスのローカル時刻で、Mac（JST）では9時間ずれる。
  `timezone: "Z"` で UTC に揃えている。
- **真偽値。** `BOOLEAN` は `TINYINT(1)` なので `1 / 0` が返る。`typeCast` で直している。
- **`updatedAt`。** Prisma の `@updatedAt` は DB の機能ではなく Prisma が毎回値を足していた。
  列に既定値は無いので、INSERT / UPDATE で必ず書く。
- **一意制約違反。** Prisma の `P2002` の代わりに MySQL の `ER_DUP_ENTRY`（1062）で判定する
  （`isDuplicateEntry`）。
- **型。** 上の「Prisma の型推論」は失われた。行の型は `infra/tables.ts` に手で書いており、
  列を足してもここを直し忘れたら型エラーにならない。
- **入れ子の組み立てと SQL の本数。** `include` は JOIN ではなく、親を取ってから子を
  `IN (...)` で別に取る。生 SQL では自分で選ぶ。大学 → 学部 → タグのような一本道の
  1対多は JOIN 1本で取って詰め直す。ユーザー → 予定・実績のように1対多が並ぶときは、
  JOIN すると（予定 × 実績）の行に膨らみ合計がずれるので、別々の SQL に分ける
  （`sendDailyNotifications.ts` の `findRecipients`）。
- **並び順。** Prisma が子を取る SQL には `ORDER BY` が無く、並びは DB が返した順だった
  （大学一覧のタグの順がそうだった）。親の `ORDER BY` も、同じ値の行どうしの順番までは決めない
  （予定・実績の一覧は同じ日付の中の順番が DB 任せだった）。生 SQL では最後に id で並べて順番を固定している。
- **`upsert`。** Prisma は MySQL では SELECT してから INSERT か UPDATE を選ぶ。
  生 SQL では `INSERT ... ON DUPLICATE KEY UPDATE` の1文で行い、その間に割り込む隙間が無い。
  ただし UNIQUE が2つ以上あるテーブルでは使わない。ON DUPLICATE KEY はどの UNIQUE の重複でも
  発動するので、`LineConnection`（`userId` と `lineUserId` が UNIQUE）で他人の LINE とぶつかると、
  エラーにならず他人の行を更新してしまう（テスト用 DB で実際に確かめた）。そこでは
  `userId` で UPDATE し、1行も変わらなければ INSERT する。

上の「リポジトリ層は導入していない」は変わらない。何を取るかは services、DB との通信は
infra という分担は、ORM の有無と関係なく同じだった。

## 境界の強制

物理的に守れない部分だけを ESLint で見る（`eslint.config.mjs`）。

- **shared は apps 配下を import できない。** これは同じ tsconfig の下にあるので型解決では
  防げず、ルールが要る。
- `apps/**` は独自の tsconfig と依存を持つ別パッケージなので、ルートの lint 対象から外している。

## テスト

| 対象 | 実行 | 内容 |
|---|---|---|
| `src/shared` | ルートの vitest | 純粋関数、Zod スキーマ |
| `apps/api` | `pnpm --filter @juken-map/api test` | エンドポイントの門番（401 / 403 / 400 / 404 / 409）、通知本文、外部連携 |
| `apps/web` | `pnpm --filter @juken-map/web test` | 画面まわりの純粋関数 |
| 通し | Playwright | 記録→可視化の毎日ループ、デモ閲覧専用、モバイルナビ |

`apps/api` のテストは Fastify の `inject()` を使う。ハンドラを直接呼ばないのは、
**本物のルーティングと JSON 解析を通すため**である。とくに JSON 解析は
「壊れた JSON でも 400 にせず認証チェックを先に効かせる」という細工が入っており
（`server.ts` のコメント参照）、ハンドラ直呼びではここが素通りしてしまう。

差し替えるのは認証・外部サービスという境界だけで、門番のロジックは実物を動かす。

**生 SQL に移したサービスのテストは、DB も差し替えず本物の MySQL に流す。** ORM の呼び出しを
モックしても SQL の誤りは分からず、SQL 文字列を照合するテストは書き方を変えただけで壊れるため。
一意制約による 409 や同時アクセスの挙動も、本物の DB でしか確かめられない。

- テスト用 DB は開発用とは別の `juken_map_test`。vitest の globalSetup
  （`apps/api/src/test-db/global-setup.ts`）が作成とマイグレーションを行う。
  取り違え防止のため、DB 名が `_test` で終わらなければ何もせずに止まる。
- ローカルでは `pnpm dev:infra` で DB コンテナを起動しておく必要がある。CI は `check` ジョブに
  MySQL サービスを持つ。
- テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる
  （`test-db/fixtures.ts`）。テーブルを空にする方式と違い、並列に走る他のテストと干渉しない。
- 往復（書いて読む）だけのテストでは時間帯の誤りが打ち消されて見えないので、
  `infra/db.test.ts` で DB 側の生の値と突き合わせている。CI（UTC）でもずれを検出できるよう、
  テストは `TZ=Asia/Tokyo` で動かす。
- モックするのは外部の境界（認証のセッション取得、LINE・メールなどの外部 API）だけ。

## デプロイ

`apps/web` のビルド成果物と `apps/api` を1つのイメージに入れ、EC2 上の Docker で動かす。
nginx（EC2 ホスト上）が 443 を受けて 3000 番へ流す。設定の実物は `infra/nginx/README.md`。

`src/shared` は `apps/*` の外にあるが、pnpm workspaceのルート依存からZodなどを解決できる。
Dockerイメージにもworkspaceと同じ階層でpnpmの依存をコピーするため、個別のsymlinkは不要である。
