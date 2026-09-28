#!/usr/bin/env bash
# デプロイスクリプトが DB の資格情報をどのコンテナへ渡すかを手元で試す（JUK-81）。
#
# aws・docker・nginx などを偽物に差し替えて deploy-ec2.sh を最後まで流し、
# docker run に渡った env ファイルの中身を見る。確かめたいのは次の3つ。
# - シークレットに2つそろっていれば、マイグレーション用はアプリより先の1回きりのコンテナにだけ渡り、
#   アプリのコンテナには .env の接続先ではなくアプリ用の DATABASE_URL が渡る
# - 2つとも無ければ、これまで通り .env の DATABASE_URL で起動する
# - 片方だけなら、何も起動せずに止まる
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY="$ROOT/.github/scripts/deploy-ec2.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir "$WORK/bin"
cat > "$WORK/bin/aws" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "secretsmanager get-secret-value" ]; then cat "$SECRET_FILE"; fi
EOF
# docker run は「run のあとの最後の引数（migrate なら migrate、アプリならイメージ）」と env ファイルの中身を記録する。
cat > "$WORK/bin/docker" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "inspect" ] && exit 1
[ "$1" = "run" ] || exit 0
env_file=""
prev=""
for arg in "$@"; do
  [ "$prev" = "--env-file" ] && env_file="$arg"
  prev="$arg"
done
{ echo "run:$prev"; sort "$env_file"; } >> "$LOG"
EOF
printf '#!/usr/bin/env bash\necho 200\n' > "$WORK/bin/curl"
for c in nginx systemctl sleep; do printf '#!/usr/bin/env bash\n' > "$WORK/bin/$c"; done
chmod +x "$WORK/bin/"*

printf 'DATABASE_URL=mysql://admin:ADMIN@rds/juken_map\nBETTER_AUTH_SECRET=s\n' > "$WORK/env"
printf 'upstream juken_map_app {\n    server 127.0.0.1:3000;\n}\n' > "$WORK/upstream.conf"
printf 'proxy_pass http://juken_map_app;\n' > "$WORK/site"

LINE='"LINE_CHANNEL_SECRET":"l","LINE_CHANNEL_ACCESS_TOKEN":"t"'
APP='"DATABASE_URL":"mysql://juken_app:APP@rds/juken_map"'
MIGRATE='"MIGRATION_DATABASE_URL":"mysql://juken_migrate:MIG@rds/juken_map"'

fail=0
check() {  # $1=見出し $2=シークレットの JSON $3=期待する記録
  printf '%s' "$2" > "$WORK/secret"
  : > "$WORK/log"
  local status=0
  # 本番と同じ配送（base64 → パイプ → bash -s）で流す。
  base64 < "$DEPLOY" | tr -d '\n' | base64 -d \
    | PATH="$WORK/bin:$PATH" LOG="$WORK/log" SECRET_FILE="$WORK/secret" ENV_FILE="$WORK/env" \
      UPSTREAM_CONF="$WORK/upstream.conf" SITE_CONF="$WORK/site" \
      bash -s -- dummy-tag "" > "$WORK/out" 2>&1 || status=$?
  local got
  got="$(cat "$WORK/log"; echo "exit=$status")"
  if [ "$got" = "$3" ]; then
    printf '  ✅ %s\n' "$1"
  else
    printf '  ❌ %s\n--- 期待\n%s\n--- 実際\n%s\n--- 出力\n' "$1" "$3" "$got"
    cat "$WORK/out"
    fail=1
  fi
}

echo "deploy-ec2.sh の DB の資格情報の渡し方:"

check "2つそろえば、マイグレーションを先に流し、アプリには .env の接続先を渡さない" "{$LINE,$APP,$MIGRATE}" "run:migrate
MIGRATION_DATABASE_URL=mysql://juken_migrate:MIG@rds/juken_map
run:961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map:dummy-tag
BETTER_AUTH_SECRET=s
DATABASE_URL=mysql://juken_app:APP@rds/juken_map
LINE_CHANNEL_ACCESS_TOKEN=t
LINE_CHANNEL_SECRET=l
SKIP_MIGRATIONS=1
exit=0"

check "2つとも無ければ、これまで通り .env の接続先で起動時に当てる" "{$LINE}" "run:961457613174.dkr.ecr.ap-northeast-1.amazonaws.com/juken-map:dummy-tag
BETTER_AUTH_SECRET=s
DATABASE_URL=mysql://admin:ADMIN@rds/juken_map
LINE_CHANNEL_ACCESS_TOKEN=t
LINE_CHANNEL_SECRET=l
exit=0"

check "アプリ用だけなら、何も起動せずに止まる" "{$LINE,$APP}" "exit=1"
check "マイグレーション用だけなら、何も起動せずに止まる" "{$LINE,$MIGRATE}" "exit=1"

exit "$fail"
