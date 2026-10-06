# アーキテクチャ

受験マップは **React + Vite の SPA（`apps/web`）** と **Go の API（`apps/api`）** の
2つのアプリからなる。本番では Go が API・ログイン・ビルド済みの画面のすべてを配る（JUK-70・JUK-111・JUK-115）。
開発でしか使わない DB の道具（seed・テスト用 DB の準備・Node の DB 接続）は `db/` に置く。以前の `apps/api`（Node）は、
Fastify のサーバーを JUK-121 で、マイグレーションの適用を JUK-125 で Go へ移し、残りを JUK-130 で `db/` へ寄せて消した。
画面と共有するコードは `src/shared` に置く。Go のファイルの分け方とルートの足し方は `apps/api/README.md`。

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

apps/api/            バックエンド一式（Go）。ファイルの分け方は apps/api/README.md
                     （今は package main 1つ。cmd/・internal/ へ移す途中、下の「バックエンドの構成」、JUK-148）

db/                     DB の道具。開発でしか使わない Node の TS（workspace の @juken-map/db、JUK-130）
├ migrations/             マイグレーションの SQL（当てるのは Go の migrate、JUK-125）
├ seed*.ts                seed（pnpm db:seed など）と seed-helpers
├ connection.ts           seed とテストが使う接続プール（mysql2）
├ db-users.ts             本番の DB ユーザーの権限（print-user-grants.ts とテスト用ユーザーが使う）
└ test-db/                テスト用 MySQL の準備と、テストデータの作成（Go の DB テストも使う）

src/shared/             2つのアプリが共有する、外部依存のない純粋関数・型のみ
├ date.ts, subjects.ts, studyStats.ts, site.ts, demo.ts
├ validations/            Zod スキーマ（リクエストの契約）
└ dto/study.ts            画面↔API で受け渡す形の型定義
```

## パッケージ管理

ルート・`apps/web`・`db` をpnpm workspaceとして管理する（`apps/api` は Go のモジュールで、workspace の外）。
`pnpm-workspace.yaml`が参加パッケージを定義し、各`package.json`に依存を宣言する。
解決結果はルートの`pnpm-lock.yaml`へ集約し、pnpmのバージョンもルートの
`packageManager`で固定する。ルートの`pnpm install`で全パッケージを導入できる。

`src/shared`は今回パッケージ化せず、既存のパス参照を維持する。そこから使うZodは
ルートに宣言する。Dockerは`node_modules/.pnpm`とAPI側の相対リンクを同じ階層で
コピーするため、手作業でルートの`node_modules`をアプリへリンクする処理は不要。
開発時は従来どおりAPIと画面を別ターミナルで起動する。

## 依存の向き

```
apps/web  ──→  shared                      （TS の import）
db        ──→  shared                      （seed が日付の関数を使う）
apps/web  ── HTTP（JSON）──→  apps/api      （Go。TS のコードは import しない）
openapi/openapi.yaml  ──生成──→  shared/openapi.gen.ts・apps/api/internal/apischema/openapi.gen.go

