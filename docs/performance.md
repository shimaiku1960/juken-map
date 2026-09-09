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

---

## TASK 2: 空振りクエリの調査（2026-09-09・調査のみ、コード変更なし）

`/dashboard` の 10 本のうち、結果が 0 件と分かりきっているクエリが出ている件の原因調査。
**この TASK ではアプリコードを変更していない。**

### 事実: `/dashboard`（e2e ユーザー）の 10 本の内訳と発行元

`.next/dev/logs/next-development.log` の curl 直後クラスタを実測。発行順は Promise.all で
並行実行されるため前後する。

| # | SQL | 発行元 | 0件確定 |
|---:|---|---|:--:|
| 1 | `SELECT ... FROM session WHERE token = ?` | better-auth（`lib/auth-session.ts`） | |
| 2 | `SELECT ... FROM user WHERE id = ?` | better-auth（同上） | |
| 3 | `SELECT ... FROM StudyPlan WHERE userId = ?` | `lib/services/study-plan-service.ts:6` | |
| 4 | `SELECT ... FROM FinalGoal WHERE userId = ?` | `app/dashboard/page.tsx:29` | |
| 5 | `SELECT ... FROM StudyLog WHERE userId = ?` | `lib/services/study-log-service.ts:6` | |
| 6 | `SELECT ... FROM Textbook WHERE id IN (NULL)` | `study-plan-service.ts:9` の `include: { textbook: true }` | ✅ |
| 7 | `SELECT ... FROM Faculty WHERE id IN (NULL)` | `app/dashboard/page.tsx:31` の `include: { faculty: ... }` | ✅ |
| 8 | `SELECT ... FROM StudyLog WHERE studyPlanId IN (NULL)` | `study-plan-service.ts:9` の `include: { studyLog: { select: { id: true } } }` | ✅ |
| 9 | `SELECT ... FROM Textbook WHERE id IN (?,?,… 22個)` （パラメータは**全て null**） | `study-log-service.ts:9` の `include: { textbook: true }` | ✅ |
| 10 | `SELECT ... FROM University WHERE id IN (NULL)` | `app/dashboard/page.tsx:31` の `include: { university: true }` | ✅ |

**0 件確定は 4 本ではなく 5 本**（10 本中の半分）。引き継ぎメモが 3〜4 本としていたのは、
#9 が `IN (NULL)` ではなく `IN (?,?,...)` の形をしていて見落とされたため。#9 も
パラメータが全て null なので `id IN (NULL, NULL, ...)` と同じで、SQL 上は決して真にならず
必ず 0 件になる。

### 事実: 原因は 1 つではなく 2 つ

各サービス関数を単体で叩き、SQL とバインドパラメータを直接ログした結果（複数ユーザーで実測）:

**原因A: 親が 0 件でも Prisma は子のリレーションクエリを発行する**

親 findMany が 0 行を返しても、Prisma は `include` の子クエリを省略せず、
`IN (NULL)`（パラメータ 0 個のリテラル）という「絶対に一致しない」形で 1 往復を消費する。
`/dashboard` の #6 #7 #8 #10 がこれ。#10 は親 FinalGoal が 0 件 → Faculty が 0 件 →
University も 0 件、という連鎖で、深さのぶん往復が積み上がる。

**原因B: FK が NULL の行のぶんまで IN 句に入る（しかも重複も除かれない）**

親に行があっても、FK が NULL の行は除外されずに `IN` のパラメータへ `null` として送られる。
実測例:

```
listStudyLogs / e2e@juken-map.com   → Textbook WHERE id IN (?×22)   params=[null ×22]
listStudyLogs / dikurou@gmail.com   → Textbook WHERE id IN (?×6)    params=[null,null,null,null,null,3]
listStudyPlans / dikurou@gmail.com  → Textbook WHERE id IN (?×8)    params=[1,1,1,2,3,1,null,1]
```

`IN` のプレースホルダ数は「取得すべき別テーブルの ID の種類数」ではなく
**親の行数そのもの**。null も重複もそのまま並ぶ。

