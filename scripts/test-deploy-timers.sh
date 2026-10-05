#!/usr/bin/env bash
# デプロイスクリプトが、毎日の通知のタイマー（JUK-85、infra/systemd/）をどう入れるかを手元で試す。
#
# aws・docker・systemctl などを偽物に差し替えて deploy-ec2.sh を最後まで流し、置かれたファイルと
# systemctl に何をさせたかを見る。確かめたいのは次のこと。
# - タイマーとサービスの定義を置き、共有トークンを root だけが読めるファイル（600）に書き、タイマーを有効にする
# - 共有トークンが無ければ、ファイルを書かずにタイマーを止める
# - 定義を渡さなければ、タイマーには触らない
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
# スモークテストが通る形（Go のダッシュボードは Cookie 無しなので 401、それ以外は 200）。
cat > "$WORK/bin/curl" <<'EOF'
#!/usr/bin/env bash
case "${*: -1}" in
  */api/dashboard) echo 401 ;;
  *) echo 200 ;;
esac
EOF
# systemctl はタイマーに関わる呼び出しだけを記録する（nginx の reload は記録しない）。
cat > "$WORK/bin/systemctl" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *daily-notification*|daemon-reload) echo "systemctl $*" >> "$LOG" ;;
esac
exit 0
EOF
for c in nginx sleep; do printf '#!/usr/bin/env bash\n' > "$WORK/bin/$c"; done
chmod +x "$WORK/bin/"*

printf 'location / {\n    proxy_pass http://juken_map_app;\n}\n' > "$WORK/site"
# リポジトリの定義をそのまま使う（deploy.yml と同じまとめ方）。Mac の tar が ._ のファイルを混ぜないようにする。
UNITS_B64="$(COPYFILE_DISABLE=1 tar -C "$ROOT/infra/systemd" -cz . | base64 | tr -d '\n')"

fail=0
ok() { printf '  ✅ %s\n' "$1"; }
ng() { printf '  ❌ %s\n%s\n--- 出力\n' "$1" "$2"; cat "$WORK/out"; fail=1; }

# run_deploy .env の中身 定義の base64 → deploy-ec2.sh を本番と同じ配送（base64 → パイプ → bash -s）で流す。
run_deploy() {
  printf '%s\n' "$1" > "$WORK/env"
  rm -rf "$WORK/systemd" "$WORK/etc"
  : > "$WORK/log"
  status=0
  base64 < "$DEPLOY" | tr -d '\n' | base64 -d \
    | PATH="$WORK/bin:$PATH" LOG="$WORK/log" ENV_FILE="$WORK/env" \
      UPSTREAM_CONF="$WORK/upstream.conf" GO_UPSTREAM_CONF="$WORK/go-upstream.conf" \
      GO_ROUTES_CONF="$WORK/routes/go-routes.conf" SITE_CONF="$WORK/site" NGINX_LOGROTATE="$WORK/logrotate-nginx" \
      SYSTEMD_DIR="$WORK/systemd" NOTIFY_ENV_FILE="$WORK/etc/juken-map/daily-notification.env" \
      bash -s -- dummy-tag "" "" "${2:-}" > "$WORK/out" 2>&1 || status=$?
}

# mode ファイル → 8進の権限（Mac と Linux で stat の書き方が違う）。
mode() { stat -c %a "$1" 2>/dev/null || stat -f %Lp "$1"; }

BASE_ENV='DATABASE_URL=mysql://app@rds/juken_map
BETTER_AUTH_SECRET=s'
TIMERS="juken-map-daily-notification-morning.timer juken-map-daily-notification-evening.timer"

echo "deploy-ec2.sh の毎日の通知のタイマー:"

run_deploy "$BASE_ENV
DAILY_NOTIFICATION_SECRET=d" "$UNITS_B64"
installed="$(cd "$WORK/systemd" 2>/dev/null && ls | tr '\n' ' ')"
if [ "$status" = 0 ] \
  && [ "$installed" = "juken-map-daily-notification-evening.timer juken-map-daily-notification-morning.timer juken-map-daily-notification@.service " ] \
  && [ "$(cat "$WORK/log")" = "systemctl daemon-reload
systemctl enable --now $TIMERS" ]; then
  ok "定義を置き、daemon-reload してからタイマーを有効にする"
else
  ng "タイマーを入れる" "exit=$status
置いたもの: $installed
$(cat "$WORK/log")"
fi
envfile="$WORK/etc/juken-map/daily-notification.env"
if [ "$(cat "$envfile" 2>/dev/null)" = "DAILY_NOTIFICATION_SECRET=d" ] && [ "$(mode "$envfile")" = 600 ] \
  && [ "$(mode "$(dirname "$envfile")")" = 700 ] && [ ! -e "$envfile.tmp" ]; then
  ok "共有トークンは root だけが読めるファイル（600、置き場は 700）に書く"
else
  ng "共有トークンのファイル" "$(ls -la "$(dirname "$envfile")" 2>&1)"
fi
if cmp -s "$WORK/systemd/juken-map-daily-notification@.service" "$ROOT/infra/systemd/juken-map-daily-notification@.service"; then
  ok "置いた定義はリポジトリの infra/systemd/ と同じ中身"
else
  ng "定義の中身" "$(diff "$ROOT/infra/systemd/juken-map-daily-notification@.service" "$WORK/systemd/juken-map-daily-notification@.service")"
fi

run_deploy "$BASE_ENV" "$UNITS_B64"
if [ "$status" = 0 ] && [ ! -e "$envfile" ] && [ "$(cat "$WORK/log")" = "systemctl disable --now $TIMERS" ]; then
  ok "共有トークンが無ければ、ファイルを書かずにタイマーを止める"
else
  ng "共有トークン無し" "exit=$status
$(cat "$WORK/log")"
fi

run_deploy "$BASE_ENV
DAILY_NOTIFICATION_SECRET=d" ""
if [ "$status" = 0 ] && [ ! -s "$WORK/log" ] && [ ! -e "$WORK/systemd" ]; then
  ok "定義を渡さなければ、タイマーには触らない"
else
  ng "定義無し" "exit=$status
$(cat "$WORK/log")"
fi

exit "$fail"