apps/api の中:  router.go（入口の種類で拒否）──→ ハンドラ（入力・状態コード）──→ ストア（SQL）
```

- **shared は何にも依存しない。** DB・HTTP・ブラウザ API に依存するものは置かない。
  ここが崩れると、画面と API が同じものを別々に持つ状態に戻る。
- **apps/web は apps/api の中身を呼べない。** これは約束ではなく**物理的に不可能**である。
  API は Go なので、TS から import する経路がそもそも無い。データが必要なら HTTP を通すしかない。
- **画面と API が同じ形をやり取りすることは、契約から作った型が保証する。** 形の正は
  `openapi/openapi.yaml` で、TS の型（`src/shared/openapi.gen.ts`）と Go の型（`apps/api/internal/apischema/openapi.gen.go`）を
  `pnpm openapi:generate` で作る（JUK-76）。入力チェックの規則（Zod）は Go が手で同じものを書く（JUK-75、
  理由は `apps/api/README.md` の「書き込みのルート」）。
- **API の中は、入口 → ハンドラ → ストアの順に呼ぶ。** 分担は下の「入口・ハンドラ・ストアの分担」。
- **バックエンドのコードは `apps/api` に全部ある。** 以前は `src/backend` にも分散していたが、
  それは Next.js のモノリスを分割したときの名残で、利用者が `apps/api` だけになった時点で
  置き場所としての理由を失っていた。`src/` に残すのは 2 つのアプリが共有する `shared` だけ。

### Next.js のときとの違い

以前は `app/` が frontend と backend の両方を import できる唯一の入口だった。同じプロセスに
同居していたので、境界は**フォルダと ESLint による約束**にすぎなかった。

プロセスを分けたことで、その約束は**型解決の仕組みそのもの**に置き換わった。画面から
API の中身を直接呼ぶコードは、書いても import が解決しない。守り方としてはこちらが強い。
API を Go に移した（JUK-70）今は、言語も違うので、なおさら混ざらない。

## 判断とその理由

実装は git 履歴を見れば分かるが、選ばなかった選択肢とその理由はコードに残らないため、
ここに書く。

### 1つのコンテナが API と画面の両方を配る

分離した当初（2026-09）は Node の Fastify が配り、2026-10 に Go へ移した（JUK-111）。下の理由はどちらにも当てはまる。

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

代償は、静的配信を nginx ではなくアプリ（当時は Node、今は Go）が担うこと。この規模では実測で問題にならない。
必要になれば nginx に `location /api` を足すだけで移せる（アプリ側は無変更）。

**2026-10、JUK-111 で配る役目を Go（`apps/api`）へ移した。** 考え方は同じで、Go のコンテナが API と画面の
両方を配る。画面のビルド成果物は Node のイメージから写して Go のイメージに入れるので、画面と API の版がずれない
ことも、デプロイのスモークテストが両方を守ることも変わらない。dist は起動時にメモリへ読み、圧縮できるものは
gzip を作り置く（`apps/api/spa.go`）。

### SPA 化で失う SEO を、サーバー側で作り直した

SPA は誰が来ても同じ `index.html` を返す。JS を実行する前の HTML しか読まないクローラーと
SNS には、中身が空に見える。Next.js が黙って担っていた分をサーバー側で作り直した
（はじめは Node の `seo.ts`、JUK-111 から Go の `apps/api/seo.go`）。

- `robots.txt` と OGP 画像は `apps/web/public` の実ファイル
- `sitemap.xml` は SSG した記事から起動時に作るので API サーバーのルート
- `index.html` を返すときに head を差し込む。ログイン後ページと認証フローには `noindex` を付ける
- 記事（`/articles/:id`）はビルドで microCMS から全件取り、本文入りの HTML と meta（`dist/ssg/meta.json`）を
  作り置く（JUK-110）。記事を更新したら、デプロイ（`deploy.yml`）をやり直して作り直す。
  以前は表示のたびにサーバーで描いていた（SSR）が、本番のサーバーを Go だけにするため（JUK-109）、
  Go では描けない React の描画をビルドへ移した。規約・プライバシーポリシーも同じ仕組みで SSG している

**放置すると `/robots.txt` が 200 で HTML を返す**という、404 より質の悪い状態になっていた。

なお `/` も、ほかの画面と同じく head を差し込んでから返す（`apps/api/spa.go`）。Node の頃は
`fastify-static` の `index` を切って同じことをしていた。切らないと `/` に `index.html` が直接返り、
一番 SEO が要るトップページだけ meta が入らなかった。

一覧やアプリ内の画面は今も JS 実行後にしか中身が出ない。これは SPA である限り残る弱点で、
実害と認識のうえで許容している。

### データ取得の入口は REST API に統一している

**理由: 将来スマートフォンアプリから同じサーバーを使いたいから。** Next.js 時代は Server
Actions を使わない理由として書いていたが（URL が固定されない、識別子がビルドごとに変わる、
ボディが React 独自形式）、分離した今はそもそも選択肢が存在しない。

`POST /api/study-logs` に JSON を送るだけで、Web もアプリも同じ入口を使える。
画面と API が同じ形をやり取りすることは、同じ契約（`openapi/openapi.yaml`）から作った型が保証している
（画面は `src/shared/dto/`、Go は `apps/api/internal/apischema/openapi.gen.go`）。予定と実績の一覧は、ストア
（`study.go` の `listStudyPlans` / `listStudyLogs`）の戻り値の型をこの型にしている。

### 画面へ返す形は、SQL を読むところで組み立てる（2026-09-11〜）

以前は `apps/api/src/dto/study-mapper.ts`（Node）に変換関数を置き、routes から呼んでいた。
Next.js 時代に同じ変換が画面3箇所と API に重複していたのを集めたものだった。

SPA に分けたあとは、変換を使うのが予定と実績の一覧の GET の2か所だけになった。
流れを追うときに開くファイルが1つ増えるだけだったので、ファイルを消し、
SQL を読むところで最初から画面の形に組み立てるようにした。Go でも同じで、日時は `time.Time` にせず
DB の文字列のまま受けて ISO 文字列に直す（`apps/api/internal/database` の `ParseTime = false`）。

**見直す条件:** 日時を時刻の値のまま計算したい呼び出し元が増えたとき。そのときは読む型を
`time.Time` に戻し、画面の形への変換を呼び出し元へ分ける。

### 入口・ハンドラ・ストアの分担

Go の API は、1つの機能を1つのファイル（大きいものは数ファイル）に置き、その中を
**ハンドラ**（HTTP を読み書きする）と**ストア**（SQL を流す）に分ける。その手前に、入口の種類ごとの
拒否（`router.go`）がある。Node の頃の `routes/` と `services/` の分担を、Go へ移すとき（JUK-70）に
フォルダではなくファイルの中の分担にした。

1. **入口（`router.go`）**：未ログイン・停止中・管理者でない・デモの書き込みを断る。ルートは
   `rt.user` や `rt.admin` のように種類を選んで登録し、種類を選ばずに登録する方法が無いので、書き忘れが起きない
2. **ハンドラ**：本文と入力を確かめ（`readBody`・`readObject`）、自分の行かを確かめ、
   ストアの結果をステータスコードへ翻訳する（見つからない → 404、重複 → 409 など）
3. **ストア**：SQL を流し、失敗は値（`admin_masters.go` の `masterOutcome`）か目印のエラー
   （`study_plan_writes.go` の `errAlreadyCompleted`）で返す。ステータスコードは知らない。
   一意制約違反の判定（`database.IsMySQLError(err, database.DuplicateEntry)`、`internal/database`）もストアの中で済ませる

**この順番に意味がある。** 誰か分からない人に入力の良し悪しを教えないため、
入口の拒否 → 入力の順で門番を並べている。入力の確認は「自分の行か」より先に行う（Node と同じ応答にするため）。

Node の頃は、予定の完了時の範囲チェックや「実績済みの予定は未完了に戻せない」がルートに
書かれていて、サービスを直接呼ぶと素通りできた（JUK-17 で services へ移した）。Go では予定を完了にする
ストア（`studyPlanWriteStore`）をハンドラ以外から呼ぶ所が無いので、範囲チェックのようなルールはハンドラに置いている。
ハンドラの外（タイマーのジョブや運用のコマンド）から同じ書き込みをすることになったら、ルールをストアか共通の関数へ移す。
この条件は、運用のコマンド（`incident.go`）が管理画面と同じ利用停止を行うようになった時点で満たしていたので、
利用停止は持ち主の操作（`internal/write/account` の `Suspend`）にして、両方から呼ぶようにした（JUK-151）。
書き込みは持ち主へ集める（下の「バックエンドの構成」、JUK-148）。

所有者チェックは `WHERE id = ? AND userId = ?` の形に統一している（`findStudyPlan` など）。取得と判定が
1回のクエリで済み、比較の書き忘れも起きない。他人の ID を渡しても読み書きできないことは
`ownership_db_test.go` が user の全ルートについて確かめる。

### 画面から DB を触らない

**理由は「DB を隠すため」ではなく「同じクエリが増殖するのを防ぐため」。** Next.js 時代、
`app/profile/page.tsx` と `app/api/notification-preferences/route.ts` に1文字違わず同じ
`findUnique` が書かれていた（`docs/performance.md` の TASK 3）。片方だけ `select` を直せば、
画面と API で返るデータが静かにずれる。

API に集約すれば直す場所は1つになる。加えて全リクエストがアクセスログと計測（`apps/api/middleware.go`）を
通るので、どの API が遅いかがログとメトリクスから追える（Node の頃は全サービスが `measured()` を通していた）。
画面に直書きされたクエリはこの計測から漏れる。

分離後は、このルールは ESLint ではなく構成そのものが保証している（上記「依存の向き」）。

### バックエンドの構成：読み取りは入口の近く、書き込みは持ち主へ（2026-10-06 決定、JUK-148 で移行中）

テーブルごとに読み書きを隠すリポジトリ層（`GoalRepository` のようなもの）は入れない。そのかわり、
**決まりを含む書き込みだけ、データのまとまりごとに持ち主を1か所に決める**。読み取りは、
画面や入口ごとの処理の近くに置いたままにする。

2026-09-13 には「リポジトリ層は入れない」とだけ決め、見直す条件を「同じテーブルへの SQL が
3ファイル以上に散ったとき」にしていた。2026-10-06 に測ると、その条件を満たしていた。ただし、
散らばり方は読み取りと書き込みでまったく違った。

- **読み取り**：`Textbook` を8ファイル、`FinalGoal` を6ファイルが読む。どれも画面ごとに列も JOIN も
  違い、同じ処理の重複ではない。
- **書き込み**：`user` の UPDATE が9ファイル、`DELETE FROM AuthSession` が5か所にある。
  利用停止（停止の印を付けて、その人のセッションを消す）は `admin_users.go` と `incident.go` の2か所にあり、
  前者は1つのトランザクションで、後者は別々に流していた（JUK-151 で `account.Suspend` の1つにした）。`firstStudyLogAt` の更新は
  `study_log_writes.go` と `study_plan_writes.go` に同じ SQL が2つあった（JUK-153 で `studyrecord` の中の1つにした）。

**理由：問題が起きているのは書き込みだけで、読み取りを集めても得るものが無いため。**

- **書き込みの誤りは DB に残る。** 読み取りの誤りは表示がおかしくなるだけで、直せば戻る。書き込みには
  「停止したら全端末を落とす」「初回の日時は1回だけ付ける（`WHERE firstStudyLogAt IS NULL`）」のような
  決まりがそのまま入っているので、2か所にあると片方だけ直される。利用停止は、実際にずれていた。
- **読み取りには隠せる情報が無い。** 生 SQL は画面ごとに取る列が違うので、1か所に集めると
  「SQL 1本 = メソッド1個」になり、名前を付け替えるだけの層になる。同じ列を何度も読むものは、
  列の並びを定数（`goals.go` の `goalColumns`、`study_plan_writes.go` の `studyPlanRowColumns` など）にまとめてある。
- **リポジトリ層の利点は、このプロジェクトには当てはまらない。** よく挙がる利点は
  「DB を差し替えられること」と「テストでモックしやすいこと」だが、MySQL から移る予定は無く、
  テストは本物のテスト用 MySQL で行っている。残る「データの扱いを1か所に集める」は、書き込みだけで足りる。
- **業務ルールが SQL そのものになっている。** 予定の完了は、`UPDATE ... WHERE firstStudyLogAt IS NULL`
  での初回判定、UNIQUE 制約と `ER_DUP_ENTRY` による二重完了の検出、トランザクションで成り立っている。
  「ルールはサービス、SQL はリポジトリ」と分けると1つのルールが2つのファイルに割れるので、
  持ち主はルールと SQL を一緒に持つ。

読み取りと書き込みで設計を分けるのは CQRS の軽い形にあたり、書き込みを「一緒に変わるまとまり」で
持つのは DDD の集約（aggregate）にあたる。

#### 目標の構成

外枠は Go 公式の構成ガイド（https://go.dev/doc/modules/layout ）の `cmd/`・`internal/` に従う。
その下の分け方は受験マップに合わせたもの。

```
apps/api/
├ cmd/api/main.go         起動の入口（サーバーと運用のコマンドを1つのバイナリで受ける）
└ internal/
  ├ app/                  起動・終了、依存の組み立て、ルートとアクセス条件の一覧
  ├ feature/              画面・入口ごとの処理（ハンドラと読み取りの SQL）
  │ ├ auth/  study/  goals/  textbooks/  dashboard/
  │ └ admin/  line/  ops/
  ├ write/                書き込みの持ち主（操作とトランザクション）
  │ ├ account/  authguard/  studyrecord/  textbook/  goal/
  │ └ university/  textbookmaster/  notification/  simulation/   （一覧は下の「持ち主の一覧」）
  ├ httpx/                HTTP の共通処理（本文の読み取り・エラー応答・入力チェック・ルーター）
  ├ database/             DB 接続
  ├ telemetry/            ログ・メトリクス・トレース
  ├ apischema/            OpenAPI から生成した型
  ├ spa/                  画面と SEO の配信
  ├ migrate/              マイグレーションの適用
  └ dbtest/               DB テストの補助（テスト用 DB への接続・テスト用の利用者やデータ）
