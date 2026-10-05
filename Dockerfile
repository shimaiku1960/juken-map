# 本番で動くアプリは Go（apps/api-go）だけで、このイメージはサーバーを持たない（JUK-121）。役目は2つ。
#   - 画面のビルド成果物を /app/web に置く。Go のイメージが --build-context web= でここから写す（deploy.yml）
#   - デプロイのたびにマイグレーションを当てる1回きりのコンテナ（docker-entrypoint.sh）
FROM node:24-slim AS base
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# ローカル・CI・DockerでpackageManagerの固定バージョンを共有する。
RUN npm install --global "$(node -p "require('./package.json').packageManager")"
COPY apps/api/package.json ./apps/api/package.json
COPY apps/web/package.json ./apps/web/package.json

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

# ---- マイグレーションが使う依存だけをインストールする ----
FROM base AS api-deps
# tsxを実行時にも使うため、devDependenciesは維持する。
RUN pnpm --filter @juken-map/api install --frozen-lockfile

# ---- 実行 ----
FROM node:24-slim AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN groupadd --system --gid 1001 nodejs \
  && useradd --system --uid 1001 --gid nodejs app

# pnpmの実体（.pnpm）と各パッケージの相対リンクを同じ配置でコピーする。
COPY --from=api-deps --chown=app:nodejs /app/node_modules ./node_modules
COPY --from=api-deps --chown=app:nodejs /app/apps/api/node_modules ./apps/api/node_modules
COPY --chown=app:nodejs package.json ./package.json
COPY --chown=app:nodejs db/migrations ./db/migrations
COPY --chown=app:nodejs apps/api ./apps/api
COPY --from=web-builder --chown=app:nodejs /app/apps/web/dist ./web
COPY --chown=app:nodejs docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./docker-entrypoint.sh
USER app
ENTRYPOINT ["./docker-entrypoint.sh"]
