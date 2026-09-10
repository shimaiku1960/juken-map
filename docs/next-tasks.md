# 受験マップ 改善タスク（TASK 2 以降）引き継ぎ

Next.js 16 App Router / React 19 / TypeScript / Prisma 7 / MySQL 8.4 / Better Auth の
個人開発プロジェクト。本番は AWS EC2 + RDS（juken-map.com）で稼働中。
Prisma Client の出力先は app/generated/prisma/client。

目的は機能追加ではなく、計測にもとづく構造改善と性能改善。

## 進め方の原則（厳守）
- 大規模リファクタはしない。1タスクずつ、変更を小さく保つ
- 各タスクの前後で必ず計測し、数字を docs/performance.md に追記する
- 調査で分かった事実と推測を区別して書く。推測には「推測」と明記する
- 判断に迷う設計上の選択が出たら、勝手に決めずに選択肢とトレードオフを提示して止まる
- 機能追加はしない
- AGENTS.md の指示に従い、コードを書く前に node_modules/next/dist/docs/ の該当ガイドを読む
- 応答は日本語

---

## 現在の状態

### 完了済み: TASK 1（getSession の重複解消）

lib/auth-session.ts を新設し、getSession() を React の cache() で包んだ。
Header.tsx と7ページ（/, /dashboard, /goals, /profile, /explore,
/explore/[universityId], /line/link）の呼び出しを getCurrentSession() に置き換え済み。
measured() で "auth.getSession" としてラップしてある。

結果（/dashboard を1回 GET）:
- session SELECT 2本 → 1本
- user SELECT 2本 → 1本
- ページ全体の prisma:query 12本 → 10本

tsc / eslint / npm test（240件）すべて通過済み。
一時デバッグコードは削除済み。記録は docs/performance.md にある。

better-auth の session.cookieCache は **有効化していない**。
セッション無効化が TTL 中反映されない副作用があるため、別途判断する。今回も触らないこと。

### コミット状況

`refactor/layer-boundary` ブランチに5コミット。作業ツリーはクリーン
（`docs/next-tasks.md` はこの手順書自体なので未追跡のまま）。

```
fc4276b  refactor: UI層からの Prisma 直接呼び出しをサービス層へ移す  ← TASK 3
8c2acc5  docs: TASK 2 の回避策は「何もしない」と決定した記録を残す   ← TASK 2
2b705b2  perf: セッション取得をリクエスト内で1回に集約する          ← TASK 1
8e80d76  refactor: 一覧取得をサービス層に切り出し所要時間を計測する
8e9146b  refactor: ログアウトをクライアント側の authClient に統一
113721c  (main の先頭)
```

**push も PR 作成もまだしていない。** 勝手に push・PR 作成・main へのマージをしないこと。
必要になったらユーザーに確認する。以降のタスクもこのブランチで進めてよい。

---

## 計測の標準手順（毎回これに従う）

dev server はポート3000で既に起動している場合がある（`lsof -ti:3000` で確認）。
起動していなければ `npm run dev:infra` で DB を上げてから `npx next dev` する。
ユーザーが起動したプロセスを勝手に kill しないこと。

1. curl でログインして Cookie を取得する
   ```
   curl -s -c /tmp/cookies.txt -X POST http://localhost:3000/api/auth/sign-in/email \
     -H 'Content-Type: application/json' \
     -d '{"email":"e2e@juken-map.com","password":"e2epass1234"}'
   ```
   E2Eユーザーが未投入なら `npx tsx --env-file=.env prisma/seed-e2e.ts` で作る。
   認証情報は e2e/credentials.ts にある。
2. コンパイルのため対象ページを1回踏む（この回は計測に含めない）
3. curl で対象ページを1回 GET し、`.next/dev/logs/next-development.log` に出た
   prisma:query の本数とレスポンス時間を記録する
4. 同じ手順を2回実施して再現性を確認する

### ハマりどころ（前任者が踏んだ）
- **ブラウザで同じアプリを開いたままだと、その裏側の fetch が同じログに混ざる。**
  ログ行はタイムスタンプの空白（300ms以上）でクラスタに分け、curl 直後の最初の
  クラスタだけを数えること。これを怠ると10本のはずが19本に見える。
- **自分で複数ページを連続で叩いた直後も同じ問題が起きる。** 7ページのスモークテストの
  直後に計測すると、7リクエスト分のログが1クラスタに融合して18本に見える。
  計測の前は5〜6秒空けること。
