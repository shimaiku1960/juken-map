# フロントエンド / バックエンド分離の移行計画

**方針決定: 2026-09-09（ユーザー判断）。** 受験マップ本体を、React + Vite の SPA と
Fastify の API サーバーに分離する。学習用の別リポジトリではなく、**本番アプリを分ける。**

## なぜ分けるか

Next.js はフロントとバックが同じプロセスに同居する設計である。2026-09-09 に `lib/` を
`src/frontend` / `src/backend` / `src/shared` へ再編し ESLint で境界を強制したが
（`docs/architecture.md`）、それは**フォルダと約束による境界**であって物理的な分離ではない。
`app/` が両方を import できるのは、両者が1つのプログラムにコンパイルされるためである。

プロセスを分けると、`page.tsx` からの `listGoals()` という関数呼び出しが HTTP になる。
シリアライズ、CORS、Cookie の伝搬、認証の受け渡しが「見える」ようになり、境界を守るのが
ESLint ではなくネットワークそのものになる。**バックエンドを根本から理解する**のが目的である。

## 到達点

**同一リポジトリのモノレポ**に組み替える。

```
juken-map/
├ apps/api    Fastify。src/backend と src/shared をそのまま持ち込み、
│             HTTP の入口 20本だけ Fastify 形式に書き換える
├ apps/web    React + Vite（SPA）。ここだけゼロから作る
└ （Next.js は apps/web が動いたら削除）
```

配信は **nginx で同一オリジンのままパス振り分け**（`/api/*` → Fastify、`/*` → 静的 SPA）。
別ドメインにすると認証 Cookie が `SameSite=None; Secure` 必須になるため、同一オリジンにする。

### なぜ別リポジトリにしないか（2026-09-09 判断）

当初「別リポジトリに同じものを作る」案を検討したが、実数で比べて**同一リポジトリに決めた**。

- **作業量は減らない。** SPA 化を選んだ以上、フロントエンド 6,218行の書き直しはどちらの案でも
  同じだけ発生する。置き場所を変えても1行も減らない
- **むしろ増える。** 別リポジトリだと Terraform 270行、GitHub Actions 302行、デプロイ
  スクリプト87行、Dockerfile 53行を作り直し、ドメイン・HTTPS証明書・ECR・SSM権限の
  付け替えも必要になる。**どれも学習目的（フロント/バック分離）と無関係**。git 履歴417
  コミットも切れる
- **別リポジトリ最大の利点が効かない。** 「本番を壊さずに作れる」ことが利点だが、
  **利用者は開発者本人のみ**なので守る対象が存在しない

## 移行対象の規模（2026-09-09 実測）

| 領域 | 規模 | 難易度 | 備考 |
|---|---|---|---|
| API Route | **20本 / 1,439行** | 低〜中 | ほぼ機械的に移植できる |
| Server Component からのサービス層直呼び | **10ファイル** | **高** | HTTP が無い経路。全部 fetch に置き換わる |
| 認証（better-auth） | `auth.ts` 64行＋`proxy.ts` 66行＋DB4テーブル | **最高** | 下記の検証済み事項を参照 |
| LINE OAuth / Webhook | callback・start・webhook | 中 | **LINE Developers 側の登録 URL 変更が必要** |
| cron 通知 | GitHub Actions → `/api/cron/...` | 低 | 向き先の変更のみ |
| microCMS（ブログ） | Server Component から直接取得 | 低 | 置き場所は Phase 2 で判断 |
| デプロイ | Docker 1コンテナ（:3000）＋SSM | 中 | 2コンテナ化とプロキシ設定 |

移植元となる `src/backend/`（19本）は TASK 1〜7 でサービス層・DTO・infra に切り出し済みで、
**ほぼそのまま Fastify へ持っていける。** 先の整理がここで効く。

## 認証は移行できる（Phase 0 で実証済み・2026-09-09）

better-auth 1.6.22 は `better-auth/node` に **`toNodeHandler`** を提供している。Node 標準の
`IncomingMessage` / `ServerResponse` を受けるハンドラなので、Fastify の `req.raw` / `res.raw` を
渡せば動く。したがって:

