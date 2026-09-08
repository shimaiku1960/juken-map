# パフォーマンス計測記録

計測にもとづく構造改善の記録。数字は全て開発環境（`next dev` / Docker MySQL 8.4 / localhost）での実測値。

## 計測の標準手順

本番環境ではなく開発サーバーで、SQL の発行本数を数える。Prisma のクエリログは
`lib/prisma.ts` で開発環境のみ有効化してある。

1. curl で better-auth の `/api/auth/sign-in/email` にログインし Cookie を取得する
2. コンパイルのため対象ページを1回踏む（この回は計測に含めない）
3. curl で対象ページを1回 GET し、`.next/dev/logs/next-development.log` に出た
   `prisma:query` の本数とレスポンス時間を記録する
4. 同じ手順を2回実施して再現性を確認する

> **注意**: ブラウザで同じアプリを開いたままにしていると、その裏側の fetch が同じログに
> 混ざる。ログ行はタイムスタンプの空白（300ms 以上）でクラスタに区切り、curl 直後の
> 最初のクラスタだけを数えること。これを怠ると 10 本のはずが 19 本に見える。

計測はクライアント JS を実行しない curl で行う。ブラウザで開くと、ページ描画の後に
`/api/study-logs` などへの fetch が続けて走り、1リクエスト分の数字が取れないため。

---

## TASK 1: getSession の重複解消（2026-09-09）

### 問題

`/dashboard` を1回表示するだけで、`auth.api.getSession()` が**同一リクエスト内で2回**
呼ばれていた。呼び出し元は `app/layout.tsx` が描画する `app/components/Header.tsx` と、
各 `page.tsx`。

better-auth の Prisma アダプタは session と user を JOIN せず別クエリに分割するため
（`@better-auth/prisma-adapter` の `findOne` が `select: { user: true }` を使い、
Prisma 7 / MySQL がこれを2本の SQL に分ける）、`getSession()` 1回につき SQL が2本出る。
`session.cookieCache` は未設定なので毎回DBに当たる。

同一リクエストであることは、React の `cache()` で発行したリクエストIDを一時ログに
付けて確認した（同じ `req=` が Header と page の両方から出た）。

### 対策

`lib/auth-session.ts` を新設し、`getSession()` を React の `cache()` で包んだ。
`cache()` は同一リクエストのレンダリング中だけ結果を使い回すので、Header と page.tsx が
それぞれ呼んでもDBアクセスは1回で済む。引数を取らない関数にして `headers()` を内側に
閉じ込めているのは、メモ化のキーを安定させるため。

Next.js 16 の公式ガイド（`node_modules/next/dist/docs/01-app/02-guides/authentication.md`）も、
DAL の `verifySession()` を React `cache()` で包む形を推奨しており、それに沿っている。

置き換えたのは Server Component のみ（`Header.tsx` と7ページ）。API Route は
Server Component と同じレンダリングパスに乗らず `cache()` が効かないため対象外。

**`session.cookieCache` は有効化していない。** セッションを無効化しても TTL の間は
有効なままという副作用があるため、別途判断する。

### 結果（`/dashboard` を1回 GET）

| 項目 | 修正前 | 修正後 | 差分 |
|---|---:|---:|---:|
| `getSession()` の実行回数 | 2 | **1** | -1 |
| `SELECT ... FROM session` | 2 | **1** | -1 |
| `SELECT ... FROM user` | 2 | **1** | -1 |
| 認証関連の SQL 合計 | 4 | **2** | **-50%** |
| ページ全体の `prisma:query` | 12 | **10** | -17% |
| レスポンス時間 | 0.134s / 0.115s | 0.098s / 0.091s | 参考値 |

レスポンス時間は開発サーバーでの値なので参考程度。意味があるのは SQL 本数のほう。

対象は7ページ（`/`, `/dashboard`, `/goals`, `/profile`, `/explore`,
`/explore/[universityId]`, `/line/link`）。Header は全ページ共通なので、
ログイン中の全ページ遷移でこの2本が減る。

### 確認したこと

- 対象8ファイル（Header + 7ページ）すべて HTTP 200
- 未ログインで `/dashboard` は `/login` へ 307（認可の挙動は変わっていない）
- `npx tsc --noEmit` / `npx eslint --max-warnings 0` / `npm test`（240件）すべて通過
- 計測用の一時コード（`lib/__debug-session-count.ts` と各ファイルのログ呼び出し）は削除済み
