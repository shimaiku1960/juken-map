#!/usr/bin/env bash
# デプロイスクリプトが nginx のログの回し方（/etc/logrotate.d/nginx、JUK-128）をどう直すかを手元で試す。
#
# aws・docker・logrotate などを偽物に差し替えて deploy-ec2.sh を最後まで流し、設定ファイルがどうなったかを見る。
# 確かめたいのは次のこと。
# - Ubuntu の既定の設定を、空の日も回し（ifempty）、回したものを14日で消す（maxage 14）形に直す
# - 2回流しても同じ（maxage を重ねて足さない）
# - logrotate が読めない形になったら元に戻し、デプロイを失敗にする
# - 設定ファイルが無ければ触らない
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEPLOY="$ROOT/.github/scripts/deploy-ec2.sh"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

mkdir "$WORK/bin"
cat > "$WORK/bin/aws" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "secretsmanager get-secret-value" ]; then
  echo '{"LINE_CHANNEL_SECRET":"l","LINE_CHANNEL_ACCESS_TOKEN":"t"}'
fi
EOF
cat > "$WORK/bin/docker" <<'EOF'
#!/usr/bin/env bash
[ "$1" = inspect ] && exit 1
exit 0
EOF
cat > "$WORK/bin/curl" <<'EOF'
#!/usr/bin/env bash
case "${*: -1}" in
  */api/dashboard) echo 401 ;;
  *) echo 200 ;;
esac
EOF
# LOGROTATE_FAIL=1 なら、設定を読めなかったことにする。
cat > "$WORK/bin/logrotate" <<'EOF'
#!/usr/bin/env bash
[ "${LOGROTATE_FAIL:-}" = 1 ] && { echo "error: bad line" >&2; exit 1; }
exit 0
EOF
for c in nginx sleep systemctl; do printf '#!/usr/bin/env bash\n' > "$WORK/bin/$c"; done
chmod +x "$WORK/bin/"*

printf 'location / {\n    proxy_pass http://juken_map_app;\n}\n' > "$WORK/site"
printf 'DATABASE_URL=mysql://app@rds/juken_map\nBETTER_AUTH_SECRET=s\n' > "$WORK/env"

# 2026-10-05 に本番の EC2 で読んだ /etc/logrotate.d/nginx（Ubuntu の nginx パッケージの既定）。
TAB="$(printf '\t')"
DEFAULT="/var/log/nginx/*.log {
${TAB}daily
${TAB}missingok
${TAB}rotate 14
${TAB}compress
${TAB}delaycompress
${TAB}notifempty
${TAB}create 0640 www-data adm
${TAB}sharedscripts
${TAB}prerotate
${TAB}${TAB}if [ -d /etc/logrotate.d/httpd-prerotate ]; then \\
${TAB}${TAB}${TAB}run-parts /etc/logrotate.d/httpd-prerotate; \\
${TAB}${TAB}fi \\
${TAB}endscript
${TAB}postrotate
${TAB}${TAB}invoke-rc.d nginx rotate >/dev/null 2>&1
${TAB}endscript
}"
EXPECTED="${DEFAULT/${TAB}rotate 14/${TAB}rotate 14
${TAB}maxage 14}"
EXPECTED="${EXPECTED/${TAB}notifempty/${TAB}ifempty}"

fail=0
ok() { printf '  ✅ %s\n' "$1"; }
ng() { printf '  ❌ %s\n%s\n--- 出力\n' "$1" "$2"; cat "$WORK/out"; fail=1; }

# run_deploy → deploy-ec2.sh を本番と同じ配送（base64 → パイプ → bash -s）で流す。
run_deploy() {
  status=0
  base64 < "$DEPLOY" | tr -d '\n' | base64 -d \
    | PATH="$WORK/bin:$PATH" ENV_FILE="$WORK/env" \
      UPSTREAM_CONF="$WORK/upstream.conf" GO_UPSTREAM_CONF="$WORK/go-upstream.conf" \
      GO_ROUTES_CONF="$WORK/routes/go-routes.conf" SITE_CONF="$WORK/site" \
      NGINX_LOGROTATE="$WORK/logrotate.d/nginx" \
      bash -s -- dummy-tag "" "" "" > "$WORK/out" 2>&1 || status=$?
}

echo "deploy-ec2.sh の nginx のログの保存期間:"

mkdir "$WORK/logrotate.d"
printf '%s\n' "$DEFAULT" > "$WORK/logrotate.d/nginx"
run_deploy
if [ "$status" = 0 ] && [ "$(cat "$WORK/logrotate.d/nginx")" = "$EXPECTED" ] && [ "$(ls "$WORK/logrotate.d")" = nginx ]; then
  ok "既定の設定を、空の日も回し14日で消す形に直す（ほかのファイルを置かない）"
else
  ng "既定の設定を直す" "exit=$status
$(diff <(printf '%s\n' "$EXPECTED") "$WORK/logrotate.d/nginx")
$(ls "$WORK/logrotate.d")"
fi

run_deploy
if [ "$status" = 0 ] && [ "$(cat "$WORK/logrotate.d/nginx")" = "$EXPECTED" ]; then
  ok "2回流しても同じ（maxage を重ねて足さない）"
else
  ng "2回目" "exit=$status
$(diff <(printf '%s\n' "$EXPECTED") "$WORK/logrotate.d/nginx")"
fi

printf '%s\n' "$DEFAULT" > "$WORK/logrotate.d/nginx"
LOGROTATE_FAIL=1 run_deploy
if [ "$status" != 0 ] && [ "$(cat "$WORK/logrotate.d/nginx")" = "$DEFAULT" ]; then
  ok "logrotate が読めなければ元に戻し、デプロイを失敗にする"
else
  ng "読めないとき" "exit=$status
$(diff <(printf '%s\n' "$DEFAULT") "$WORK/logrotate.d/nginx")"
fi

rm -rf "$WORK/logrotate.d"
run_deploy
if [ "$status" = 0 ] && [ ! -e "$WORK/logrotate.d" ]; then
  ok "設定ファイルが無ければ触らない"
else
  ng "設定ファイル無し" "exit=$status"
fi

exit "$fail"