```

`feature/` と `write/` の直下には Go のファイルを置かず、その下の `study` や `studyrecord` を
パッケージにする。ディレクトリを見ただけで、「どの入口の処理か」と「誰が変更に責任を持つか」が分かる。
機能ごとのファイル数は処理の量に合わせてよく、全機能に同じファイル一式をそろえる必要はない。

| 処理 | 置き場所 |
|---|---|
| 学習の一覧を、参考書名も含めて読む | `feature/study`（読み取りの SQL） |
| JSON を読み、HTTP の応答を作る | `feature/study`（ハンドラ） |
| 予定を完了できるか確かめ、実績を作り、初回記録日時を付ける | `write/studyrecord` |
| 管理画面から利用者を停止する | `feature/admin` → `write/account` |
| 運用のコマンドから利用者を停止する | `feature/ops` → 同じ `write/account` |

#### 決まり

1. **feature 同士は呼び合わない。** 必要な読み取りは自分で書き、変更は write を呼ぶ。
2. **write は HTTP を知らない。** `httpx` や `apischema`（画面に返す形）に依存せず、操作に必要な引数と
   結果の型を自分で決める。作った行を画面へ返すときは、feature がその形へ直す。
3. **持ち主が外に出すのは「操作」。** `account.Suspend` が停止の印とセッションの削除をまとめて行い、
   `studyrecord.CompletePlan` が予定の完了・実績の作成・初回記録日時の更新をまとめて行う。
   呼ぶ側に、複数の関数を正しい順番で呼ぶ責任を残さない。
4. **トランザクションと条件の保証は、操作の中で完結させる。** 呼ぶ側が済ませた所有者の確認などを
   そのまま信用せず、必要な条件は操作の中で保証する（`WHERE id = ? AND userId = ?`）。
   書き込むための読み取り（自分の行かの確認など）も、書き込みの一部として持ち主に置く。
5. **単純な更新にも持ち主を決める。** 志望校の更新が単純なら、`write/goal` は小さな具体型と関数だけでよい。
   例外を作らないのは、「INSERT・UPDATE・DELETE は `internal/write/` の下にしか無い」をテストで確かめられるように
   するためである。例外があると、テストが例外の一覧の管理になり、守られなくなる。
   アプリのデータではないものを書く `internal/migrate`（マイグレーションの記録）と `internal/dbtest`
   （テストデータの作成）の2つだけは、最初から対象の外にする。
6. **インターフェースや追加の層は先に作らない。** 使う側が必要になったら、使う側で定義する。
   今、一部のストア（管理画面のマスター・利用者、LINE、通知）を interface にしているのは、DB を使わない
   テストで失敗を作るためで、DB を差し替えるためではない。
7. **持ち主をまたいで同時に確定させたい操作が出たら、境界を見直す。** 別々にコミットする関数を順番に呼んで
   済ませない。なお退会は、`DELETE FROM user` の1文で外部キーの `ON DELETE CASCADE` が全部消すので
   （`auth_delete_account.go`）、これに当たらない。

#### 持ち主の一覧（2026-10-06、JUK-150）

書き込みを含む約80の関数（アプリの表は27）を洗い出し、**1つのトランザクションで一緒に変える表どうしを同じ持ち主にした**。
一緒に変える操作が無い表どうしは、関係が深くても分けた。

| 持ち主 | 書く表 | 同じ持ち主にした根拠（一緒に確定する操作） |
|---|---|---|
| `account` | `user`（下の列を除く）・`AuthSession`・`AuthPassword`・`AuthToken`・`AuthIdentity`・`AuthTotp`・`AuthBackupCode`・`AuthMfaChallenge`・`OpsAuditLog` | 利用停止（停止の印＋セッション削除）、パスワードの再設定（パスワード＋セッション・トークン・確認待ちの削除）、OAuth の連携（利用者＋連携＋パスワード・セッション・トークンの削除）、2段階認証の設定とリセット（TOTP＋予備コード＋確認待ち）、退会と管理者の削除（`DELETE FROM user` と外部キーの連鎖） |
| `authguard` | `AuthOAuthState`・`AuthThrottle`・`EmailSend` | ログインの途中の一時的な状態と、回数の制限。`account` の表と一緒に変える操作は無い |
| `studyrecord` | `StudyLog`・`StudyPlan`・`user.firstStudyLogAt` | 予定の完了（予定＋実績＋初回記録日時）、実績の記録（実績＋初回記録日時） |
| `textbook` | `Textbook` | 学習記録と一緒に変える操作は無い（予定の完了は総量を読むだけ） |
| `goal` | `FinalGoal` | 単独 |
| `university` | `University`・`Faculty`・`_FacultyToTag` | 学部の作成・変更（学部＋タグの付け替え） |
| `textbookmaster` | `TextbookMaster`・`TextbookMasterMetric` | 参考書マスターの作成・変更（マスター＋測り方の付け替え）。大学と一緒に変える操作は無いので、`catalog` にはまとめない |
| `notification` | `LineConnection`・`LineLinkNonce`・`LineOAuthAttempt`・`LineWebhookEvent`・`NotificationPreference`・`NotificationDelivery` | LINE の連携の解除（連携＋確認用の値＋通知の設定） |
| `simulation` | `user.simSeq`・`simCohort`・`simLastActedOn`・`simDormantFrom` | 負荷のシミュレーション（`/admin/sim`）の利用者の印。ほかの持ち主の列と一緒に変える操作は無い |

決めたこと：

- **`user` は列ごとに持ち主を分ける。** 行を作る・消す・停止する・権限を変えるのは `account`。初回記録日時は、
  実績と一緒に確定させるので `studyrecord`。シミュレーションの列は `simulation`。ニックネームの変更と、
  登録の計測を送った印（`analyticsSignUpTrackedAt`）は `account` に置く。
- **監査ログ（`OpsAuditLog`）は `account` に置く。** 書いているのは運用のコマンドだけで、記録する操作
  （停止・解除・セッションの削除・権限・2段階認証のリセット）はすべて `account` の操作である。
  操作と同じトランザクションで書く。記録が書けなければ操作も取り消す（記録の無い変更を残さない）。
  停止・解除（JUK-151）に続き、セッションの取り消し・2段階認証のリセット・権限の付け替えもこの形にした（JUK-154）。
  記録は運用のコマンドから呼んだときだけ書くので、操作は記録の付け足し（`*account.OpsAudit`）を受け取り、
  管理画面は `nil` を渡す（管理画面の操作はログに残す）。
- **期限切れの行の掃除（`expired_cleanup.go`）は、持ち主をまたぐが境界を見直さない。** 表ごとに別々に消してよく、
  同時に確定させる必要が無いため。各持ち主が「期限切れを消す」操作を出し、タイマーのジョブがそれを順に呼ぶ。
- **退会は `account` の操作のまま。** ほかの持ち主の表も消えるが、それは外部キーの `ON DELETE CASCADE` という
  DB の決まりで、コードが順番に消しているのではない。

#### 移し方

- **最初は account の利用停止で試した（JUK-151）。** 入口・持ち主・テストの分担は次のように分かれた。
  - 入口：管理画面のハンドラは HTTP（守りの確認・404・応答の形）、CLI はメールアドレスから相手を引くことと出力を持つ。
  - 持ち主：トランザクション、書くための読み取り（止める前後の `bannedAt` を行を押さえて読む）、監査ログ。
    応答の `bannedAt` は、ハンドラが先に読んだ値ではなく、持ち主が書いた後に DB から読み直した値になった。
  - テスト：持ち主のパッケージに DB テストを置けなかった。DB テストの補助が `package main` の `_test.go` にあり、
    ほかのパッケージから import できないため。補助を `internal/dbtest` へ出し（JUK-158）、`account` の操作の DB テストは
    `internal/write/account/suspend_db_test.go` に置いた。管理画面・運用のコマンドを通した確認は入口側（`package main`）に残す。
  - 管理画面のストアの interface（DB を使わないテストで結果を作るため）は残し、その中身は `account` の操作を呼ぶだけになった。
  その前に、持ち主が使う DB の補助（`InTx`・`Placeholders`・`IsMySQLError` など）を
  `internal/database` へ移した（JUK-152）。`package main` は import できないので、これが無いと持ち主のパッケージが作れない。
- **学習記録（JUK-153）は、移動と中身の変更を1つの PR にした。** 持ち主は `package main` の型（入力の
  `optional`・応答の `StudyLogRow` など）を使えないので、移すだけでも引数と戻り値を持ち主の型に作り直すことになり、
  「移動だけ」の段階が作れなかった。分かれ方は次のとおり。
  - 決まり（自分の参考書か・範囲が参考書の逆算設定に合うか・実績のある予定を未完了に戻さない）は、ハンドラから
    持ち主の操作の中へ移し、確かめと書き込みを1つのトランザクションにした（変える行は `FOR UPDATE` で押さえる）。
    断る理由は持ち主のエラー（`ErrNotFound`・`RangeError` など）で返し、入口の `writeStudyRecordError` が 404・400・409 に直す。
  - 「送られなかった」と null の区別は `internal/write/opt` の `opt.Field` で受ける（最初は studyrecord の中に置き、
    参考書の持ち主でも要ったので共通にした。JUK-154）。入力の形の確かめ（Zod と同じ 400）は入口に残す。
  - 持ち主の行の型（`Log`・`Plan`）は、項目の並びを `apischema` の型とそろえ、入口は型の変換だけで応答にする。
    予定の完了の応答に付ける参考書の行は画面の形なので、確定した後に入口が読む。
  - 実績の変更は「無ければ本文に関わらず 404」を保つため、入口が本文の確かめの前に有るかだけを見る（Node と同じ順）。
    書き込みの正しさは持ち主の中の確かめが守り、入口の確認は応答の順番のためだけにある。
- **移動だけの PR と、中身を変える PR を分ける。** 1回の PR で1パッケージにする。挙動は変えないので、
  `go build`・`go vet`・golangci-lint・`go test`・DB テスト・E2E と、ルート一覧が前と同じことで確かめる。
  多くのファイルを動かす PR は、ほかの worktree の作業とぶつかるので、並行する作業が無いときに出す。
- **Go ではディレクトリがパッケージの境界になる。** 小文字の名前は外から見えなくなり、パッケージ同士の
  循環 import はビルドが通らない。ルーターは今と同じくセッションの読み込みを関数で受け取り
  （`router.go` の `newRouter(load sessionLoader)`）、`httpx` が `write/account` に依存しない形を保つ。
- **一緒に動かすもの。** `//go:embed` は同じディレクトリかその下しか読めないので、よく使われるパスワードの一覧や
  RDS の証明書も移す。`_test.go` の補助はほかのパッケージから import できないので `internal/dbtest` に
  移し、全ルートを通すテスト（`ownership_db_test.go`）は `internal/app` に置く。`oapi-codegen.yaml` の
  `package: main`、Dockerfile の `go build .`、この文書・`apps/api/README.md`・AGENTS.md に書いたファイルの場所も直す。