### 事実: 実データでも起きる（テストデータ固有ではない）

`prisma/schema.prisma` の NULL 許容:

| カラム | 型 | 判定 |
|---|---|---|
| `FinalGoal.facultyId` | `Int`（NOT NULL） | 原因B は起きない。原因A（志望校0件）のみ |
| `Faculty.universityId` | `Int`（NOT NULL） | 同上 |
| `StudyPlan.textbookId` | `Int?` | 原因A・B とも起きる |
| `StudyLog.textbookId` | `Int?` | 原因A・B とも起きる |
| `StudyLog.studyPlanId` | `Int?`（`@unique`） | 原因A・B とも起きる |

開発DBの実ユーザーで再現を確認済み:

- `waseda.no.takahashi@gmail.com`（志望校0・予定0・記録0＝新規登録直後と同じ状態）
  → `IN (NULL)` が listGoals で 3 本、listStudyPlans で 2 本、listStudyLogs で 1 本
- `dikurou@gmail.com`（実使用中のアカウント。志望校4・予定8・記録6）
  → 原因A は出ないが、原因B により `params=[null,null,null,null,null,3]` のような
    無駄を含む IN が出る

したがって **「新規登録直後のユーザー」は原因A に、「参考書を紐付けずに記録している
ユーザー」は原因B に該当し、どちらも本番の実データで日常的に発生している。**
参考書の紐付けは任意入力なので、原因B は例外ではなく通常ケース。

### 事実: 1 本あたりのコスト（ローカル Docker MySQL・200回の中央値）

| クエリ | median | p95 |
|---|---:|---:|
| `SELECT 1`（往復の下限） | 0.600ms | 1.004ms |
| `Textbook WHERE id IN (NULL)` | 0.620ms | 1.027ms |
| `Textbook WHERE id IN (NULL ×22)` | 0.527ms | 0.885ms |

空振りクエリのコストは**ほぼ純粋なネットワーク往復**で、SQL 自体の実行はゼロに近い。
localhost の Docker では 5 本で約 3ms。

**推測**: 本番は EC2 → RDS の実ネットワーク越しなので 1 往復あたりのコストはこれより
大きく、空振り 5 本の影響も localhost より大きい。ただし本番で計測はしていないため
数値は未確認。

### 事実: 10万件時の IN 句肥大化リスク（実測）

一時ユーザーに `StudyLog` を大量投入し（`textbookId` は全て NULL）、`listStudyLogs` 相当を実行:

| 親の行数 | 発行クエリ数 | Textbook の IN プレースホルダ数 | 所要 |
|---:|---:|---|---:|
| 1,000 | 2 | 1,000 | 36ms |
| 10,000 | 2 | 10,000 | 127ms |
| 65,000 | 2 | 65,000 | 839ms |
| 70,000 | **3** | 65,535 + 4,465 に分割 | 917ms |
| 100,000 | **3** | 65,535 + 34,465 に分割 | 1,236ms |

**評価**: Prisma は 65,535 個（MySQL のプリペアドステートメントのプレースホルダ上限）で
自動的にチャンク分割するため、**件数が増えてもエラーにはならない**。ただし

- IN 句のサイズが親の行数に比例して線形に増える（NULL も重複も含めて送られる）
- 65,535 を超えると空振りクエリの本数まで増える（100,000 行なら 0 件確定のクエリが 2 本）

`listStudyLogs` / `listStudyPlans` には `take` が無く、そのユーザーの全行を取得する実装
なので、行数はユーザーの利用期間に比例して増え続ける。

**推測**: 1日5件の学習記録でも 65,535 行に達するには約 36 年かかるため、
現実のユーザー1人でチャンク分割まで到達する可能性は低い。ただし数千〜1万行規模
（数年利用）は十分あり得て、その時点で「全部 null の IN 句を 1 万個」送ることになる。
これはクエリ本数ではなくクエリ**サイズ**の問題として効いてくる。

### 回避策の選択肢（実装はしていない。TASK 2 の範囲外）

