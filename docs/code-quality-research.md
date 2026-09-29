# コード品質・データ構造・アーキテクチャ調査

**調査日: 2026-09-19**

**状態: 調査のみ。ここに挙げた改善候補は、まだ実装方針として確定していない。**

## 結論

このプロジェクトで目指す「良いコード」は、層やデザインパターンの数ではなく、次の性質で判断する。

- 正しさをDB制約・型・テストで守れる
- 変更箇所と影響範囲が予測できる
- 処理量が増えたとき、計測にもとづいて改善できる
- 障害時に原因を追える
- 小さく安全にレビュー・リリース・ロールバックできる

受験マップでは、全面的な作り直しやリポジトリ層の追加より、現在の
`routes → services → infra`を保ちながら、機能単位の凝集、境界の機械的な強制、
API契約とDB整合性の強化を進める方が実務的である。

## 評価軸

| 観点 | 良い状態 | このプロジェクトで確認すること |
|---|---|---|
| データ構造 | 重複状態を持たず、検索・集計に適した形 | React state、DTO、DB制約、Map/Set |
| アルゴリズム | Big-Oだけでなく実データで計測 | SQL本数、全件走査、N+1、実行計画 |
| 責務分離 | 変更理由の異なる処理が分離されている | HTTP、業務ルール、SQL、外部API |
| 依存方向 | 上位から下位への一方向 | routes→services→infra、sharedの純度 |
| 契約 | 入出力とエラーを機械的に検証できる | Zod、レスポンスschema、DTO |
| テスト | 壊れやすい境界を本物に近い状態で確認 | Fastify inject、本物のMySQL、E2E |
| 運用 | コード変更なしに遅延・障害を調査できる | logs、metrics、traces、SLO |
| チーム開発 | 所有者が分かり、小さくレビューできる | PR分割、CODEOWNERS、ADR |

## 1. データ構造とアルゴリズム

Webアプリでは、アプリ内の配列処理より、DB・ネットワーク・外部APIがボトルネックに
なりやすい。改善は次の順で考える。

1. 不要なSQLやN+1をなくす
2. `WHERE`・`JOIN`・`ORDER BY`に合う複合インデックスを設計する
3. `EXPLAIN ANALYZE`で推測と実測を比較する
4. 大量データをアプリへ取得してから絞らず、DBで絞る
5. 同じ配列を繰り返し検索するときだけMap/Setへ変える

インデックスは読み取りを速くする一方、書き込み・容量・オプティマイザのコストを増やす。
理論だけで追加せず、代表データと実行計画で判断する。

現状の良い例：

- `db/migrations/20260915090000_add_user_date_indexes/migration.sql`では、36.9万行で
  `268ms → 1.75ms`を実測してから複合インデックスを追加している
- `src/shared/studyStats.ts`では集計にMap/Setを使っている
- 本番サービスで目立った`SELECT *`がなく、必要列を明示している
- JOIN、複数行INSERT、条件付きUPDATEを用途に応じて使っている

静的確認では、重大なO(n²)処理は見つからなかった。固定された少数分類に対する
`map()`内の`find()`などは、Mapへ変更しても実質的な改善がないため、機械的に置換しない。

今後データ量が増えた際の確認候補：

- 一覧のページネーション
- 管理画面の相関サブクエリ
- 通知処理の対象者数とチャネル数
- 大量ユーザー時の認証SQL本数

## 2. DBで守る正しさ

業務上壊れてはいけない条件は、可能なら次の順で守る。

1. `NOT NULL`
2. `UNIQUE`
3. `FOREIGN KEY`
4. `CHECK`
5. 条件付き`UPDATE`
6. トランザクション
7. アプリ側の事前検証

予定完了と実績作成を同一トランザクションにする、一意制約で二重完了を止める、
初回記録を条件付きUPDATEで判定する現在の実装は、この考え方に合っている。

改善候補として、更新・削除は事前に所有者を確認するだけでなく、最終SQL自体を
`WHERE id = ? AND userId = ?`として`affectedRows`を確認する。認可条件が最終操作まで残り、
確認と更新の間の競合にも強くなる。

## 3. ディレクトリ構造

この規模では、最上位を機能単位、その機能内を責務単位にするハイブリッド型が候補になる。

```text
apps/api/src/
  features/
    study-plans/
      routes.ts
      service.ts
      schema.ts
      queries.ts       # SQLが大きくなった場合のみ
      routes.test.ts
      service.test.ts
    study-logs/
    goals/
    notifications/
  platform/
    db/
    auth/
    observability/
    integrations/
  app.ts
  server.ts

apps/web/src/
  features/
    study-plans/
      api.ts
      hooks.ts
      model.ts
      components/
      tests/
    study-logs/
    goals/
  pages/
  shared/
    ui/
    lib/

src/shared/
  dto/
  validations/
  domain/
```