**代償：** 小さな機能でも、入口と持ち主を行き来する手間がかかる。列名を変えると、読み取りのファイル全部に
影響する（本物の MySQL に流すテストで、直し漏れは落ちる）。それでも、同じ変更が管理画面・ログイン・運用のコマンドなど
複数の入口から行われ、すでに実装がずれているので、責任を明示するほうを取る。

**見直す条件：** 持ち主をまたぐ操作が続けて出て、境界の見直しが繰り返されるとき。または、書き込みの
置き場所を確かめるテストに例外を足したくなったとき。

### ORM をやめて生 SQL へ（2026-09-11 完了）

Prisma を段階的に外し、`mysql2` で SQL を直接書く形へ移した。
クエリ・Better Auth・マイグレーション・seed のすべてから Prisma を無くした。

**理由: 「ORM があると処理が追いにくい」ため。学習目的も兼ねる。** 1回の呼び出しの裏で
何本の SQL が流れるか（`include` は JOIN ではなく `IN (...)` の別クエリになる）、
`updateMany` の条件付き更新がどんな SQL か、が API の書き方に隠れていた。

当時の Node の `services/`（study-plan・study-log・textbook・university・user・notification・sendDailyNotifications・goal・line-connection）は
すべて mysql2 へ移した。Better Auth も同じ mysql2 のプールを使っていた。`schema.prisma`・生成コード・Prisma の
依存パッケージも消した（2026-09-11）。

