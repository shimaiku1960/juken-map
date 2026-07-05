# 受験マップ

大学受験生のための志望校・併願校管理アプリ。全国の大学マスターから志望校を選んで管理し、受験日程をカレンダーで俯瞰、併願校を条件で探せます。

🌐 **本番環境**: https://juken-map.com （AWS EC2 上でセルフホスト）

## テックスタック

| カテゴリ | 技術 |
|---------|------|
| フレームワーク | Next.js 16 (App Router) |
| UI | React 19 / shadcn/ui |
| 言語 | TypeScript |
| スタイリング | Tailwind CSS v4 |
| フォーム | React Hook Form + Zod |
| サーバー状態管理 | TanStack Query |
| カレンダー | Schedule-X |
| CMS | microCMS |
| ORM | Prisma 7 |
| データベース | MySQL 8.4（ローカル: Docker / 本番: AWS RDS） |
| 認証 | Better Auth（Google / GitHub / メール+パスワード） |
| メール送信 | Resend（メール検証・パスワードリセット） |
| コンテナ | Docker（マルチステージ / standalone / 本番はコンテナ運用） |
| IaC | Terraform（AWS リソースをコード管理） |
| インフラ | AWS EC2 (Ubuntu) / Nginx / RDS / ECR |
| CI/CD | GitHub Actions（OIDC 認証 / ECR ビルド → 自動デプロイ） |

## 主な機能

- **志望校管理** — 全国の大学・学部マスターから志望校を選択。第一志望／併願を分けて管理し、同一学部の重複登録を多層防御（フロント／API／DB の3層）で防止
- **大学を探す** — 全国 823 大学のマスターから、大学名・都道府県・設置区分で絞り込み。学部詳細では学部系統タグを表示し、その場で志望校に追加
- **受験日程カレンダー** — 登録した志望校の受験日を Schedule-X で月表示。ダッシュボードで一覧
- **ユーザー認証** — Better Auth による Google / GitHub / メール+パスワードの3方式ログイン。メール検証・パスワードリセット（Resend 経由）、ルート保護に対応
- **プロフィール管理** — ニックネームの表示・編集
- **ブログ** — microCMS で管理する記事の一覧・詳細表示

## 設計上の工夫

- **サーバー状態の一元管理** — 志望校データを TanStack Query で `["goals"]` キャッシュに集約。複数コンポーネント（志望校一覧／大学を探す）で共有・自動同期し、取得ロジックは `useGoals` カスタムフックに集約
- **バリデーションの single source of truth** — Zod スキーマ（`lib/validations`）をサーバー／クライアントで共通利用。フロント（UX）・API（門番）・DB（最後の砦）の多層防御
- **Infrastructure as Code** — 本番の VPC / EC2 / RDS / セキュリティグループ / ECR / IAM を Terraform で import しコード管理（`terraform/`）
- **コンテナ化 / デプロイ自動化** — マルチステージ Dockerfile（`output: "standalone"` / 非 root 実行 / 起動時マイグレーション）。CI でビルドしたイメージを ECR に push し、本番 EC2 が IAM インスタンスロール経由で pull して `docker run`。`main` への push で「ビルド → ECR push → EC2 でコンテナ入れ替え」まで一気通貫の自動デプロイ

## データソース