目的は木構造を整えることではなく、1機能を変更するときに関係ファイルが近く、影響範囲を
予測しやすくすること。全機能へ機械的に`repository/`、`usecase/`、`entity/`を追加しない。

リポジトリ層は引き続き導入しない。同じテーブルのSQLが3ファイル以上へ散り、列変更時の
修正漏れが実際に起きるなど、既存の見直し条件を満たした場合だけ再検討する。

## 4. 依存境界の機械的な強制

現状の優先度が高い候補。ルート`eslint.config.mjs`は`apps/**`を対象外にしているため、
実際のWeb・APIコードにルートのESLintルールが適用されていない。

将来的に自動検出したいもの：

- WebからAPI内部へのimport
- routesからinfraへの直接依存
- sharedからappsへの依存
- feature同士の内部ファイルへの直接import
- 循環依存
- 未処理Promise、危険な型アサーション
- React Hooks、TanStack Queryの誤用

TypeScript Project Referencesやパッケージ境界は、ビルド順だけでなく論理的な分離の強制にも
利用できる。ただし現在の規模でパッケージを細分化しすぎない。

## 5. API契約とエラー

現在はZodでリクエストを検証しているが、成功レスポンスの実行時契約は弱い。
また、エラーが文字列の場合とZod Issue配列の場合があり、Web側の`api-client.ts`が差を吸収している。

改善候補：

- リクエスト、成功レスポンス、エラーレスポンスを明示する
- エラーに安定した機械用コードを付ける
- 画面表示文言とエラー種別を分ける
- 入出力schema、型、APIドキュメントの正を一つにする
- レスポンスschemaで意図しない項目の流出を防ぐ

エラー形式はRFC 9457の`type`、`title`、`status`、`detail`、`instance`を設計材料にできる。
将来スマートフォンアプリから同じAPIを利用する段階ではOpenAPI生成の価値が上がるが、
先にレスポンスschemaとエラー契約を一貫させる。

## 6. Fastifyのモジュール境界

現在は各`registerXRoutes(app)`が同じFastifyインスタンスへ直接ルートを登録しており、
Fastifyの`register()`によるカプセル化は利用していない。

機能単位のplugin境界を使うと、その機能の認証hook、schema、prefix、依存を閉じ込められる。
ただし、ファイルを増やすことを目的にせず、LINE連携のようにrouteが大きく、認証・外部連携・
業務分岐がまとまっている機能から検討する。

## 7. フロントエンド

現在のTanStack Queryの使い方は良い。

- query keyをhookへ集約している
- mutation成功後のinvalidateをhook側で行う
- API呼び出しを`api-client`へ集約している
- サーバー状態をローカルstateへ複製していない

React stateの基準：

- propsや既存stateから計算できる値をstateにしない
- 同じ情報を複数のstateへ重複保存しない
- 選択中オブジェクトではなくIDを持つ
- Effectは外部システムとの同期に使う
- 複雑なUI状態は状態遷移として整理する

現状は`useEffect`が5件で乱用は見られない。一方、500行を超えるフォーム、ダイアログ、
LPコンポーネントがある。行数だけで分割せず、データ取得・フォーム状態・計算・表示など、
異なる変更理由が混在している場合に機能単位で分ける。

## 8. テストとレビュー

推奨する役割分担：

- 純粋な業務ルール・集計：単体テスト
- API、認証、SQL、制約：Fastify `inject()`＋本物のMySQLによる統合テスト
- ユーザーの重要動線：少数のPlaywright E2E
- 性能：代表データによる`EXPLAIN ANALYZE`と応答時間計測

現在の「外部サービスだけ差し替え、SQLは本物のMySQLへ流す」方針は生SQL構成に合っている。
テスト件数やカバレッジ率だけを目標にせず、壊したとき本当に失敗するテストかをレビューする。

リファクタと機能変更は原則別PRにし、1PRを1つの自己完結した変更にする。ディレクトリ再編も
一括移動ではなく、機能単位で挙動不変を確認しながら進める。

## 9. 追加で組み込む価値がある要素

### DBスキーマとTypeScript型のドリフト検出

生SQLではmigrationに列を追加しても`infra/tables.ts`の更新漏れをTypeScriptが検出できない。
CIで`information_schema`と期待schemaを照合するか、行型の生成を検討する。

### セキュリティ基準

