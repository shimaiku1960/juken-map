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

---

## TASK 3: UI層からの Prisma 直接呼び出しをサービス層へ移す（2026-09-09）

### やったこと

`app/**/*.tsx`（Server Components）からの `prisma.` 直接呼び出し 7 箇所 5 ファイルを、
すべて `lib/services/` 経由に置き換えた。追加したサービス関数は 6 つで、全て `measured()`
でラップしている。

| 移動元 | 追加した関数 | 置き場所 |
|---|---|---|
| `app/page.tsx` | `findFirstChoiceGoal(userId)` | `goal-service.ts` |
| `app/goals/page.tsx` | `listTextbookSubjects(userId)` | `textbook-service.ts` |
| `app/explore/[universityId]/page.tsx` | `findUniversityDetail(id)` | `university-service.ts` |
| `app/explore/[universityId]/page.tsx` | `listGoalFacultyIds(userId)` | `goal-service.ts` |
| `app/dashboard/page.tsx` | `listGoalsWithFaculty(userId)` | `goal-service.ts` |
| `app/profile/page.tsx` | `findNotificationPreference(userId)` | `notification-service.ts`（新設） |
| `app/profile/page.tsx` | `findLineConnection(userId)` | `notification-service.ts`（新設） |

あわせて、手順書が「1文字違わず同一」と指摘していた重複を解消した。
`app/api/notification-preferences/route.ts` の GET と PUT も、上の
`findNotificationPreference` ／ `findLineConnection` を使うように変更している。
同ルートの `upsert`（書き込み）は TASK 3 の範囲外なので `prisma` 直呼びのまま残した。

### 判断: dashboard は `listGoals` を使わず専用関数を新設した（ユーザー判断）

手順書は「`app/dashboard/page.tsx` の `finalGoal.findMany` は既存の `listGoals` に
置き換えること」としていたが、**そのままでは完了条件「クエリ本数が増えていないこと」を
満たせない**ため、選択肢を提示してユーザー判断を仰いだ。

- `listGoals` の `include` は `faculty → university, tags`。`Faculty.tags` は
  `Tag[]` ↔ `Faculty[]` の**暗黙的多対多**なので、Prisma が中間テーブル経由で
  **追加 1 本**を発行する（TASK 2 の実測で listGoals = 4 本、dashboard の直呼び = 3 本）。
- dashboard の画面に tags は出てこない。純粋な over-fetch ＋ SQL 1 本増になる。
- 結論: tags を含まない `listGoalsWithFaculty(userId)` を新設した。tags を実際に使うのは
  志望校ページだけなので、`listGoals`（tags あり）はそちらの専用として残す。

### 計測結果（e2e ユーザー・2回実施して再現性を確認）

計測前にログが 4 秒間伸びなくなるまで待ってクラスタ混線を防いだ（前任者が踏んだ罠の対策）。

| ページ | prisma:query | 応答時間（1回目 / 2回目） |
|---|---:|---|
| `/dashboard` | **10 本**（TASK 1 後のベースラインと同じ） | 100.6ms / 94.2ms |
| `/`（トップ） | 8 本 | 80.0ms / 77.0ms |
| `/goals` | 7 本 | 84.8ms / 82.4ms |
| `/profile` | 4 本 | 149.2ms / 79.1ms |
| `/explore/1` | 6 本 | 88.0ms / 69.5ms |

**クエリ本数は増えていない。** `/dashboard` は TASK 1 完了時点の 10 本を維持している。
この TASK は責務の移動が目的で、本数削減は狙っていない（削減は TASK 2 で「何もしない」と
決定済み）。

新しいサービス関数が実際に計測ログへ出ていることも確認した（例）:

```
{"operation":"goal.findFirstChoice","duration_ms":17.07,"success":true}
{"operation":"goal.listWithFaculty","duration_ms":21.15,"success":true}
{"operation":"goal.listFacultyIds","duration_ms":2.44,"success":true}
{"operation":"textbook.listSubjects","duration_ms":2.72,"success":true}
{"operation":"university.findDetail","duration_ms":11.11,"success":true}
{"operation":"notificationPreference.find","duration_ms":6.62,"success":true}
{"operation":"lineConnection.find","duration_ms":2.66,"success":true}
```

