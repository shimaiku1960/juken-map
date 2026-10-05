# Go の API（apps/api-go）のイメージ。ビルドの文脈はこのディレクトリ（docker build apps/api-go）。
# Node のイメージ（ルートの Dockerfile）とは別に作り、本番では別のコンテナとして並べて動かす（JUK-72）。

# ---- ビルド ----
FROM golang:1.27.1-bookworm AS build
WORKDIR /src
# 依存を先に取っておくと、コードだけ変えたときはこの層がキャッシュから使われる。
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED=0 で C のライブラリに頼らない1つの実行ファイルにする（下の static イメージで動く）。
# -trimpath はビルドした PC のパスを実行ファイルに残さない。-s -w はデバッグ情報を省いて小さくする。
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api-go .

# ---- 画面のビルド成果物 ----
# Go が画面（SPA・SSG の HTML）も配る（JUK-111）。画面のビルドは Node のイメージ（ルートの Dockerfile の web-builder）が
# 記事の SSG まで済ませているので、作り直さずにそれを写す。本番のデプロイは
#   docker build --build-context web=docker-image://<Node のイメージ> apps/api-go
# で、この web を Node のイメージ（/app/web に dist が入っている）に差し替える。差し替えずに作ると中身が空で、
# Go は画面を配らない（API だけ。CI のビルドの確認と、手元で API だけ動かすとき）。
FROM build AS web
RUN mkdir -p /app/web

# ---- 実行 ----
# distroless の static は、シェルもパッケージ管理も無い最小のイメージ（CA 証明書と時間帯のデータだけ入っている）。
# 攻撃に使える道具が中に無い。nonroot は UID 65532 で動く。
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api-go /api-go
COPY --from=web /app/web /web
ENV PORT=8080
ENV WEB_DIST_DIR=/web
# イメージを作ったコミット（git の SHA）。/api/health が返し、デプロイ後の E2E が本番で新しい版が
# 動いているかを見る（JUK-101）。値が毎回変わるので、ビルドのキャッシュを壊さないよう最後に置く。
ARG APP_COMMIT=""
ENV APP_COMMIT=$APP_COMMIT
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api-go"]
