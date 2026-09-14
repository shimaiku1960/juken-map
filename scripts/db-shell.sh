#!/usr/bin/env bash
# ローカルDB（docker compose の db サービス）の MySQL 対話画面を開く。
# 接続情報は docker-compose.yml がコンテナへ渡している MYSQL_USER / MYSQL_PASSWORD /
# MYSQL_DATABASE をコンテナの中で読むので、ここにはパスワードを書かない。
#
#   pnpm db:shell                          対話画面を開く（exit で終了）
#   pnpm db:shell -e "SHOW TABLES"         SQLを1本だけ実行して終わる
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

# パスワードはコマンド引数（-p）にも環境変数（MYSQL_PWD、8.4で非推奨）にも載せず、
# その場で作った設定ファイル（[client] セクション）として mysql に渡す。
# シングルクォートの中身はコンテナ内の bash が展開する。
# 文字コードは mysql の既定（auto）だとコンテナのロケール（未設定）から latin1 になり、
# 日本語が ??? に化けるので utf8mb4 を指定する。
docker compose exec db bash -c '
  exec mysql \
    --defaults-extra-file=<(printf "[client]\nuser=%s\npassword=%s\n" "$MYSQL_USER" "$MYSQL_PASSWORD") \
    --default-character-set=utf8mb4 \
    "$MYSQL_DATABASE" "$@"
' bash "$@"