その後、API は Go（`database/sql`）へ移して Node から消し（JUK-70・JUK-84）、ログインも Better Auth から
Go の自作に替えた（JUK-115）。いま Node の mysql2 を使うのは、開発用の seed とテストの準備（`db/`）だけで、
`db/seed-helpers.ts` 経由で `db/connection.ts` の接続プールを使い、日時の扱い（UTC）を Go と揃えている。
テーブル定義の正は `db/migrations` の SQL、Go の行の型は各ファイルの struct と、契約から作った `internal/apischema/openapi.gen.go`。

### マイグレーション（テーブル定義の変更）

`prisma migrate deploy` の代わりに Go の `migrate` コマンド（`apps/api/migrate.go`、JUK-125）が当てる。
本番はデプロイがアプリを起動する前に、Go のイメージの1回きりのコンテナ（`/api migrate`）で流す。
`db/migrations` はイメージの `/migrations` に入れてある（`deploy.yml` の `--build-context migrations=`）。
ローカルは `pnpm dev` / `pnpm run db:migrate`、CI は E2E の前、テストは globalSetup で流す。

- 本番の DB ユーザーは役割ごとに分けてある（`db/db-users.ts`）。アプリは DML だけの
  `juken_app` で繋ぎ、テーブル定義を変えられる `juken_migrate` はマイグレーションのコンテナにだけ渡す
  （`MIGRATION_DATABASE_URL`）。調査用の `juken_readonly` は SELECT だけ。テストも同じ権限のユーザーで
  動かすので、アプリが DML 以外の SQL を使い始めるとテストが落ちる。

