#!/usr/bin/env bash
# Node（apps/api）と Go（apps/api-go）の GET /api/dashboard を、1リクエストあたりの
# CPU 時間で比べる（JUK-69）。
#
# 負荷試験ではない。リクエストは1件ずつ順番に送るので、同時に動くのは
# 「curl 1本・サーバー1つ・DB」だけで、Mac のほかの作業は止まらない。
# サーバーの CPU 時間（ps -o time）が N 件でどれだけ増えたかを N で割って出す。
# 同時に何件さばけるか（限界 RPS）は測れないが、比べたい「1件の重さ」はこれで足りる。
#
# 使い方（worktree のルートから）:
#   bash apps/api-go/compare-cpu.sh
#   N=1000 ROUNDS=5 bash apps/api-go/compare-cpu.sh
#
# 前提: DB が起動していて、合成データ（pnpm db:seed:synthetic）が入っていること。
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../.." && pwd)
N=${N:-500}              # 1回に送る件数
ROUNDS=${ROUNDS:-3}      # Node と Go を交互に何回ずつ測るか
WARMUP=${WARMUP:-300}    # 測る前の慣らし（Node の JIT、DB のキャッシュを温める）
NODE_PORT=${NODE_PORT:-18000}
GO_PORT=${GO_PORT:-18080}
WORK=$(mktemp -d)

pids=()
cleanup() {
  # 止めたジョブの「Terminated」表示は bash 自身が stderr に出すので、終了処理の間は捨てる。
  exec 2>/dev/null
  for pid in "${pids[@]}"; do kill "$pid" || true; done
  wait || true
  rm -rf "$WORK"
}
trap cleanup EXIT

for port in "$NODE_PORT" "$GO_PORT"; do
  if lsof -ti tcp:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    echo "ポート $port が使用中です。NODE_PORT / GO_PORT で変えてください。" >&2
    exit 1
  fi
done

# ps -o time は "分:秒.xx"（長いと "時:分:秒"）。秒に直す。
cpu_secs() { ps -o time= -p "$1" | awk -F: '{s=0; for (i=1;i<=NF;i++) s=s*60+$i; printf "%.3f", s}'; }
listener() { lsof -ti tcp:"$1" -sTCP:LISTEN | head -1; }

# ログインは HTTP で通さず、合成ユーザーのセッションを DB に直接発行する（load-tests と同じ）。
cookies=()
while IFS= read -r c; do cookies+=("$c"); done < <(
  cd "$ROOT" && COUNT=20 pnpm exec tsx --env-file=.env db/issue-loadtest-sessions.ts 2>/dev/null | tail -1 | jq -r '.cookies[]'
)
if [ ${#cookies[@]} -eq 0 ]; then
  echo "セッションを発行できませんでした。合成データが入っているか確かめてください。" >&2
  exit 1
fi

echo "Go をビルドしています…"
(cd "$ROOT/apps/api-go" && go build -o "$WORK/api-go" .)

# pnpm や tsx を挟むと PID がラッパーのものになるので、実際に待ち受けている PID を後で引く。
(cd "$ROOT" && NODE_ENV=production API_PORT=$NODE_PORT BETTER_AUTH_URL=http://localhost:$NODE_PORT \
  pnpm --filter @juken-map/api start >"$WORK/node.log" 2>&1) &
pids+=($!)
(set -a; source "$ROOT/.env"; set +a; PORT=$GO_PORT "$WORK/api-go" >"$WORK/go.log" 2>&1) &
pids+=($!)

for _ in $(seq 1 120); do
  curl -sf -o /dev/null "localhost:$NODE_PORT/api/health" && curl -sf -o /dev/null "localhost:$GO_PORT/api/health" && break
  sleep 0.5
done
node_pid=$(listener "$NODE_PORT")
go_pid=$(listener "$GO_PORT")
pids+=("$node_pid" "$go_pid")

# hit ポート 件数 → 1件ごとの応答時間（秒）を1行ずつ出す。
# 圧縮は Go 側に無いので、条件を揃えるため identity にする。
hit() {
  local port=$1 n=$2 i
  for ((i = 0; i < n; i++)); do
    curl -s -o /dev/null -w '%{time_total}\n' -H "Cookie: ${cookies[i % ${#cookies[@]}]}" \
      -H 'Accept-Encoding: identity' "localhost:$port/api/dashboard"
  done
}

echo "慣らし（${WARMUP}件ずつ）…"
hit "$NODE_PORT" "$WARMUP" >/dev/null
hit "$GO_PORT" "$WARMUP" >/dev/null

# 手元は他のアプリの影響で揺れるので、Node と Go を交互に測って、同じ時間帯どうしで比べる。
printf "round\ttarget\tcpu_ms/req\tp50_ms\tp95_ms\n"
for round in $(seq 1 "$ROUNDS"); do
  for target in node go; do
    if [ "$target" = node ]; then port=$NODE_PORT pid=$node_pid; else port=$GO_PORT pid=$go_pid; fi
    before=$(cpu_secs "$pid")
    hit "$port" "$N" | sort -n >"$WORK/lat.txt"
    after=$(cpu_secs "$pid")
    awk -v a="$after" -v b="$before" -v n="$N" -v r="$round" -v t="$target" \
      '{x[NR]=$1} END {printf "%s\t%s\t%.2f\t%.2f\t%.2f\n", r, t, (a-b)*1000/n, x[int(NR*0.5)]*1000, x[int(NR*0.95)]*1000}' \
      "$WORK/lat.txt"
  done
done
echo "load average: $(sysctl -n vm.loadavg)"