- 計測は必ず curl で行う。ブラウザで開くとページ描画の後に /api/study-logs などへの
  fetch が続き、1リクエスト分の数字が取れない。
- tsx でスクリプトを書くとき、トップレベル await は cjs 出力で通らない。async main() で包む。

---

## TASK 2: 空振りクエリの調査（完了・コード変更なし）

**決定（2026-09-09・ユーザー判断）: 選択肢1「何もしない」を採用。**
空振り5本のコストは約3ms、対する選択肢2は preview 機能 relationJoins への依存を負う。
実測値がそのリスクに見合わないため見送った。検証結果は docs/performance.md に残してあるので、
実データで問題が出たら再検証なしに再検討できる。以下は調査時の記録。

### 事実（計測済み）
/dashboard の10クエリ中3本が、結果0件と分かりきっているのに発行されている。

```
SELECT ... FROM `Faculty`    WHERE `Faculty`.`id`          IN (NULL)
SELECT ... FROM `Textbook`   WHERE `Textbook`.`id`         IN (NULL)
SELECT ... FROM `University` WHERE `University`.`id`       IN (NULL)
SELECT ... FROM `StudyLog`   WHERE `StudyLog`.`studyPlanId` IN (NULL)
```

発行順から、以下の include が原因と見られる（未確定・要検証）:
- Faculty / University → app/dashboard/page.tsx:30 の
  `include: { faculty: { include: { university: true } } }`
- Textbook / StudyLog.studyPlanId → lib/services/study-plan-service.ts と
  lib/services/study-log-service.ts の `include: { textbook: true, studyLog: {...} }`

### やること
1. 発行元のコードを特定する（どの include / select が原因か、行番号まで）
2. テストデータ固有の現象か、実データでも起きるかを判断する
   - 確認すべき観点: 親の行数が0件のときに出るのか、親の行はあるが FK が全部 NULL の
     ときに出るのか。schema.prisma で FinalGoal.facultyId / StudyPlan.textbookId /
     StudyLog.textbookId の NULL 許容を確認すること
   - 「新規登録直後の志望校0件・参考書未登録のユーザー」で再現するなら実データでも起きる
3. 回避策を提示する。**実装はしない**

### 完了条件
原因と、実データでも発生するかどうかの判断を報告する。
10万件データを入れたときに IN 句が巨大化するリスクがあるかも併せて評価すること。

---

## TASK 3: UI層からの Prisma 直接呼び出しをサービス層へ移す（完了・2026-09-09）

**完了。7箇所すべてサービス層経由になり、`grep -rn "prisma\." app/ --include="*.tsx" |
grep -v "/api/"` は0件。`/dashboard` は10本のままで増えていない。変更前後の HTML を
取得して表示 DOM が全ページ完全一致することを確認済み（志望校4件の demo ユーザーでも検証）。
`npm run check` 成功（240テスト）。詳細は docs/performance.md の TASK 3 節。**

**dashboard の判断:** 手順書の指示（既存 listGoals を使う）には従わず、tags を含まない
`listGoalsWithFaculty` を新設した。listGoals だと暗黙的多対多 Faculty.tags のぶん
SQL が1本増え、完了条件「クエリ本数が増えていないこと」を満たせないため。ユーザー承認済み。

TASK 3 完了。

以下は着手時の調査記録（行番号は TASK 3 実施後にずれている）。

### 対象（調査済み・7箇所5ファイル）
| ファイル | 行 | 内容 |
|---|---|---|
| app/page.tsx | 36 | finalGoal.findFirst（第一志望1件、include: faculty.university） |
| app/goals/page.tsx | 46 | textbook.findMany（select: subject のみ） |
| app/explore/[universityId]/page.tsx | 33 | university.findUnique（include: faculties.tags） |
| app/explore/[universityId]/page.tsx | 47 | finalGoal.findMany（select: facultyId のみ） |
| app/dashboard/page.tsx | 30 | finalGoal.findMany（既存 listGoals で置き換え可能） |
| app/profile/page.tsx | 38 | notificationPreference.findUnique |
| app/profile/page.tsx | 42 | lineConnection.findUnique |

※ TASK 1 で import 行が減っているため、行番号は数行ずれている可能性がある。grep で確認すること。

### 重複している処理（統合対象）
- app/profile/page.tsx の notificationPreference.findUnique と
  app/api/notification-preferences/route.ts:21 が where・select とも完全に同一