OWASP ASVSを全部そのまま導入せず、認証、アクセス制御、入力検証、外部通信、秘密情報など
該当項目を選んでチェックリスト化する。生SQLの値はパラメータ化し、列名・並び順など
プレースホルダーを使えない部分はコード内の許可リストから選ぶ。

### 可観測性とSLO

既存のlogs、metrics、tracesに、次のユーザー視点の正常条件を結び付ける。

- API成功率
- p95レイテンシ
- DBクエリ時間
- 通知成功率
- フロントエンドエラー率

### アクセシビリティ

型や性能に加え、キーボード操作、フォーカス、ラベル、コントラスト、エラー通知を品質基準に
含める。基準候補はWCAG 2.2 AA。

### 所有権と意思決定

- 重要領域のCODEOWNERS
- アーキテクチャ変更の短いADR
- 小さなPRと関連テスト
- 変更理由と負のトレードオフも残す

`docs/architecture.md`は、採用しなかった案と理由まで記録しており、すでにADRに近い良い状態。

## 実装する場合の優先順位候補

以下は調査時点の提案であり、ユーザーが着手を決定したものではない。

1. `apps/**`を含むlintと依存境界の強制
2. routesに残る業務ルールをservices/domainへ移す
3. 更新・削除SQLに`userId`条件を含める
4. API成功レスポンスとエラー契約を統一する
5. 大きな機能からfeature単位へ配置を整理する
6. 主要SQLを`EXPLAIN ANALYZE`と実データ量で評価する
7. DB型ドリフト検出、セキュリティ、SLO、アクセシビリティを品質基準へ追加する

避けるもの：

- ディレクトリを整えることだけを目的にした一括移動
- 通過するだけの層を増やすリファクタ
- 行数やBig-Oだけを根拠にした最適化
- リファクタと挙動変更を同じ大きなPRに混ぜること

## 不確実な点

- 今回は静的調査であり、本番トラフィック下の全SQL実行計画は取得していない
- 大きいコンポーネントは分割候補だが、行数だけでは責務混在を断定できない
- feature-first構成は有力候補だが、チーム人数と変更頻度が増える前に全面移行する必要はない
- OpenAPI、Project References、Fastify plugin化は、導入コストに見合う対象から段階的に判断する

## 主な一次資料

- [MySQL 8.4: Optimization and Indexes](https://dev.mysql.com/doc/refman/8.4/en/optimization-indexes.html)
- [MySQL 8.4: EXPLAIN](https://dev.mysql.com/doc/refman/8.4/en/explain.html)
- [MySQL 8.4: Transaction Isolation Levels](https://dev.mysql.com/doc/refman/8.4/en/innodb-transaction-isolation-levels.html)
- [MySQL 8.4: CHECK Constraints](https://dev.mysql.com/doc/refman/8.4/en/create-table-check-constraints.html)
- [Fastify: Validation and Serialization](https://fastify.dev/docs/latest/Reference/Validation-and-Serialization/)
- [Fastify: Encapsulation](https://fastify.dev/docs/latest/Reference/Encapsulation/)
- [Fastify: Testing](https://fastify.dev/docs/latest/Guides/Testing/)
- [React: Choosing the State Structure](https://react.dev/learn/choosing-the-state-structure)
- [React: You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)
- [TanStack Query: Query Keys](https://tanstack.com/query/latest/docs/framework/react/guides/query-keys)
- [TanStack Query: Invalidations from Mutations](https://tanstack.com/query/latest/docs/framework/react/guides/invalidations-from-mutations)
- [TypeScript: Project References](https://www.typescriptlang.org/docs/handbook/project-references)
- [RFC 9457: Problem Details for HTTP APIs](https://www.rfc-editor.org/rfc/rfc9457.html)
- [OWASP ASVS](https://owasp.org/projects/asvs)
- [OWASP SQL Injection Prevention](https://cheatsheetseries.owasp.org/cheatsheets/SQL_Injection_Prevention_Cheat_Sheet.html)
- [OpenTelemetry: Observability Primer](https://opentelemetry.io/docs/concepts/observability-primer/)
- [Google SRE: Service Level Objectives](https://sre.google/sre-book/service-level-objectives/)
- [WCAG 2.2](https://www.w3.org/TR/wcag/)
- [Google Engineering Practices: What to Look for in a Code Review](https://google.github.io/eng-practices/review/reviewer/looking-for.html)
- [Google Engineering Practices: Small CLs](https://google.github.io/eng-practices/review/developer/small-cls.html)
- [GitHub Docs: CODEOWNERS](https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/customizing-your-repository/about-code-owners)
- [Documenting Architecture Decisions](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