### 表示が変わっていないことの検証

「目視で同じに見える」ではなく、**変更前後の HTML を実際に取得して差分を取った**。
変更を `git stash` して同じ dev server から before を取得し、pop して after と比較した。

`<script>`（RSC ペイロード）と `<style>` を除いた**表示 DOM** で比較:

| ページ | データ | DOM 行数 | 結果 |
|---|---|---:|---|
| `/dashboard` | e2e（志望校 0 件） | 557 | 完全一致 |
| `/goals` | e2e | 107 | 完全一致 |
| `/profile` | e2e | 183 | 完全一致 |
| `/explore/1` | e2e | 87 | 完全一致 |
| `/`（トップ） | e2e | 89 | 完全一致 |
| `/dashboard` | **demo（志望校 4 件）** | 501 | 完全一致 |
| `/goals` | **demo（志望校 4 件）** | 324 | 完全一致 |

e2e ユーザーは志望校が 0 件で dashboard の志望校描画を通らないため、**志望校を持つ demo
ユーザーでも追加検証した**（`listGoalsWithFaculty` で tags を落とした影響がここに出る）。
demo でも `/dashboard` は 10 本のままで、DOM も完全一致。

RSC ペイロード内には差分が出るが、内容は `self.__next_r`（リクエストごとのランダム ID）と
flight payload の内部参照番号（`$176` → `$177` 等、import 順の変化によるインデックスずれ）
だけで、表示内容ではない。

### 完了条件の達成状況

- ✅ 7 箇所すべてがサービス層経由
- ✅ `grep -rn "prisma\." app/ --include="*.tsx" | grep -v "/api/"` が **0 件**
- ✅ 各ページの表示が変わっていない（HTML 差分で確認・上表）
- ✅ クエリ本数が増えていない（`/dashboard` 10 本を維持）
- ✅ `npm run check` 成功（ESLint、tsc、Vitest 34 ファイル 240 テスト、production build）

---

## TASK 4: DTO変換の重複を解消（2026-09-09）

### 事実: 重複は2箇所ではなく4箇所だった

手順書は「app/page.tsx と app/dashboard/page.tsx に約25行がほぼ同一」としていたが、実際は
**同じ DTO を4箇所が別々に作っていた**。

| 場所 | 変換対象 | 書き方 |
|---|---|---|
| `app/page.tsx` | StudyPlan | 26行を手書き |
| `app/dashboard/page.tsx` | StudyPlan | **1文字違わず同じ26行** |
| `app/dashboard/page.tsx` | StudyLog | 同じ構造の25行 |
| `app/api/study-plans/route.ts` | StudyPlan | `studyLogId` の平坦化だけ手書き、`Date → ISO 文字列`は`JSON.stringify`任せ |

4番目が重要で、API は**変換を書いていないのに結果の形が一致していた**。`NextResponse.json()`
が内部で呼ぶ `JSON.stringify` が Date を暗黙に ISO 文字列へ変えるため。つまり形の一致が
**偶然に依存**しており、フィールドを1つ足すと4箇所が黙ってずれる状態だった。

`app/api/study-logs/route.ts` に至っては `NextResponse.json(logs)` とサービス層の戻り値を
そのまま渡しており、変換は完全に暗黙だった。

### 判断: `lib/dto/study.ts` に型と変換を両方置く（ユーザー承認済み）

置き場所の決め手は**依存の向き**だった。`StudyPlan` 型は `app/hooks/useStudyPlans.ts`
（TanStack Query 用のクライアントフック）に定義され、**14ファイル**が import している。
変換関数だけを `lib/` へ移すと `lib/` が `app/hooks/` の型を参照し、依存が逆流する。
TASK 6 で「ESLint によるレイヤー境界の強制」を入れる予定なので、ここで逆流を残すと
TASK 6 でやり直しになる。