- app/profile/page.tsx の lineConnection.findUnique と
  app/api/notification-preferences/route.ts:52 が1文字違わず同一
- app/dashboard/page.tsx の finalGoal.findMany は既存 listGoals で置き換え可能

### 既存のサービス層（命名と粒度をこれに合わせる）
lib/services/ に5ファイル、各1関数。全て measured() でラップ済み。
- goal-service.ts: listGoals(userId)
- study-log-service.ts: listStudyLogs(userId)
- study-plan-service.ts: listStudyPlans(userId)
- textbook-service.ts: listTextbookMasters()
- university-service.ts: listUniversitiesForExplore()

### やること
1ファイルずつ、以下の手順で進める。
1. 対応するサービス関数を lib/services/ に追加する（既存があれば流用）
2. page.tsx の直接呼び出しを置き換える
3. 動作確認する（curl で HTTP 200 と、表示内容が変わっていないこと）
4. 次のファイルに進む

全関数を measured() でラップする。

**app/dashboard/page.tsx の finalGoal.findMany は新規作成せず、既存の listGoals に
置き換えること。** ただし listGoals の include のほうが範囲が広く（faculty.tags を含む）、
余分なデータを取ることになる。それが許容できるか判断し、できない場合は理由と代案を
報告して止まること。

### 完了条件
- 上記7箇所すべてがサービス層経由になっている
- `grep -rn "prisma\." app/ --include="*.tsx" | grep -v "/api/"` が0件
- 各ページの表示が変わっていないことを確認
- 標準手順で計測し、クエリ本数が増えていないことを確認
- docs/performance.md に記録

---

## TASK 4: DTO変換の重複を解消（完了・2026-09-09）

**完了。`lib/dto/study.ts` に型と変換（`toStudyPlanDTO` / `toStudyLogDTO`）を集約し、
`app/hooks/` は re-export だけにした（既存14ファイルの import は無変更）。API Route の GET も
同じ変換を通している。API応答の値は4応答すべて完全一致、表示DOMも7ページ完全一致、
`npm run check` 成功（240テスト）。詳細は docs/performance.md の TASK 4 節。**

**実際の重複は2箇所ではなく4箇所だった**（API が `JSON.stringify` の暗黙 Date 変換に
依存して、書かずに同じ形を作っていた）。置き場所は依存の向きで決定：型が
`app/hooks/useStudyPlans.ts` にあるため、変換だけ lib へ移すと lib→app の逆流が残り
TASK 6 と衝突する。よって型ごと lib/dto へ移した。

TASK 4 完了。

以下は着手時の記録。

### 事実
app/page.tsx と app/dashboard/page.tsx に、StudyPlan を UI 用の型
（app/hooks/useStudyPlans の StudyPlan）に変換する処理が約25行、ほぼ同一で存在する。
Date を toISOString() して textbook をネストし直すだけの処理。

### やること
変換ロジックの置き場所を決めて共通化する。
サービス層に入れるか、別途 lib/ に変換層を作るかは、
**選択肢とトレードオフを提示してから決めること。勝手に決めない。**

論点として最低限これは含めること:
- サービス層が Prisma の型をそのまま返すべきか、UI 用 DTO を返すべきか
- API Route も同じ変換を通すべきか（現状は Prisma の戻り値をそのまま JSON にしている）

### 完了条件
重複が解消され、両ページの表示が変わっていない。

---

## TASK 5: lib/ 直下の整理（完了・2026-09-09）

**完了。lib/ 直下の平置きは0件。** 採用構成（案A・現行ファイル名維持／ユーザー承認済み）:
domain（純粋10本）/ infra（サーバー専用8本）/ ui（ブラウザ4本）/ demo（constants・guard・client）
/ config（site）、services に sendDailyNotifications を追加。dto・validations・observability は既存のまま。
表示DOM 7ページ完全一致、API応答4本はバイト単位で一致、デモガードも403のまま、
`npm run check` 成功（240テスト）。詳細は docs/performance.md の TASK 5 節。

**手順書の記述を1点訂正:** `dailyNotification.ts` は「バックエンド専用」ではなく
`@/lib/site` しか参照しない純粋関数だったので domain/ に置いた。

