# 受験マップ

大学受験生が、毎日の学習を簡単に記録し、予定・実績・志望校まで一か所で振り返れるWebアプリです。

「記録を続けるために入力の手間を減らすこと」を中心に、学習タイマー、クイック記録、カレンダーによる可視化を実装しています。全国の大学・学部マスターを使った志望校管理や、受験日程の確認にも対応しています。

[本番環境を開く](https://juken-map.com)（AWS EC2上でセルフホスト）

## デモ

登録せずに、志望校や学習予定が入った状態を試せます。

1. [ログイン画面](https://juken-map.com/login)を開きます。
2. 「デモでログイン」を選択します。
3. 学習記録、カレンダー、志望校管理などを確認します。

デモユーザーは読み取り専用です。編集操作はサーバー側でも制限しています。

手動でログインする場合は、次のテストアカウントを使用できます。

- メールアドレス: `demo@juken-map.com`
- パスワード: `demodemo1234`

## 画面

![学習内容を選んで記録を始める画面](app/components/landing/images/study-start-hero.png)

## 主な機能

### 学習を記録する

- **学習タイマー** — 今日の予定や「その他の学習」から内容を選び、タイマーを開始できます。終了後は内容と時間を確認して、そのまま実績として保存できます。
- **クイック記録** — タイマーを使わなかった学習も、日付と時間を選んで後から追加できます。
- **学習実績の編集** — 日付、時間、科目、教材、範囲、メモを後から修正できます。
- **学習予定** — 日ごとの予定に科目、参考書、学習範囲を設定できます。ドラッグによる日付変更や完了操作にも対応しています。

### 学習を振り返る

- **学習カレンダー** — 予定と実績を同じ月間カレンダーに表示します。日ごとの学習時間を色の濃淡で、科目の内訳をバーで確認できます。
- **学習状況の可視化** — 継続日数、直近7日間の科目別学習時間、当日の進捗を表示します。
- **逆算ナビ** — 参考書の総量、目標日、到達位置から、今日取り組む範囲の目安を提示します。

### 志望校と受験日程を管理する

- **大学を探す** — 全国823大学のマスターから、大学名、都道府県、設置区分で絞り込めます。
- **志望校管理** — 気になる大学を候補として保存し、受験校へ確定できます。第一志望と併願校も分けて管理できます。
- **受験日程** — 登録した志望校の受験日をタイムラインやカレンダーで確認できます。

### その他

- **認証** — Google、GitHub、メールアドレスとパスワードに対応しています。メール確認とパスワード再設定にはResendを使用しています。
- **PWA** — スマートフォンのホーム画面やMacのDockへ追加し、単独のアプリとして起動できます。
- **ブログ** — microCMSで管理する記事を一覧・詳細ページに表示します。

## 設計上の工夫

- **入力から振り返りまでをつなぐ設計** — 学習の開始、実績保存、カレンダーへの反映を一連の流れとして扱い、記録の手間を抑えています。
- **サーバー状態の一元管理** — TanStack Queryを使い、志望校、学習予定、実績、参考書の取得・更新とキャッシュを管理しています。
- **バリデーションの一元化** — `lib/validations/` のZodスキーマをクライアントとAPIで共有しています。フロント、API、データベースの各層で不正な入力や重複を防ぎます。
- **デモ環境の保護** — UIだけに依存せず、更新APIにも読み取り専用ガードを適用しています。
- **Infrastructure as Code** — VPC、EC2、RDS、セキュリティグループ、ECR、IAMをTerraformで管理しています。
- **自動テストとデプロイ** — Vitest、Playwright、GitHub Actionsを使い、検査からAWSへのデプロイまでを自動化しています。

## アーキテクチャ

```mermaid
flowchart LR
    User[利用者] --> Nginx[Nginx / HTTPS]
    Nginx --> App[Next.js 16\nDocker on EC2]
    App --> RDS[(Amazon RDS\nMySQL 8.4)]
    App --> Auth[Google / GitHub OAuth]
    App --> Resend[Resend]
    App --> CMS[microCMS]

    GitHub[GitHub Actions] -->|OIDC| ECR[Amazon ECR]
    GitHub -->|SSM Run Command| EC2[Amazon EC2]
    ECR --> EC2
    EC2 --> App
```

本番では、GitHub ActionsがDockerイメージをECRへpushします。デプロイ処理はSSM Run CommandでEC2上に実行し、EC2のIAMインスタンスロールを使ってイメージをpullします。SSHの22番ポートは公開していません。

## テックスタック

| カテゴリ | 技術 |
|---|---|
| フレームワーク | Next.js 16（App Router）/ React 19 |
| 言語 | TypeScript |
| UI | shadcn/ui / Tailwind CSS v4 / Motion |
| フォーム・検証 | React Hook Form / Zod |
| サーバー状態 | TanStack Query |
| カレンダー | FullCalendar / Schedule-X |
| ORM・DB | Prisma 7 / MySQL 8.4 |
| 認証 | Better Auth |
| メール | Resend |
| CMS | microCMS |
| テスト | Vitest / Playwright |
| コンテナ | Docker / Docker Compose |
| インフラ | AWS EC2 / RDS / ECR / Nginx / Systems Manager |
| IaC | Terraform |
| CI/CD | GitHub Actions / AWS OIDC |

## データソース

全国大学マスターには、[ASTI アマノ技研「国内大学の位置データ」](https://amano-tec.com/)を利用しています。国立・公立・私立の823校を取り込み、住所から都道府県と設置区分を整形しています。

変換処理は `scripts/transform-universities.ts`、整形済みデータは `data/clean/universities.json` にあります。

## ローカルセットアップ

### 前提条件

- Node.js 24
- npm
- Docker Desktopなど、Docker Composeを実行できる環境

OAuthログイン、メール送信、ブログまで確認する場合は、Google・GitHub OAuth、Resend、microCMSの資格情報も必要です。

### 初回起動

1. リポジトリをクローンします。

   ```bash
   git clone https://github.com/shimaiku1960/juken-map.git
   cd juken-map
   ```

2. 依存関係をインストールします。

   ```bash
   npm ci
   npm run hooks:install
   ```

   `hooks:install`は、依存ファイルを含むpushの前だけLinux環境でlockfileを確認するGitフックを有効にします。

3. 環境変数ファイルを作成します。

   ```bash
   cp .env.example .env
   ```

4. `.env` の各値を開発環境に合わせて変更します。

   [注意] `.env` には秘密情報が含まれます。Gitへコミットしないでください。

5. MySQLを起動します。

   ```bash
   npm run dev:infra
   ```

6. Prisma Clientを生成し、マイグレーションを適用します。

   ```bash
   npx prisma generate
   npx prisma migrate deploy
   ```

7. 大学マスターとデモデータを投入します。

   ```bash
   npx prisma db seed
   ```

8. 開発サーバーを起動します。

   ```bash
   npm run dev
   ```

9. [http://localhost:3000](http://localhost:3000)を開きます。

### 2回目以降の起動

次のコマンドだけで、MySQLの起動を待ってから開発サーバーを開始できます。

```bash
npm run dev
```

本番相当のDocker構成を確認する場合は、次のコマンドを使用します。

```bash
docker compose up --build
```

### 完了の確認

次の状態になれば、ローカルセットアップは完了です。

- `http://localhost:3000` でトップページが表示される
- ログイン画面からデモユーザーでログインできる
- ダッシュボードに学習予定や志望校が表示される

## 環境変数

設定項目は [.env.example](.env.example) を参照してください。

| 変数 | 用途 |
|---|---|
| `DATABASE_URL` | MySQLへの接続 |
| `BETTER_AUTH_SECRET` | セッションなどの署名 |
| `BETTER_AUTH_URL` | Better AuthのベースURL |
| `AUTH_GOOGLE_ID` / `AUTH_GOOGLE_SECRET` | Google OAuth |
| `AUTH_GITHUB_ID` / `AUTH_GITHUB_SECRET` | GitHub OAuth |
| `RESEND_API_KEY` | メール確認・パスワード再設定・各種メール通知 |
| `ADMIN_NOTIFICATION_EMAIL` | 新規ユーザー登録の通知先メールアドレス |
| `DAILY_NOTIFICATION_SECRET` | 朝・夜の学習通知APIを保護する秘密値（本番サーバーとGitHub Actionsで同じ値を設定） |
| `MICROCMS_API_KEY` / `MICROCMS_SERVICE_DOMAIN` | ブログ記事の取得 |

## npm scripts

| コマンド | 説明 |
|---|---|
| `npm run dev` | MySQLを起動し、開発サーバーを開始する |
| `npm run dev:infra` | MySQLコンテナを起動する |
| `npm run dev:infra:stop` | MySQLコンテナを停止する |
| `npm run dev:infra:logs` | MySQLコンテナのログを表示する |
| `npm run build` | Prisma生成、マイグレーション、本番ビルドを実行する |
| `npm run start` | 本番サーバーを起動する |
| `npm run lint` | ESLintを実行する |
| `npm run test` | Vitestを実行する |
| `npm run e2e` | PlaywrightのE2Eテストを実行する |
| `npm run check` | Prisma生成、Lint、型チェック、Vitest、ビルドをまとめて実行する |
| `npm run capture:seed` | LP撮影用ユーザーをローカルDBへ投入する |
| `npm run hooks:install` | リポジトリ管理のGitフックを有効にする |
| `npm run lock:check` | Linux環境でlockfileを非破壊検証する |
| `npm run lock:linux` | DockerのLinux環境でlockfileを更新し、`npm ci`まで検証する |
| `npm run lock:fix` | `lock:linux`の互換エイリアス |

## テストとCI

変更をpushする前に、次のコマンドで主要な検査をまとめて実行できます。

```bash
npm run check
```

`package.json`または`package-lock.json`を含むpushでは、pre-pushフックが`npm run lock:check`を自動実行します。不整合がある場合はpushを中止するため、`npm run lock:linux`でlockfileを更新してコミットしてください。通常のコード変更では、この追加検査は実行しません。

GitHub Actionsでは、次の3ジョブを実行します。

- `lockfile` — Linux向けネイティブ依存関係がlockfileに含まれるか確認する
- `check` — Lint、型チェック、Vitest、本番ビルドを実行する
- `e2e` — MySQLサービスコンテナ上でPlaywrightを実行する

## デプロイ

`main` ブランチへのpushを起点に、GitHub Actionsが次の順序で本番へデプロイします。

1. Dockerイメージをビルドします。
2. AWS OIDCで一時的な認証情報を取得します。
3. DockerイメージをAmazon ECRへpushします。
4. SSM Run CommandでEC2上のデプロイスクリプトを実行します。
5. EC2がIAMインスタンスロールでECRからイメージをpullします。
6. マイグレーションとコンテナの入れ替えを実行します。
7. スモークテストに失敗した場合は直前のイメージへ戻します。

デプロイジョブには `concurrency` を設定し、複数のデプロイが同時に本番環境を変更しないようにしています。

## 主なディレクトリ

```text
app/
├── api/                 # Route Handlers
├── articles/[id]/       # ブログ記事詳細
├── blog/                # ブログ一覧
├── components/          # 画面・機能コンポーネント
├── dashboard/           # 学習記録と振り返り
├── explore/             # 大学検索
├── goals/               # 志望校管理
├── hooks/               # TanStack Queryのカスタムフック
├── login/               # ログイン
├── profile/             # プロフィール
└── schedule/            # 学習予定

components/ui/           # 共通UIコンポーネント
e2e/                     # Playwright E2Eテスト
lib/                     # 認証、DB、検証、ドメインロジック
prisma/                  # スキーマ、マイグレーション、seed
scripts/                 # データ変換などの補助スクリプト
terraform/               # AWSインフラ定義
```

## 主要なデータモデル

- **User / Account / Session / Verification** — Better Authの認証データ
- **FinalGoal** — 志望校、第一志望、候補・受験校の状態
- **StudyPlan** — 日ごとの学習予定、科目、参考書、学習範囲
- **StudyLog** — 学習時間、到達範囲、メモ
- **Textbook / TextbookMaster** — ユーザーの参考書と参考書マスター
- **University / Faculty / Tag** — 大学、学部、学部系統タグのマスター