そこで**型と変換をセットで `lib/dto/study.ts` へ置き**、`app/hooks/` 側は re-export だけに
した。既存14ファイルの import は1行も変えていない。

```
lib/dto/study.ts       … Textbook / StudyPlan / StudyLog 型 ＋ toStudyPlanDTO / toStudyLogDTO
app/hooks/useStudyPlans.ts … export type { Textbook, StudyPlan } from "@/lib/dto/study";
app/hooks/useStudyLogs.ts  … export type { StudyLog } from "@/lib/dto/study";
```

変換の入力型は `Prisma.StudyPlanGetPayload<{ include: ... }>` で Prisma から導出している。
schema や include を変えると変換側が型エラーになるので、直し忘れに気づける。

### API Route も同じ変換を通した

`study-plans` / `study-logs` の GET を `toStudyPlanDTO` / `toStudyLogDTO` 経由にした。
これで「JSON.stringify が暗黙に変換してくれている」という依存が消え、画面と API の応答形が
**1つの関数で保証される**ようになった。

### 検証: API 応答の値が変わっていないこと

TASK 3 と同じ方法（`git stash` で変更前に戻して同じ dev server から取得 → pop して比較）で
4つの API 応答を比較した。

| 応答 | 件数 | 値の一致 | キー順 |
|---|---:|:--:|---|
| `/api/study-plans`（e2e） | 0 | ✅ | 一致 |
| `/api/study-logs`（e2e） | 22 | ✅ | 相違 |
| `/api/study-plans`（demo） | 8 | ✅ | 相違 |
| `/api/study-logs`（demo） | 17 | ✅ | 相違 |

**値はすべて完全一致。** キーの並び順だけが変わっている（従来は `...plan` のスプレッドで
Prisma の列順、現在は DTO 関数が組み立てる順）。JSON の消費側はプロパティ名でアクセスする
ため意味に影響はなく、クライアントコードは無変更。

### 検証: 表示 DOM

| ページ | データ | DOM 行数 | 結果 |
|---|---|---:|---|
| `/dashboard` | e2e | 557 | 完全一致 |
| `/goals` | e2e | 107 | 完全一致 |
| `/profile` | e2e | 183 | 完全一致 |
| `/explore/1` | e2e | 87 | 完全一致 |
| `/`（トップ） | e2e | 89 | 完全一致 |
| `/dashboard` | demo | 501 | 完全一致 |
| `/`（トップ） | demo | 92 | 完全一致 |

クエリ本数も変化なし（`/dashboard` 10本、`/` 8本）。

### テストを1件修正した（実装のバグではない）

`app/api/study-logs/route.test.ts` の「ログイン済みなら自分の実績を 200 で返す」が
`Cannot read properties of undefined (reading 'toISOString')` で落ちた。

原因は**モックが実際の Prisma では返り得ない部分的な行**
（`{ id: 1, minutes: 60, userId: "user-1" }`）を返していたこと。変換を通さない旧実装
だったから通っていただけで、実データでは全スカラー列が必ず返る。モックを実際の行に揃え、
**日付が ISO 文字列になって返ることまで検証する**アサーションに変更した。
テストが1段強くなっている。

### 完了条件の達成状況

- ✅ 重複が解消（4箇所 → `lib/dto/study.ts` の2関数）
- ✅ 両ページの表示が変わっていない（DOM 差分・上表）
- ✅ API 応答の値も変わっていない（4応答すべて一致）
- ✅ `npm run check` 成功（ESLint、tsc、Vitest 34ファイル 240テスト、production build）

---

## TASK 5: lib/ 直下の整理（2026-09-09）

構造のみの変更で、ロジックは1行も書き換えていない。計測値に影響する変更ではないが、
検証のため TASK 3〜4 と同じ前後比較を行った。

### 手順書の記述との相違（実測で2点を訂正）

1. **`dailyNotification.ts` は「バックエンド専用」ではなく純粋関数だった。** 依存は
   `@/lib/site`（定数）のみで、Prisma もネットワークも触らない。メッセージ組み立てと
   日付範囲計算だけなので `domain/` に分類した。