**選択肢1: 何もしない**
- コスト: 空振り 5 本 ≒ ローカルで 3ms。ページ全体（約 90ms）に対する比率は小さい
- 利点: リスクゼロ
- 欠点: 原因B のクエリサイズ肥大は将来じわじわ効く

**選択肢2: `relationLoadStrategy: "join"` を使う（検証済み・推奨候補）**
- `previewFeatures = ["relationJoins"]` を generator に追加し、対象の findMany に
  `relationLoadStrategy: "join"` を指定する。Prisma が相関サブクエリ＋`JSON_OBJECT` で
  1 本にまとめるため、**子クエリが原理的に消える**
- 本体を汚さずに検証済み（一時クライアントを `node_modules/.joinprobe` に生成して実行、
  検証後に削除）。MySQL 8.4 + `@prisma/adapter-mariadb` で動作した:

  | 対象 | query 戦略 | join 戦略 | 結果の一致 |
  |---|---:|---:|---|
  | listGoals（志望校0件） | 4本 / 3.39ms | **1本 / 0.89ms** | 一致 |
  | listStudyPlans | 3本 / 1.53ms | **1本 / 0.75ms** | 一致 |
  | listStudyLogs（22件） | 2本 / 1.95ms | **1本 / 1.18ms** | 一致 |
  | listStudyLogs（128件） | 2本 / 3.84ms | **1本 / 2.42ms** | 一致 |

  3ユーザー × 3クエリの全パターンで、返り値の JSON が完全一致することを確認済み
- 欠点: **preview 機能**である（GA ではない）。preview を1つ有効化すると、その機能の
  仕様変更を将来のアップグレードで踏むリスクを負う。またクエリが 1 本になるぶん
  ログからは「どのリレーションが重いか」が見えにくくなる

**選択肢3: `include` をやめて必要な列だけ手で引く**
- 例えば `listStudyLogs` の `textbook` は、UI で使うのは 7 列だけ。textbookId が非 NULL の
  ものだけを集めて `distinct` してから 1 本引き、アプリ側で結合する
- 利点: preview 機能に依存しない。原因A・B とも完全に消える
- 欠点: 手で結合するコードが増え、Prisma の型推論の恩恵が減る。サービス層が肥大化する

**選択肢4: 原因A だけをアプリ側で潰す（親が 0 件なら子を引かない）**
- `include` を条件分岐すると Prisma の戻り値の型が分岐して扱いにくい。効果も原因A のみ
- 費用対効果が悪く、非推奨

**判断が必要な論点**: 選択肢2 は効果が最大（10本 → 6本、しかも将来のサイズ肥大も解消）
だが preview 機能への依存が増える。選択肢1 と 2 のどちらを取るかはユーザー判断とする。

### 検証方法（再現手順）

一時スクリプトで、`log: [{ emit: "event", level: "query" }]` を有効にした Prisma クライアントから
サービス層と同じクエリを直接実行し、SQL とバインドパラメータを出力した。
規模テスト用の一時ユーザーと 10 万行、および検証用の一時 Prisma クライアントは
すべて削除済み（作業ツリーへの変更なし）。

### 判断: 選択肢1「何もしない」を採用（2026-09-09・ユーザー判断）

回避策は **選択肢1（何もしない）** に決定した。**この TASK でのアプリコード変更はなし。**

- 理由: 空振り 5 本のコストはローカル実測で約 3ms（ページ全体 約90ms の数%）。一方
  選択肢2 は preview 機能 `relationJoins` への依存を新たに負う。現時点の実測値では
  そのリスクに見合わないと判断した。
- 選択肢2 の検証結果（10本→6本、速度約半分、返り値 JSON 完全一致）は上記に記録済みなので、
  実データで実際に問題が出た時点で再検討できる。再検証はやり直さなくてよい。
- 再検討のトリガー: 原因B の `IN` 句サイズが実データで肥大した場合（学習記録が数千件規模の
  ユーザーが出た場合）、または `/dashboard` の応答時間が実測で問題になった場合。

