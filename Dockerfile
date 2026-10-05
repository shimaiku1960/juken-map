# 画面（apps/web）をビルドするだけのイメージ（JUK-121・JUK-125）。本番で動くのは Go（apps/api）だけで、
# Go のイメージが --build-context web= で、ここの /app/web を写す（deploy.yml）。このイメージは実行しない。
FROM node:24-slim AS base
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# ローカル・CI・DockerでpackageManagerの固定バージョンを共有する。
RUN npm install --global "$(node -p "require('./package.json').packageManager")"
COPY apps/web/package.json ./apps/web/package.json
COPY db/package.json ./db/package.json

# ---- SPAをビルドする（型検査も含む） ----
# ブログの記事もここで microCMS から取って SSG する（JUK-110）。本番のデプロイは
# SSG_ARTICLES=required を渡し、記事を取れなければビルドを止める。API キーは BuildKit の secret で渡し、
# イメージの層にも履歴にも残さない（サービスのドメインは秘密ではないので build-arg）。
FROM base AS web-builder
ARG MICROCMS_SERVICE_DOMAIN
ARG SSG_ARTICLES
RUN pnpm install --frozen-lockfile
COPY src/shared ./src/shared
COPY apps/web ./apps/web
RUN --mount=type=secret,id=microcms_api_key \
  MICROCMS_API_KEY="$(cat /run/secrets/microcms_api_key 2>/dev/null || true)" \
  pnpm --filter @juken-map/web build

# ---- 画面のビルド成果物だけを残す ----
# 実行するものが無いので、中身は /app/web だけにする（ECR に置く量も減る）。
FROM scratch AS web
COPY --from=web-builder /app/apps/web/dist /app/web
