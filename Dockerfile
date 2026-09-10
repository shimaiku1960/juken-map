# SPA（apps/web）と API（apps/api）を1つのイメージにまとめ、Fastify が両方を配る。
# nginx は :3000 へ丸ごと流すだけなので、本番ホストの設定を変えずに入れ替えられる。

# ---- SPA をビルドする ----
FROM node:24-slim AS web-builder
WORKDIR /app

COPY apps/web/package.json apps/web/package-lock.json ./apps/web/
RUN npm ci --prefix apps/web

# apps/web は @/shared 経由でリポジトリ直下の src/shared を読む。
COPY src/shared ./src/shared
COPY apps/web ./apps/web

# src/shared は zod を使うが、/app/src からの解決は /app/node_modules までしか辿らず
# apps/web/node_modules に届かない（ローカルではリポジトリ直下の node_modules が拾っていた）。
# npm workspaces を使っていないので、同じ効果を symlink で作る。
RUN ln -s /app/apps/web/node_modules /app/node_modules
# apps/web の build スクリプトは tsc --noEmit を伴うが、型検査はリポジトリ直下の
# node_modules（vitest / zod の型）に依存しており、ここには無い。型検査は CI の役目なので
# イメージのビルドでは vite build だけを走らせる。
RUN cd apps/web && npx vite build

# ---- API の依存と Prisma Client を用意する ----
FROM node:24-slim AS api-deps
WORKDIR /app

RUN apt-get update && apt-get install -y openssl && rm -rf /var/lib/apt/lists/*

COPY apps/api/package.json apps/api/package-lock.json ./apps/api/
RUN npm ci --prefix apps/api

# Prisma Client は schema の output 指定により /app/app/generated/prisma へ出る。
COPY prisma ./prisma
COPY apps/api/prisma.config.ts ./apps/api/prisma.config.ts
RUN npm run prisma:generate --prefix apps/api

# ---- 実行 ----
FROM node:24-slim AS runner
WORKDIR /app

RUN apt-get update && apt-get install -y openssl && rm -rf /var/lib/apt/lists/*

ENV NODE_ENV=production
# nginx が :3000 へ流す既存構成に合わせる。ここを変えると本番の nginx 変更が要る。
ENV API_PORT=3000
ENV WEB_DIST_DIR=/app/web

RUN groupadd --system --gid 1001 nodejs \
  && useradd --system --uid 1001 --gid nodejs app

# tsx と prisma CLI を実行時にも使うため、devDependencies を含めたまま渡す。
COPY --from=api-deps --chown=app:nodejs /app/apps/api/node_modules ./apps/api/node_modules
COPY --from=api-deps --chown=app:nodejs /app/app/generated ./app/generated
COPY --chown=app:nodejs prisma ./prisma
COPY --chown=app:nodejs apps/api ./apps/api
# apps/api は @/backend と @/shared 経由でリポジトリ直下を読む（tsconfig の paths）。
COPY --chown=app:nodejs src/backend ./src/backend
COPY --chown=app:nodejs src/shared ./src/shared
COPY --from=web-builder --chown=app:nodejs /app/apps/web/dist ./web

# 生成された Prisma Client は /app/app/generated 配下にあり、そこからの解決は
# /app/node_modules までしか辿らず apps/api/node_modules に届かない。web-builder と
# 同じ理由（npm workspaces を使っていない）なので、同じく symlink で橋渡しする。
RUN ln -s /app/apps/api/node_modules /app/node_modules

COPY --chown=app:nodejs docker-entrypoint.sh ./docker-entrypoint.sh
RUN chmod +x ./docker-entrypoint.sh

USER app

EXPOSE 3000

ENTRYPOINT ["./docker-entrypoint.sh"]