- DB テーブル（`user` / `session` / `account` / `verification`）はそのまま
- パスワードハッシュもそのまま
- **既存ユーザーは再登録不要**

Next.js 固有なのは `plugins: [nextCookies()]` の1行だけで、これは Server Actions 用なので
落とせる（このリポジトリに Server Actions は既に0件）。

**2026-09-09、Phase 0 で実際に動かして確認済み。** 詳細は下記 Phase 0 の節。

## 進め方

Phase 0（検証）は完了済み。以降は Step 1 → 2 → 3 の順に進める。

### ✅ Phase 0 — 認証の実現可能性検証（2026-09-09 完了・実証済み）

**結論: 移行できる。既存ユーザーの再登録もログアウトも発生しない。**

リポジトリ外の隔離ディレクトリに Fastify + better-auth のスパイクを作り（本番コードにも
リポジトリにも変更なし）、**本番と同じローカル MySQL の既存ユーザー**に対して検証した。
実装は `auth.ts` 28行 + `server.ts` 51行のみ。

| 確認項目 | 結果 |
|---|---|
| 既存ユーザーでログイン（既存のパスワードハッシュ） | ✅ HTTP 200 |
| セッション Cookie の発行（`better-auth.session_token`・HttpOnly） | ✅ |
| `auth.api.getSession` で認証済みユーザーを取得 | ✅ `nickname` などの additionalFields も復元 |
| セッションに紐づく Prisma クエリ | ✅ `studyLogCount: 22`（本番アプリでの実測と一致） |
| Cookie 無しは拒否 | ✅ |
| 誤パスワードは拒否 | ✅ HTTP 401 |
| メール未確認ユーザーは拒否（`requireEmailVerification`） | ✅ HTTP 401 |
| **Next.js が発行した Cookie を Fastify が受理** | ✅ **移行時にログアウトが起きない** |

最後の項目が決定的である。Next.js（:3000）でログインして得た Cookie を、そのまま
Fastify（:4000）へ送って認証が通った。**セッションテーブルも Cookie 形式も共通なので、
切り替え時にユーザーはログインしたままでいられる。**

#### 検証中に踏んだ2点（Phase 1 でそのまま効く）

1. **Fastify のボディ解析が better-auth より先に走ると 400 になる。** better-auth は
   Node のリクエストストリームを自分で読むが、Fastify は既定で `application/json` を
   読み切ってしまう。`addContentTypeParser` を `parseAs: "string"` で挟んでも同じで、
   ストリームが枯れる。**解決は `onRequest` フック**（ボディ解析より前に走る）で
   `reply.hijack()` してから `toNodeHandler` に渡すこと。

2. **`BETTER_AUTH_SECRET` が一致しないと、Cookie の署名検証に失敗して静かに未認証になる。**
   エラーではなく「ログインしていない」として扱われるので原因が見えにくい。
   Cookie は `トークン.署名` の形式で、署名はこの secret で作られる。**移行時は新旧で
   同じ secret を使うこと**（別の値にすると全ユーザーが強制ログアウトになる）。

#### Next.js 側との設定差

本番の `src/backend/infra/auth.ts` から落としたのは `plugins: [nextCookies()]` の1行だけ
（Server Actions 用で、このリポジトリに Server Actions は0件）。DB アダプタ・
`additionalFields`・`emailAndPassword`・`trustedOrigins` はそのまま使えた。

### Step 1 — `apps/api`（Fastify）を作る

`src/backend`（19本・809行）と `src/shared`（12本）を **そのままコピー**し、HTTP の入口
20本（1,439行）だけを Fastify 形式に書き換える。

**なぜコピーで済むのか:** サービス層・infra・DTO は Next.js に依存していない。例えば
`services/study-log-service.ts` の import は Prisma と自作の `measured` だけである。
TASK 3〜5B で「UI 層から Prisma を直接呼ばない」「サービス層へ切り出す」をやった結果、
ビジネスロジックが Next.js から切り離されていた。当時は設計上の理由だったが、
**結果的に移行の下準備になっていた。**

書き換えが必要なのは Route Handler にある Next.js 固有の3点だけ。