全国大学マスターは [ASTI アマノ技研「国内大学の位置データ」](https://amano-tec.com/)（商用利用無料）を利用。国立・公立・私立の 823 校を取り込み、住所から都道府県・設置区分を整形して投入しています（`scripts/transform-universities.ts`）。

## セットアップ

### 前提条件

- Node.js 20 以上
- Docker（ローカル DB = MySQL 8.4 を起動）
- microCMS アカウント（ブログ管理用）
- 各 OAuth / メール送信の資格情報（Google / GitHub OAuth、Resend）

### 環境変数

プロジェクトルートに `.env` を作成し、以下を設定します：

```
# DB（ローカルは Docker の MySQL）
DATABASE_URL="mysql://juken:jukenpassword@localhost:3306/juken_map"

# Better Auth
BETTER_AUTH_SECRET="your-random-secret"
BETTER_AUTH_URL="http://localhost:3000"

# OAuth プロバイダ
AUTH_GOOGLE_ID="your-google-client-id"
AUTH_GOOGLE_SECRET="your-google-client-secret"
AUTH_GITHUB_ID="your-github-client-id"
AUTH_GITHUB_SECRET="your-github-client-secret"

# メール送信（メール検証・リセット）
RESEND_API_KEY="your-resend-api-key"

# microCMS（ブログ）
MICROCMS_API_KEY="your-microcms-api-key"
MICROCMS_SERVICE_DOMAIN="your-service-domain"
```

### インストールと起動

```bash
# ローカル DB（MySQL）を Docker で起動
docker compose up -d db

# 依存関係のインストール
npm install

# Prisma Client の生成
npx prisma generate

# データベースマイグレーション
npx prisma migrate deploy

# 大学マスターなどの初期データ投入
npx prisma db seed

# 開発サーバーの起動
npm run dev
```

http://localhost:3000 でアクセスできます。

> ローカル開発はアプリを `npm run dev`、DB のみ Docker コンテナで動かす構成です。本番相当（standalone ビルド）で確認したいときは `docker compose up --build`（アプリも含めてコンテナ起動）を使います。

## npm scripts

| コマンド | 説明 |
|---------|------|
| `npm run dev` | 開発サーバー起動 |
| `npm run build` | Prisma 生成 + マイグレーション + 本番ビルド |
| `npm run start` | 本番サーバー起動 |
| `npm run lint` | ESLint によるコード検査 |

## デプロイ / インフラ（AWS）

本番環境は Vercel ではなく AWS 上にセルフホストしています（インフラ学習目的）。

- **EC2 (Ubuntu / t3.micro)** 上で **Docker コンテナ**としてアプリを常駐（`--restart always` で自動復旧）、**Nginx** をリバースプロキシに（443 → コンテナの 3000）
- **RDS (MySQL 8.4)** をデータベースに使用（VPC 内からのみアクセス可）
- **GitHub Actions** で `main` への push をトリガーに、**ビルド → ECR push → EC2 で pull & コンテナ入れ替え**を自動実行（1 ワークフローに統合し `needs` で順序を保証）
  - ビルド機（Actions ランナー）から ECR への push は **OIDC 認証**、実行機（EC2）から ECR の pull は **IAM インスタンスロール**。いずれもアクセスキーを持たせない短命トークン方式
- 本番インフラ（VPC / EC2 / RDS / セキュリティグループ / ECR / IAM）は **Terraform** でコード管理

## プロジェクト構成

```
app/
├── page.tsx              # ダッシュボード（受験日程カレンダー）
├── explore/              # 大学を探す
│   ├── page.tsx          # 大学一覧（絞り込み検索）
│   └── [universityId]/   # 大学詳細（学部一覧・志望校追加）
├── goals/                # 志望校管理
├── profile/              # プロフィール
├── blog/ articles/[id]/  # ブログ一覧・記事詳細（microCMS）
├── login/ signup/        # 認証ページ
├── forgot-password/ reset-password/  # パスワードリセット
├── api/
│   ├── auth/[...all]/    # Better Auth ハンドラ
│   ├── goals/            # 志望校 CRUD
│   └── profile/          # プロフィール更新
├── hooks/useGoals.ts     # 志望校のサーバー状態フック（TanStack Query）
├── providers.tsx         # QueryClientProvider
└── components/           # 画面コンポーネント

lib/
├── auth.ts / auth-client.ts  # Better Auth（サーバー / クライアント）
├── email.ts / resend.ts      # メール送信（Resend）
├── microcms.ts               # microCMS クライアント
├── prisma.ts                 # Prisma クライアント
└── validations/              # Zod スキーマ（サーバー/クライアント共通）

prisma/
├── schema.prisma         # データベーススキーマ
├── seed.ts               # 初期データ投入
└── migrations/           # マイグレーション

terraform/                # AWS リソースの IaC（VPC/EC2/RDS/SG/ECR/IAM）

Dockerfile                # マルチステージ（standalone / 非 root / 起動時マイグレーション）
docker-compose.yml        # ローカル DB（+ 本番相当のアプリ起動）
scripts/                  # 大学マスター整形スクリプト
```

## データベースモデル（主要）

- **User / Account / Session / Verification** — Better Auth の認証モデル（ユーザー情報を自前 MySQL で保持）
- **FinalGoal** — 志望校（User × Faculty。`@@unique` で重複防止、第一志望フラグを保持）
- **University** — 大学マスター（名称・都道府県・設置区分）
- **Faculty** — 学部マスター（受験日を保持。University にリレーション）
- **Tag** — 学部系統タグ（Faculty と多対多）
