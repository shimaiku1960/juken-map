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

```
                       ┌─ /api/*  → Fastify（:4000）
インターネット → nginx ─┤
                       └─ /*      → 静的 SPA（React + Vite）
```

**同一オリジンで配信する。** 別ドメインにすると認証 Cookie が `SameSite=None; Secure` 必須に
なり、ブラウザ側の制約が一気に増える。パスで振り分ければ Cookie は現状のまま動く。

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

## 段階

各段階は独立してデプロイでき、途中で止めても壊れない。一発切り替えはしない。

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

### Phase 1 — API を Fastify へ（画面は無変更）

20本の Route Handler を Fastify へ移植する。`src/backend/` の services / infra / dto は流用する。

- nginx で `/api/*` だけ Fastify に向ける
- **Next.js の画面は一切変更しない**（サービス層の直呼びもそのまま）
- 本番で「API が別プロセスで動いている」状態を安全に検証できる区切り
- 完了条件: 既存の画面が今までどおり動き、`/api/*` が Fastify から応答している

### Phase 2 — SPA を作る（本番は無変更のまま並行作業）

React + Vite で画面を移植する。**Server Component の直呼び10ファイルはすべて fetch になる。**
作業量が最も多い段階。

- 認証状態は better-auth のクライアントで扱う
- ここで LP・ブログ・規約の扱いを判断する（下記「未決事項」）
- 完了条件: SPA でログインから主要導線までひととおり動く

### Phase 3 — 切り替え

nginx の `/*` を SPA に向け、Next.js を停止する。問題があれば nginx の設定を戻すだけで
復旧できる形にしておく。

### Phase 4 — 後始末

- cron（GitHub Actions）の向き先変更
- **LINE Developers のコールバック URL 更新**
- デプロイ構成の2コンテナ化、`Dockerfile` の分割

## 未決事項（判断を後回しにできるもの）

### SEO をどうするか（Phase 2 で判断）

**SPA 化すると SSR が無くなり、現在の SEO は失われる。** LP は新規登録の入口で、
インデックス済み・`sitemap.xml` 送信済み・Search Console 登録済みである
（`lp-and-mockup.md`）。これは実害として認識したうえで SPA を選択している。

緩和策: LP・ブログ・規約・プライバシーポリシーだけを**事前生成した静的 HTML** として
配信すれば、検索流入は維持できる。ログイン後のアプリ部分は SPA で問題ない。
Phase 2 で具体化する。

### その他

- モノレポ（`apps/web`, `apps/api`）にするか、リポジトリを分けるか
- microCMS ブログの置き場所
- 学習用リポジトリ `juken-map-fullstack-lab` の位置づけ（本番移行と重複するため、
  Phase 0〜1 の練習台として使うか、役目を終えたとするか）

## 要確認（リポジトリ管理外）

**HTTPS を終端しているリバースプロキシの設定が、リポジトリに存在しない。** 現在の
デプロイは `docker run -p 3000:3000` のみで、その前段（nginx 等）は EC2 ホスト上に
手で置かれていると思われる。Phase 1 でパス振り分けを追加する前に、SSM で接続して
実機の設定を確認し、可能ならリポジトリ管理下へ持ってくる。

## 進め方の制約（TASK 1〜7 から引き継ぐ）

- 1段階ずつ進め、各段階で表示 DOM・API 応答の前後比較を行う（手順は
  `docs/architecture.md` の「検証方法」）
- 判断に迷う設計上の選択が出たら、選択肢とトレードオフを提示して止まる
- push・PR 作成・main へのマージは勝手にしない
