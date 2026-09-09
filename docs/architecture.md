# アーキテクチャ

受験マップのコードは、**責務**で `src/frontend` / `src/backend` / `src/shared` の3層に分かれる。
判断基準は「どこで実行されるか」ではなく「何に責任を持つか」である。

Server Component（`page.tsx`）はサーバーで実行されるが、責務は UI の組み立てなので
**frontend 側の呼び出し元**として扱う。バックエンドのサービス層を呼び、受け取った
データを画面部品へ渡すだけの入口である。

## ディレクトリ

```
app/                    Next.js のルーティング定義（入口だけの薄い層）
├ page.tsx / layout.tsx    UI の組み立てのみ
├ api/**/route.ts          HTTP の入口（認証・バリデーション・ステータスコード）のみ
├ robots.ts / sitemap.ts / manifest.ts / opengraph-image.tsx
└ providers.tsx / globals.css

src/frontend/           UI の組み立て・表示・ユーザー操作
├ components/             画面部品（ui/ は shadcn/ui）
├ hooks/                  TanStack Query のサーバー状態フック
└ lib/                    utils(cn), analytics, auth-client, browser, demo-client,
                          prefectures, studySession, studyLog, studyPlan,
                          examSchedule, reverseStudyNavigation

src/backend/            データアクセス・外部連携・ドメインロジック
├ infra/                  prisma, auth, auth-session, email, resend, microcms, line, lineLogin
├ services/               ユースケース（goal / study-log / study-plan / textbook /
│                         university / notification / sendDailyNotifications）
├ dto/study-mapper.ts     Prisma の戻り値 → 共有 DTO への変換
├ domain/dailyNotification.ts  通知本文の組み立て
├ observability/measured.ts    サービスの所要時間計測
└ demo-guard.ts           デモアカウントの編集を 403 にする門番

src/shared/             外部依存のない純粋関数・型・定数データのみ
├ date.ts, subjects.ts, studyStats.ts, site.ts, demo.ts
├ validations/            Zod スキーマ（リクエストの契約）
└ dto/study.ts            フロント↔バックで受け渡す形の型定義
```

`@/frontend/*`・`@/backend/*`・`@/shared/*` は `tsconfig.json` の `paths` で
`src/` 配下へ解決する。`@/*` は従来どおりリポジトリルートを指す。

## 依存の向き

```
app/  ──→  frontend  ──→  shared
  └─────→  backend   ──→  shared
```

- **shared は何にも依存しない。** DB・HTTP・ブラウザ API・React に依存するものは置かない。
  ここが崩れると、フロントとバックの両方が同じものを別々に持つ状態に戻る。
- **frontend と backend は互いに依存しない。** 両方が必要とするものは shared へ置く。
- `app/` は両方を呼ぶ入口。ロジックは持たせない。

## 配置の判断根拠

### なぜ実行場所ではなく責務で分けたか

Next.js の Server Component はサーバーで動くが、書いているのは JSX の組み立てである。
実行場所で分けると `page.tsx` が backend になり、「バックエンド＝データを扱う場所」という
本来の区別が失われる。責務で分ければ、`page.tsx` は frontend の入口、`route.ts` は
backend の入口、と一貫して説明できる。

### shared に入れたもの（両方が実際に使っているもの）

参照元を機械的に数えて決めた（`"use client"` はコメントを除いたディレクティブで判定）。

| ファイル | frontend からの参照 | backend からの参照 |
|---|---:|---|
| `date.ts` | 7 | studyStats・dailyNotification・prisma/seed 3本 |
| `subjects.ts` | 9 | validations 3件＋studyLog・studyStats |
| `studyStats.ts` | 2 | Server Component 1・lib 1 |
| `validations/` | 各1〜8 | API Route |
| `dto/study.ts`（型） | hooks が re-export | study-mapper・API・page |

### 個別に判断したもの

**`subjects.ts` は分割していない。** 色（UI）と `SUBJECT_VALUES`（バリデーション）が
同居しているが、色は hex 文字列＝定数データで React にもブラウザにも依存しない。
分割すると同じ科目リストが2箇所に生まれ、片方を直し忘れる状態になる。これは
DTO 変換の重複（`docs/performance.md` の TASK 4）で一度潰した問題なので繰り返さない。

**`site.ts` は shared に置いた。** 参照元はサーバー側のみ（21件）だが、責務は
データアクセスでも外部連携でもドメインロジックでもなく、純粋な定数（`SITE_URL`・
`NOINDEX`・LINE 公式アカウント URL）である。`app/robots.ts`・`sitemap.ts`・各 `layout` の
metadata から使われる。