- `db/migrations/<名前>/migration.sql` を名前順に見て、まだ当てていないものだけを流す。
  既存の22本はそのまま使う（ディレクトリ名に prisma が残るのはそのため）。
- 当てた記録は、Prisma が使っていた表 `_prisma_migrations` にそのまま書く。本番の DB に残る
  Prisma の記録を引き継げるので、移し替えは要らない。checksum も Prisma と同じ「ファイルの SHA-256」。
  Prisma で当てた DB と、この仕組みで当てた DB のテーブル定義・記録が一致することを確かめてある。
- MySQL の CREATE / ALTER はトランザクションで取り消せない。途中で失敗したら「失敗した」記録を
  残して止まり、人が DB を直して記録の `rolled_back_at`（やり直す）か `finished_at`（手で当て終えた）
  を埋めるまで、次の実行も止まる（Prisma と同じ振る舞い）。本番では起動が止まるので、
  デプロイのスモークテストが落ちて前のイメージに戻る。
- `GET_LOCK` で、同時に2つ動いても二重に当てない。
- **新しいマイグレーションは SQL を手で書く。** ORM がスキーマの差分から作ってくれることはもう無い。
  `db/migrations/<YYYYMMDDHHMMSS>_<内容>/migration.sql` を足し、その列を読み書きする Go の SQL と struct
  （応答に出る列なら `openapi/openapi.yaml` も）を合わせて直す。当てたあとの migration.sql は書き換えない（変えても DB には
  反映されず、警告だけが出る）。直すときは新しいマイグレーションを足す。