| Next.js | Fastify |
|---|---|
| `export async function GET()` | `app.get("/api/...", handler)` |
| `await headers()` | `fromNodeHeaders(request.headers)` |
| `NextResponse.json(x)` / `{ status: 401 }` | `return x` / `reply.code(401).send(...)` |

認証のマウントは Phase 0 で確立した方法（`onRequest` フックで `reply.hijack()` してから
`toNodeHandler`）を使う。

完了条件: 20本すべてが Fastify で動き、curl で既存ユーザーの認証込みの応答が得られる。

### Step 2 — `apps/web`（React + Vite）を作る

ここだけゼロから。**作業量の大半（コンポーネント35本・6,218行、画面18本、hooks 5本）はここ。**

- `src/frontend/components/ui`（shadcn 10本）はそのままコピーできる
- Server Component がサービス層を関数で直接呼んでいた**10ファイルは、すべて fetch になる**
- 認証状態は better-auth のクライアントで扱う
- `src/shared` は `apps/api` と共用する

### Step 3 — 切り替えて Next.js を消す

nginx のルーティングを `apps/web` に向け、Next.js を削除する。**利用者が開発者本人のみなので
段階移行はしない。** パリティに達したら一気に切り替える。問題があれば nginx の設定を戻す。

その後、cron（GitHub Actions）の向き先と **LINE Developers のコールバック URL** を更新する。

### 段階移行をしない理由（2026-09-09 判断）

当初は Phase 1 で「API だけ先に本番へ出し、画面は無変更のまま検証する」段階を置いていた。
これは**稼働中のサービスを守るための工程**である。

**利用者が開発者本人のみであることが確認されたため、この工程は削除した。** 守る対象が
存在しないのに段階を踏むのは、単に遅くなるだけである。

本番であることから来る懸念も、大半は既に解消している。

| 懸念 | 状態 |
|---|---|
| 学習記録が消える | **同じ DB を見る**のでデータ移行そのものが発生しない |
| 全員が強制ログアウトされる | Phase 0 で Next.js の Cookie を Fastify が受理すると実証済み |
| 切り替えに失敗して戻せない | nginx の向き先を戻すだけ。旧アプリは残しておける |

残る実質的な制約は SEO（後述）と、LINE のコールバック URL 更新、cron の向き先だけである。

## 未決事項（判断を後回しにできるもの）

### SEO をどうするか（Step 2 で判断）

**SPA 化すると SSR が無くなり、現在の SEO は失われる。** LP は新規登録の入口で、
インデックス済み・`sitemap.xml` 送信済み・Search Console 登録済みである
（`lp-and-mockup.md`）。これは実害として認識したうえで SPA を選択している。

緩和策: LP・ブログ・規約・プライバシーポリシーだけを**事前生成した静的 HTML** として
配信すれば、検索流入は維持できる。ログイン後のアプリ部分は SPA で問題ない。
Step 2 で具体化する。

### その他

- microCMS ブログの置き場所
- 学習用リポジトリ `juken-map-fullstack-lab` の位置づけ。本番移行と目的が重複するため、
  役目を終えたと考えてよい。Phase 1 相当（Fastify + メモリ保存の CRUD、PATCH/DELETE 込み）
  まで実装済みだが未コミットで残っている

## 要確認（リポジトリ管理外）

**HTTPS を終端しているリバースプロキシの設定が、リポジトリに存在しない。** 現在の
デプロイは `docker run -p 3000:3000` のみで、その前段（nginx 等）は EC2 ホスト上に
手で置かれていると思われる。Step 3 で切り替える前に、SSM で接続して実機の設定を確認し、可能なら
リポジトリ管理下へ持ってくる。

## 進め方の制約（TASK 1〜7 から引き継ぐ）

- 各 Step の完了時に動作を確認する。SPA になると表示 DOM の前後比較は使えなくなるため
  （HTML の生成方法自体が変わる）、API 応答の比較と実際の画面確認に切り替える
- 判断に迷う設計上の選択が出たら、選択肢とトレードオフを提示して止まる
- push・PR 作成・main へのマージは勝手にしない