2. `site.ts` がクライアントから import されていないことは手順書どおり。調査の途中で
   「5件ある」と誤認したが、原因は `grep -rl '"use client"'` が**コメント内の文字列**
   （`// page.tsx が "use client" のため〜`）を拾っていたこと。ディレクティブは先頭の
   文字列リテラルなので、コメントと空行を除いた先頭で判定し直して確認した。

### 採用した構成（案A・現行ファイル名を維持／ユーザー承認済み）

```
lib/
├── domain/         純粋ロジック（10本）DB・ネットワーク・ブラウザAPIに触れない
│     date, subjects, prefectures, studyLog, studyPlan, studyStats,
│     studySession, examSchedule, reverseStudyNavigation, dailyNotification
├── infra/          サーバー専用（8本）外部システムとの接続
│     prisma, resend, email, microcms, line, auth, auth-session, lineLogin
├── ui/             ブラウザ側（4本）
│     browser, analytics, utils, auth-client
├── demo/           constants（定数）/ guard（サーバー）/ client（"use client"）
├── config/         site
├── services/       既存5本 ＋ sendDailyNotifications
└── dto/ validations/ observability/   （既存のまま）
```

ファイル名は現行のまま移動した（`git mv` で履歴を保持）。差分が「移動」だけになり、
レビューと切り戻しが楽なため。既存 `services/` `dto/` は kebab-case、移動分は camelCase
のままなので、lib 内で命名は混在している（今回は変えない判断）。

### 判断が必要だった2点

**`demo.ts` の分割 → `lib/demo/` に3ファイル。** 参照元を実測すると、`DEMO_EMAIL` は
Server Component 5ファイル、`demoReadOnlyGuard` は API Route 12ファイルが使い、
**両方を使うファイルは1つも無かった**ため分割は安全と判断した。`guard.ts` は
`next/server` に依存するサーバー専用モジュールなので、定数を `constants.ts` へ分けて
画面から参照できるようにしている。既存の `demo-client.ts` も `demo/client.ts` へ寄せた。

**`sendDailyNotifications.ts` → `lib/services/`。** Prisma・Resend・LINE・純粋ビルダー
（`dailyNotification.ts`）を束ねて1つの Route Handler から呼ばれており、役割は
アプリケーションサービスそのもの。純粋な `dailyNotification.ts` は `domain/` に分けた。

### import パスの更新

`@/lib/xxx` 形式を機械置換（121ファイル）。加えて手作業で拾ったもの:

- `@/lib/demo` は分割したので、import する識別子（`DEMO_EMAIL` / `demoReadOnlyGuard`）で
  `constants` と `guard` に振り分けた（17ファイル）
- `vi.mock("@/lib/demo", ...)` が1件残っていた（import 文ではないので一括置換の対象外）
- `prisma/seed*.ts` の3件が `from "../lib/date"` という**相対パス**で参照しており、
  `@/lib/` を対象にした置換では拾えなかった。tsc が検出した
- **`components.json`（shadcn/ui の設定）の `"utils": "@/lib/utils"`。** これを直さないと
  今後 `npx shadcn add` が生成するコンポーネントの import が壊れる。`@/lib/ui/utils` に更新した

### 検証

| 対象 | 結果 |
|---|---|
| 表示 DOM 7ページ（e2e 5・demo 2） | すべて完全一致 |
| API 応答 4本（0件・22件・8件・17件） | **バイト単位で完全一致** |
| デモの閲覧専用ガード（demo で POST /api/study-logs） | 403（分割後も機能） |
| `npm run check` | 成功（ESLint、tsc、Vitest 34ファイル 240テスト、build） |

TASK 4 と違いロジックを変えていないので、API 応答はキー順まで含めて一致している。

### 完了条件の達成状況

- ✅ `lib/` 直下に平置きファイルが残っていない（本番コード25本・テスト14本すべて移動）
- ✅ `npm run build` が通る
- ✅ 既存テストが全て通る（240件）
- ✅ テストファイルは隣接配置（`domain/date.ts` の横に `domain/date.test.ts`）