ORM を外すと、次のことを自分で持つことになる。どれも Go の `apps/api/internal/database`、seed とテスト用の `db/connection.ts` とテストで押さえている。

- **日時の時間帯。** MySQL の `DATETIME` は時間帯を持たない。Prisma は UTC として読み書き
  していたが、ドライバの既定はプロセスのローカル時刻で、Mac（JST）では9時間ずれる。
  Go は `cfg.Loc = time.UTC`、Node（`db/connection.ts`）は `timezone: "Z"` で UTC に揃えている。
- **真偽値。** `BOOLEAN` は `TINYINT(1)` なので `1 / 0` が返る。Go は `bool` へ Scan すれば変換され、
  Node は `typeCast` で直している。
- **`updatedAt`。** Prisma の `@updatedAt` は DB の機能ではなく Prisma が毎回値を足していた。
  列に既定値は無いので、INSERT / UPDATE で必ず書く。
- **一意制約違反。** Prisma の `P2002` の代わりに MySQL の `ER_DUP_ENTRY`（1062）で判定する
  （Go は `internal/database` の `IsMySQLError(err, DuplicateEntry)`）。
- **型。** Prisma の型推論は失われた。Go の行の型は struct に手で書いており、列を足しても
  直し忘れはコンパイルでは分からない（SELECT の列と Scan の受け皿の数が合わなければ、DB に流すテストで落ちる）。
- **入れ子の組み立てと SQL の本数。** `include` は JOIN ではなく、親を取ってから子を
  `IN (...)` で別に取る。生 SQL では自分で選ぶ。大学 → 学部 → タグのような一本道の
  1対多は JOIN 1本で取って詰め直す。ユーザー → 予定・実績のように1対多が並ぶときは、
  JOIN すると（予定 × 実績）の行に膨らみ合計がずれるので、別々の SQL に分ける
  （毎日の通知の宛先を集める SQL。今は Go の `notifications.go`）。
- **並び順。** Prisma が子を取る SQL には `ORDER BY` が無く、並びは DB が返した順だった
  （大学一覧のタグの順がそうだった）。親の `ORDER BY` も、同じ値の行どうしの順番までは決めない
  （予定・実績の一覧は同じ日付の中の順番が DB 任せだった）。生 SQL では最後に id で並べて順番を固定している。
- **`upsert`。** Prisma は MySQL では SELECT してから INSERT か UPDATE を選ぶ。
  生 SQL では `INSERT ... ON DUPLICATE KEY UPDATE` の1文で行い、その間に割り込む隙間が無い。
  ただし UNIQUE が2つ以上あるテーブルでは使わない。ON DUPLICATE KEY はどの UNIQUE の重複でも
  発動するので、`LineConnection`（`userId` と `lineUserId` が UNIQUE）で他人の LINE とぶつかると、
  エラーにならず他人の行を更新してしまう（テスト用 DB で実際に確かめた）。そこでは
  `userId` で UPDATE し、1行も変わらなければ INSERT する。

