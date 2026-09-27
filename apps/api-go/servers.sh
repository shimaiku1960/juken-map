# Node と Go を手元で並べて動かすための関数。compare-cpu.sh と parity.sh が source して使う。
# 呼ぶ側で ROOT（worktree のルート）と WORK（一時ディレクトリ）を決め、終了時に stop_servers を呼ぶこと。

server_pids=()

stop_servers() {
  for pid in "${server_pids[@]}"; do kill "$pid" 2>/dev/null || true; done
  wait 2>/dev/null || true
}

# ensure_free ポート... → どれかが使用中なら止める。
ensure_free() {
  local port
  for port in "$@"; do
    if lsof -ti tcp:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      echo "ポート $port が使用中です。環境変数でポートを変えてください。" >&2
      exit 1
    fi
  done
}

# issue_cookies 人数 → 合成ユーザーのセッション Cookie を1行ずつ出す。
# ログインは HTTP で通さず、DB に直接発行する（load-tests と同じ。db/issue-loadtest-sessions.ts）。
issue_cookies() {
  (cd "$ROOT" && COUNT=$1 pnpm exec tsx --env-file=.env db/issue-loadtest-sessions.ts 2>/dev/null | tail -1 | jq -r '.cookies[]')
}

# start_servers Nodeのポート Goのポート → 両方を起動し、/api/health が返るまで待つ。
# 起動後は node_pid と go_pid に、実際に待ち受けているプロセスの PID が入る
# （pnpm や tsx を挟むと、$! はラッパーの PID になるため）。
start_servers() {
  local node_port=$1 go_port=$2

  echo "Go をビルドしています…"
  (cd "$ROOT/apps/api-go" && go build -o "$WORK/api-go" .)

  # 本番の Node は SPA も配っていて、そのときだけ /api/* の 404 が JSON のエラー応答になる
  # （apps/api/src/spa.ts の setNotFoundHandler）。SPA のビルドは要らないので、
  # 空の index.html だけを置いて本番と同じ形で起動する。
  mkdir -p "$WORK/web"
  echo '<!doctype html><title>stub</title>' >"$WORK/web/index.html"

  (cd "$ROOT" && NODE_ENV=production API_PORT=$node_port BETTER_AUTH_URL=http://localhost:$node_port \
    WEB_DIST_DIR="$WORK/web" pnpm --filter @juken-map/api start >"$WORK/node.log" 2>&1) &
  server_pids+=($!)
  # NODE_ENV=production は Node と同じく reqId を UUID にするため。
  (set -a; source "$ROOT/.env"; set +a; NODE_ENV=production PORT=$go_port "$WORK/api-go" >"$WORK/go.log" 2>&1) &
  server_pids+=($!)

  local _
  for _ in $(seq 1 120); do
    curl -sf -o /dev/null "localhost:$node_port/api/health" && curl -sf -o /dev/null "localhost:$go_port/api/health" && break
    sleep 0.5
  done
  node_pid=$(lsof -ti tcp:"$node_port" -sTCP:LISTEN | head -1)
  go_pid=$(lsof -ti tcp:"$go_port" -sTCP:LISTEN | head -1)
  if [ -z "$node_pid" ] || [ -z "$go_pid" ]; then
    echo "サーバーが起動しませんでした。ログ: $WORK/node.log / $WORK/go.log" >&2
    tail -5 "$WORK/node.log" "$WORK/go.log" >&2
    exit 1
  fi
  server_pids+=("$node_pid" "$go_pid")
}