**`dto/study.ts` は型と変換関数を分けた。** 型はフロントとバックの契約なので shared、
変換関数は Prisma の型に依存するので `backend/dto/study-mapper.ts`。shared に外部依存を
持ち込まないための分割である。

**`validations/` は6本すべて shared。** `goal.ts` だけは現状クライアントからの参照が
0件だが、Zod スキーマはリクエストの契約という同じ性質のものであり、フォームが後から
付く可能性もある。1本だけ別の層に置く方が説明しづらいので、まとまりを優先した。

**`studyStats.ts` は shared。** ヒートマップと連続日数の集計は表示のための計算だが、
外部依存のない純粋関数で、画面とサーバー側の両方から実際に使われている。frontend に
置くと backend → frontend という逆向きの依存が残る。

**`demo` は3つに分かれる。** `DEMO_EMAIL`（shared/demo.ts）・`demoReadOnlyGuard`
（backend/demo-guard.ts、`next/server` 依存）・`notifyDemoReadOnly`
（frontend/lib/demo-client.ts、toast）。使うファイルは重複していない。

**`sendDailyNotifications.ts` は backend/services。** 通知バッチだが、Prisma・Resend・
LINE・純粋ビルダーを束ねて1つの Route Handler（cron）から呼ばれる形は他のサービスと
同じである。本文を組み立てる純粋な `dailyNotification.ts` は `backend/domain/` に分けた。

### `app/components/` と `app/hooks/` を移した理由

「`app/` は動かさない」という制約の理由はルーティング定義の保護である。`components/`
（68本）と `hooks/`（5本）はルーティングではなく UI そのものなので対象外と判断し、
`src/frontend/` へ移した。残していれば UI の本体が `app/` に残り、ディレクトリで
フロント側を示すという目的が達成できない。root の `components/`（shadcn/ui）も同様に
`src/frontend/components/ui/` へ移し、`components.json` の `aliases` を更新した。

## 検証方法

構成変更で挙動が変わっていないことは、目視ではなく前後比較で確認する。

1. dev server を起動したまま変更後の HTML・API 応答・生成 CSS を取得
2. `git stash` で変更前に戻して同じ server から同じものを取得し、`git stash pop`
3. HTML は `<script>`（RSC ペイロード）と `<style>` を除いた**表示 DOM** で比較。
   RSC ペイロードには `self.__next_r`（リクエスト毎のランダム ID）と flight payload の
   内部参照番号のずれが必ず出るため、そのままでは比較にならない
4. API 応答は JSON をパースして比較（キー順ではなく値の一致を見る）
5. **生成 CSS も比較する。** Tailwind v4 は自動でファイルを走査するので、ディレクトリを
   動かすと class が検出されなくなる可能性がある。これは HTML 差分では絶対に検出できない
   （class 名は HTML に残り、CSS だけが欠ける）
6. データが0件だと描画経路を通らないので、e2e ユーザー（学習記録あり・志望校0件）と
   demo ユーザー（志望校4件）の両方で確認する

## 境界の強制（ESLint）

上記の依存の向きは、レビューでの気づきに頼らず `eslint.config.mjs` の
`no-restricted-imports` で機械的に禁止している。CI（`.github/workflows/ci.yml`）でも
`npx eslint --max-warnings 0` が走るので、違反したままマージされることはない。

| 対象 | 禁止する import | 意図 |
|---|---|---|
| `src/shared/**` | `@/frontend/*`・`@/backend/*`・`@/app/*` | shared は最下層。何にも依存しない |
| `src/frontend/**` | `@/backend/*`、Prisma 生成クライアント | UI から DB・外部連携へ直接触らせない |
| `src/backend/**` | `@/frontend/*` | サーバー処理が画面都合に引きずられないようにする |
| `app/**/page.tsx`・`layout.tsx`・`template.tsx` | Prisma 生成クライアント、`@/backend/infra/prisma` | 画面の入口は必ずサービス層を経由する |

エラーメッセージは日本語で、禁止だけでなく**代わりに何をすべきか**を書いている
（例:「データ取得は入口で行い、結果を props で渡してください」）。

`app/api/**/route.ts` は対象外。Prisma のエラー型（`Prisma.PrismaClientKnownRequestError`
を使った `P2002 → 409` の変換など）を扱うため、生成クライアントを参照する必要がある。
`app/generated/**` は Prisma の生成物なので lint 対象から除外している。

### 導入時に直した違反（1件）

`src/frontend/components/Header.tsx` がセッションを自分で取得していた
（`@/backend/infra/auth-session` を直接 import）。取得を入口の `app/layout.tsx` へ移し、
Header は `user` を props で受け取る表示専用コンポーネントにした。Header は認証
ライブラリの型ではなく構造的な型（`HeaderUser`）で受けるので、backend への依存は残らない。

これで例外指定なしに境界が成立している。
