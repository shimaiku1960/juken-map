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

# ---- 実行 ----
# distroless の static は、シェルもパッケージ管理も無い最小のイメージ（CA 証明書と時間帯のデータだけ入っている）。
# 攻撃に使える道具が中に無い。nonroot は UID 65532 で動く。
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api-go /api-go
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/api-go"]