テーブルごとのリポジトリ層を入れないこと（上の「バックエンドの構成」）は変わらない。何を取るかを書いた SQL がそのまま業務のルールで、
それをルールと一緒に置くという考えは、ORM の有無とも、Node か Go かとも関係なく同じだった。

## 境界の強制

物理的に守れない部分だけを ESLint で見る（`eslint.config.mjs`）。

- **shared は apps 配下を import できない。** これは同じ tsconfig の下にあるので型解決では
  防げず、ルールが要る。
- `apps/**` は独自の tsconfig と依存を持つ別パッケージなので、ルートの lint 対象から外している。

Go の API の中は、今は `package main` 1つなので、ファイル同士の境界を何も検査していない。
`internal/` のパッケージに分けると、非公開の名前と循環 import の禁止で、コンパイラが境界を守る。
書き込みの SQL を `internal/write/` の下にしか書かないことは、テストで確かめる（上の「バックエンドの構成」、JUK-148）。

## テスト

| 対象 | 実行 | 内容 |
|---|---|---|
| `src/shared` | ルートの vitest | 純粋関数、Zod スキーマ |
| `apps/api` | `go test ./...`、DB に流すものは `pnpm test:go-db` | API の門番・SQL・ログイン（`apps/api/README.md`） |
| `db/` | `pnpm --filter @juken-map/db test` | 接続の設定（時間帯・真偽値）、本番の DB ユーザーの権限 |
| `apps/web` | `pnpm --filter @juken-map/web test` | 画面まわりの純粋関数 |
| 通し | Playwright | 記録→可視化の毎日ループ、デモ閲覧専用、モバイルナビ |

Go の API の DB テストは、本番と同じ `registerRoutes` で組んだルーターに `httptest` でリクエストを送る
（`apps/api/dbtest_support_test.go` の `dbTestApp`）。ハンドラを直接呼ばないのは、
**本物のルーティングと入口の拒否・本文の読み取りを通すため**である。たとえば本文は
「壊れた JSON でも 400 にせず、入口の拒否（未ログイン・デモ）を先に効かせる」ように読んでおり
（`apps/api/body.go`）、ハンドラ直呼びではここが素通りしてしまう。Node の頃（JUK-121 まで）は
Fastify の `inject()` で同じことをしていた。

差し替えるのはセッションの取得（Cookie の値を利用者 ID として読む）と外部サービスという境界だけで、
門番のロジックは実物を動かす。

**SQL を流すテストは、DB も差し替えず本物の MySQL に流す。** ORM の呼び出しを
モックしても SQL の誤りは分からず、SQL 文字列を照合するテストは書き方を変えただけで壊れるため。
一意制約による 409 や同時アクセスの挙動も、本物の DB でしか確かめられない。

- テスト用 DB は開発用とは別の `juken_map_test`。vitest の globalSetup
  （`db/test-db/global-setup.ts`）が作成とマイグレーションを行う。Go の DB テストは同じ準備を
  `pnpm --filter @juken-map/db test-db:prepare` で行う。
  取り違え防止のため、DB 名が `_test` で終わらなければ何もせずに止まる。
- ローカルでは `pnpm db:start` で DB コンテナを起動しておく必要がある。CI は `check` ジョブに
  MySQL サービスを持つ。
- Go の DB テストは `dbtest` タグを付け、名前に `DB` を入れる（`TestSuspendDB` など）。CI と `pnpm test:go-db` は
  `go test -tags dbtest -p 1 -run DB ./...` で、どのパッケージのものも拾う（JUK-158）。
  `-p 1` でパッケージを1つずつ流す。DB 全体を数えるテスト（管理画面の概要）が、ほかのパッケージの
  テストが同時に作った行を数えてしまうため。
- テストごとに使い捨てのユーザーを作り、データはすべてそのユーザーにぶら下げる
  （`db/test-db/fixtures.ts`、Go は `apps/api/internal/dbtest`）。テーブルを空にする方式と違い、並列に走る他のテストと干渉しない。
- 往復（書いて読む）だけのテストでは時間帯の誤りが打ち消されて見えないので、
  `db/connection.test.ts` で DB 側の生の値と突き合わせている。CI（UTC）でもずれを検出できるよう、
  テストは `TZ=Asia/Tokyo` で動かす。
- モックするのは外部の境界（認証のセッション取得、LINE・メールなどの外部 API）だけ。

## デプロイ

`apps/web` のビルド成果物と Go の API（`apps/api`）を1つのイメージに入れ、EC2 上の Docker で動かす。
nginx（EC2 ホスト上）が 443 を受けて 3000 番へ流す。設定の実物は `infra/nginx/README.md`。

`src/shared` は `apps/*` の外にあるが、pnpm workspaceのルート依存からZodなどを解決できる。
Dockerイメージにもworkspaceと同じ階層でpnpmの依存をコピーするため、個別のsymlinkは不要である。