**TASK 5B（ユーザーの追加指示・完了）**: その後 `lib/` を廃止し、責務で
`src/frontend` / `src/backend` / `src/shared` の3層へ再配置した。判断根拠は
`docs/architecture.md` にある。以下の TASK 6 は移動前のパスを前提に書かれているので、
`@/lib/prisma` → `@/backend/infra/prisma` と読み替えること。対象も
`src/frontend/**` を含め、除外は `app/api/**/route.ts` と `src/backend/**` になる。

**次はここから → TASK 6。**

以下は着手時の記録（分類はこの訂正を反映していない）。

### 事実
lib/ 直下に本番コード25本とテスト14本が平置きされている。調査で分類済み。

UI層でも使う（純粋関数・クライアント側）:
date.ts, subjects.ts, utils.ts, studyLog.ts, studyPlan.ts, studyStats.ts,
studySession.ts, examSchedule.ts, reverseStudyNavigation.ts, prefectures.ts,
browser.ts, analytics.ts, demo-client.ts, auth-client.ts

バックエンド専用:
prisma.ts, auth.ts, auth-session.ts, email.ts, resend.ts, microcms.ts, line.ts,
lineLogin.ts, sendDailyNotifications.ts, dailyNotification.ts

要検討:
- site.ts（定数のみ。サーバからのみ使用され、クライアントからの import は0件）
- demo.ts（NextResponse を使うサーバ専用関数 demoReadOnlyGuard と、
  UI でも使う DEMO_EMAIL 定数が同居。page.tsx 5件が定数だけを参照している）

### やること
1. **ディレクトリ構成案を提示する。実装前に承認を取ること**
2. 承認後、1ディレクトリずつ移動する
3. テストファイルは隣接配置で統一する（date.ts の横に date.test.ts）
4. import パスは自動修正するが、修正後に必ずビルドとテストを通す

判断を求められている論点2つ:
- demo.ts の分割。既に demo-client.ts が存在するため、3ファイルに分けるべきか、
  別の整理をすべきか、案を提示してから決めること
- sendDailyNotifications.ts は役割としてはアプリケーションサービスに相当する。
  lib/services/ に移すべきか、通知は別扱いにすべきか、判断を提示すること

### 完了条件
- lib/ 直下に平置きファイルが残っていない（サブディレクトリに整理済み）
- npm run build が通る
- 既存テストが全て通る

---

## TASK 6: ESLint でレイヤー境界を強制

### 前提
TASK 3 が完了していること（違反箇所が0件の状態で入れる）。

### やること
1. ESLint の設定ファイル形式を確認する（eslint.config.mjs があるので flat config）
2. no-restricted-imports で以下を禁止する
   - 対象: app/**/page.tsx, app/**/layout.tsx, components/**
   - 禁止する import: @/lib/prisma、app/generated/prisma/client への直接参照
   - 除外: app/api/**/route.ts, lib/services/**
   - エラーメッセージは日本語で代替手段を示す
     例:「UI層からDBに直接アクセスしないこと。lib/services 経由で呼ぶこと」
3. 動作確認: 適当な page.tsx に一時的に prisma の import を書いてエラーが出ることを
   確認し、その後削除する
4. GitHub Actions の既存ワークフローに lint が含まれているか確認し、なければ追加する

### 完了条件
- 境界を破る import が ESLint で検出される
- CI で lint が実行される
- 既存コードで新たなエラーが出ていない

---

## TASK 7: アーキテクチャのドキュメント化

docs/architecture.md を作成し、以下を記載する。

- レイヤー構造の図（テキストで可）
- 「UI層は Prisma を import しない」というルールとその理由
- ESLint で強制していること
- Server Actions を使い続ける理由（内部HTTPを経由せず速度を保てる）
- 外部クライアント（将来のモバイル）向けに REST API 層を残している理由
- Server Actions がモバイルから叩けない理由（URLが固定されない、
  Next-Action ヘッダのIDがビルドごとに変わる、ボディがRSC独自形式）
- リポジトリ層を導入しなかった理由（DB差し替えの予定がなく、
  Prisma の型推論と select の柔軟性を失うコストが上回るため）

**これは面接で説明する材料になるので、判断とその理由を中心に書くこと。
実装の説明ではなく、なぜそう決めたかを書く。**

---

## 全タスク完了後

docs/performance.md に、全タスクを通した改善サマリを追記する。
- 各タスクの前後の SQL 本数とレスポンス時間
- 最も効果が大きかった変更
- 数字で測れなかった変更（構造改善）については、何が良くなったかを言語化する

---

まず TASK 2 から始めて。調査結果を報告してから次に進むこと。
