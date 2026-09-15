# SPAとAPIを1つのイメージにまとめ、Fastifyが両方を配る。
FROM node:24-slim AS base
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
# ローカル・CI・DockerでpackageManagerの固定バージョンを共有する。
RUN npm install --global "$(node -p "require('./package.json').packageManager")"
COPY apps/api/package.json ./apps/api/package.json
COPY apps/web/package.json ./apps/web/package.json

# ---- SPAをビルドする（型検査も含む） ----
FROM base AS web-builder
RUN pnpm install --frozen-lockfile
COPY src/shared ./src/shared
COPY apps/web ./apps/web
RUN pnpm --filter @juken-map/web build

# ---- APIと共有コードが使う依存だけをインストールする ----
FROM base AS api-deps
# src/sharedはルートの依存を解決するため、ルートも対象に含める。
# tsxを実行時にも使うため、devDependenciesは維持する。
RUN pnpm --filter juken-map --filter @juken-map/api install --frozen-lockfile

# ---- 実行 ----
FROM node:24-slim AS runner
WORKDIR /app
ENV NODE_ENV=production
ENV API_PORT=3000
ENV WEB_DIST_DIR=/app/web
RUN groupadd --system --gid 1001 nodejs \
  && useradd --system --uid 1001 --gid nodejs app

# pnpmの実体（.pnpm）と各パッケージの相対リンクを同じ配置でコピーする。
COPY --from=api-deps --chown=app:nodejs /app/node_modules ./node_modules
COPY --from=api-deps --chown=app:nodejs /app/apps/api/node_modules ./apps/api/node_modules
COPY --chown=app:nodejs package.json ./package.json
# マイグレーション（db/migrations）はコンテナの起動時に当てる（docker-entrypoint.sh）。
COPY --chown=app:nodejs db/migrations ./db/migrations
COPY --chown=app:nodejs apps/api ./apps/api
COPY --chown=app:nodejs src/shared ./src/shared
COPY --from=web-builder --chown=app:nodejs /app/apps/web/dist ./web
COPY --chown=app:nodejs docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./docker-entrypoint.sh
USER app
EXPOSE 3000
ENTRYPOINT ["./docker-entrypoint.sh"]
