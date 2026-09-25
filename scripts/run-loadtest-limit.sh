#!/usr/bin/env bash
# 限界点の実測。RPSを段階的に上げ、最初に基準を割った段階を記録する。
#
# 測る相手は2つだけ。本番URLは受け付けない。
#   LOADTEST_ENV=local（既定）: 手元で起動した本番相当のサーバー（scripts/e2e-server.sh）
#   LOADTEST_ENV=aws          : 本番の AMI から複製した試験環境（terraform/loadtest）。
#                               負荷をかける EC2 の上で動かす。手順は terraform/loadtest/README.md
# SCENARIO=spike で、10人→100人のスパイクを1回だけ流す（RATES は使わない）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

LOADTEST_ENV="${LOADTEST_ENV:-local}"
SCENARIO="${SCENARIO:-level}"
BASE_URL="${BASE_URL:-http://host.docker.internal:3000}"
RATES="${RATES:-100 150 200 250 300 350 400}"
DURATION="${DURATION:-60s}"
OUT_DIR="${OUT_DIR:-$ROOT/load-tests/results}"
DB_CONTAINER="${DB_CONTAINER:-juken-map-db}"
[ "$SCENARIO" = spike ] && RATES="spike"

target_ok=false
case "$LOADTEST_ENV:$BASE_URL" in
  local:http://host.docker.internal:3000|local:http://localhost:3000|local:http://127.0.0.1:3000) target_ok=true ;;
esac
# 試験環境の VPC（10.50.0.0/16）の中の IP だけ。limit.js も同じ条件で確かめる。
if [ "$LOADTEST_ENV" = aws ] && [[ "$BASE_URL" =~ ^https://10\.50\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
  target_ok=true
fi
if [ "$target_ok" != true ]; then
  echo "本番や外部は対象にしません: ${BASE_URL}（LOADTEST_ENV=${LOADTEST_ENV}）" >&2
  exit 1
fi

command -v jq >/dev/null || { echo "jq が要ります" >&2; exit 1; }

api_pid=""
if [ "$LOADTEST_ENV" = local ]; then
  command -v docker >/dev/null || { echo "Docker が要ります" >&2; exit 1; }
  # APIのCPUを見るために、3000番で待ち受けているプロセスを探しておく。
  api_pid="$(lsof -ti tcp:3000 -sTCP:LISTEN 2>/dev/null | head -1 || true)"
  if [ -z "$api_pid" ]; then
    echo "3000番で待ち受けているサーバーが見つかりません（NODE_ENV=production bash scripts/e2e-server.sh）" >&2
    exit 1
  fi
else
  # 試験環境では API は別の EC2 にいるので、ここでは見ない。
  # EC2 と RDS の様子は scripts/loadtest-aws/collect-metrics.sh で段階の時間帯ごとに取る。
  command -v k6 >/dev/null || { echo "k6 が要ります" >&2; exit 1; }
  command -v mysql >/dev/null || { echo "mysql（クライアント）が要ります" >&2; exit 1; }
  # DB の接続情報は .env の DATABASE_URL から作る。パスワードをコマンドラインに出さないよう、
  # mysql には設定ファイルで渡す。
  MYSQL_CNF="$(mktemp)"
  chmod 600 "$MYSQL_CNF"
  trap 'rm -f "$MYSQL_CNF"' EXIT
  node --env-file=.env -e '
    const u = new URL(process.env.DATABASE_URL);
    console.log(["[client]", `host=${u.hostname}`, `port=${u.port || 3306}`,
      `user=${decodeURIComponent(u.username)}`, `password=${decodeURIComponent(u.password)}`,
      `database=${u.pathname.slice(1)}`, "ssl-mode=REQUIRED"].join("\n"));
  ' > "$MYSQL_CNF"
fi

# DB に SQL を1本投げて、タブ区切りで返す。
db_query() {
  if [ "$LOADTEST_ENV" = local ]; then
    docker exec "$DB_CONTAINER" mysql -uroot -prootpassword -N -B -e "$1" 2>/dev/null
  else
    mysql --defaults-extra-file="$MYSQL_CNF" -N -B -e "$1" 2>/dev/null
  fi
}

mkdir -p "$OUT_DIR"
stamp="$(date +%Y%m%d-%H%M%S)"
result_file="$OUT_DIR/limit-$stamp.jsonl"

# ログイン済みのセッションを先に発行する（HTTPでログインさせない理由は
# db/issue-loadtest-sessions.ts に書いた）。
# 試験環境は本番と同じ HTTPS なので、Cookie の名前に __Secure- が付く。
secure_cookie=off
[ "$LOADTEST_ENV" = aws ] && secure_cookie=on
users_json="$(SECURE_COOKIE="$secure_cookie" pnpm exec tsx --env-file=.env db/issue-loadtest-sessions.ts | tail -1)"
cookies="$(jq -er '.cookies | join(",")' <<<"$users_json")"
jq -r '"ログインに使う利用者: \(.count)人（1人あたりの実績 中央値\(.logs_per_user.median)件 / 最大\(.logs_per_user.max)件）"' <<<"$users_json"
jq -c '{picked_users: .count, logs_per_user: .logs_per_user}' <<<"$users_json" > "$OUT_DIR/population-$stamp.json"

# MySQL の累積カウンタ。段階の前後の差で「その段階で何が起きたか」を見る。
mysql_status() {
  db_query "
    SELECT variable_name, variable_value FROM performance_schema.global_status
    WHERE variable_name IN (
      'Threads_running','Threads_connected','Max_used_connections','Slow_queries',
      'Innodb_buffer_pool_reads','Innodb_buffer_pool_read_requests','Innodb_row_lock_waits',
      'Aborted_clients','Connection_errors_max_connections','Created_tmp_disk_tables'
    );"
}

# 試験環境では、段階ごとに結果を S3 へ上げておく（EC2 は試験後に消えるため）。
upload_results() {
  if [ "$LOADTEST_ENV" = aws ] && [ -n "${RESULTS_BUCKET:-}" ]; then
    aws s3 cp --quiet "$OUT_DIR/" "s3://$RESULTS_BUCKET/results/" --recursive || true
  fi
}

for rate in $RATES; do
  echo ""
  if [ "$SCENARIO" = spike ]; then
    echo "===== スパイク（10人→100人）====="
  else
    echo "===== ${rate} RPS（${DURATION}）====="
  fi

  before="$(mysql_status)"

  # 実行中の飽和を見る。Threads_running は「いま実際に走っているクエリ数」で、
  # ここが張り付いたらDBが詰まっている合図。APIのCPUも一緒に取る。Node は1スレッドなので、
  # 100%（1コア分）に張り付いたらDBではなくアプリ側が頭打ちということになる。
  samples="$(mktemp)"
  ( while true; do
      db_query "SELECT variable_value FROM performance_schema.global_status WHERE variable_name='Threads_running';" >> "$samples"
      if [ "$LOADTEST_ENV" = local ]; then
        docker stats --no-stream --format '{{.CPUPerc}} {{.MemUsage}}' "$DB_CONTAINER" 2>/dev/null >> "$samples.db"
        [ -n "$api_pid" ] && ps -o %cpu=,rss= -p "$api_pid" 2>/dev/null >> "$samples.api"
      fi
      sleep 2
    done ) &
  sampler=$!

  started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  set +e
  if [ "$LOADTEST_ENV" = local ]; then
    docker run --rm -i \
      -e BASE_URL="$BASE_URL" \
      -e LOAD_TEST_COOKIES="$cookies" \
      -e RATE="$rate" \
      -e MIX_WEIGHTS="${MIX_WEIGHTS:-}" \
      -e ACCEPT_ENCODING="${ACCEPT_ENCODING:-}" \
      -e DURATION="$DURATION" \
      -e SCENARIO="$SCENARIO" \
      grafana/k6:latest run --quiet - < load-tests/limit.js > "$OUT_DIR/raw-$rate.txt" 2>&1
  else
    # k6 はこの EC2 に直接入れたものを使う（環境変数はそのまま __ENV に入る）。
    BASE_URL="$BASE_URL" LOAD_TEST_COOKIES="$cookies" RATE="$rate" MIX_WEIGHTS="${MIX_WEIGHTS:-}" \
      ACCEPT_ENCODING="${ACCEPT_ENCODING:-}" \
      DURATION="$DURATION" SCENARIO="$SCENARIO" LOADTEST_ENV=aws \
      k6 run --quiet - < load-tests/limit.js > "$OUT_DIR/raw-$rate.txt" 2>&1
  fi
  k6_exit=$?
  set -e
  ended_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  kill "$sampler" 2>/dev/null || true
  wait "$sampler" 2>/dev/null || true

  after="$(mysql_status)"

  threads_max="$(sort -n "$samples" 2>/dev/null | tail -1)"
  db_cpu_max="$(awk '{gsub(/%/,"",$1); if ($1+0>m) m=$1+0} END {print m}' "$samples.db" 2>/dev/null)"
  api_cpu_max="$(awk '{if ($1+0>m) m=$1+0} END {print m}' "$samples.api" 2>/dev/null)"
  api_rss_mb="$(awk '{if ($2+0>m) m=$2+0} END {print int(m/1024)}' "$samples.api" 2>/dev/null)"
  rm -f "$samples" "$samples.db" "$samples.api"

  delta="$(join <(sort <<<"$before") <(sort <<<"$after") 2>/dev/null \
    | awk '{print $1"="($3-$2)}' | paste -sd' ' -)"

  line="$(grep -o 'LIMIT_RESULT .*' "$OUT_DIR/raw-$rate.txt" | sed 's/^LIMIT_RESULT //')"
  if [ -z "$line" ]; then
    echo "k6 の結果が取れませんでした（$OUT_DIR/raw-$rate.txt を見てください）" >&2
    exit 1
  fi

  jq -c --argjson exit "$k6_exit" --arg threads "$threads_max" --arg cpu "$db_cpu_max" \
       --arg apicpu "$api_cpu_max" --arg apirss "$api_rss_mb" --arg delta "$delta" \
       --arg env "$LOADTEST_ENV" --arg started "$started_at" --arg ended "$ended_at" \
       --arg overload "${OVERLOAD_LABEL:-}" \
    '. + {loadtest_env: $env, started_at: $started, ended_at: $ended, overload_max_in_flight: ($overload|tonumber? // null),
          k6_exit: $exit, db_threads_running_max: ($threads|tonumber? // null), db_cpu_percent_max: ($cpu|tonumber? // null),
          api_cpu_percent_max: ($apicpu|tonumber? // null), api_rss_mb_max: ($apirss|tonumber? // null), db_status_delta: $delta}' \
    <<<"$line" >> "$result_file"

  jq -r '"  実測 \(.rps_actual) RPS / 失敗率 \(.failed_rate*100|floor)% / 5xx \(.status["5xx"]) / 取りこぼし \(.dropped_iterations) / p95 \(.overall_ms.p95)ms / p99 \(.overall_ms.p99)ms"' <<<"$line"
  jq -r '"  成功 \(.ok_rps) 件/秒（p95 \(.ok_ms.p95)ms） / 混雑で断った \(.status.shed)件"' <<<"$line"
  echo "  DB: Threads_running最大 ${threads_max:-?} / CPU最大 ${db_cpu_max:-?}%  API: CPU最大 ${api_cpu_max:-?}% / RSS ${api_rss_mb:-?}MB"
  upload_results

  if [ "$k6_exit" -ne 0 ]; then
    broke_at="${broke_at:-$rate}"
    echo "  → この段階で基準を割った: $(jq -r '.thresholds_failed | join(", ")' <<<"$line")"
    if [ "${CONTINUE_AFTER_BREAK:-off}" != "on" ]; then
      echo ""
      echo "限界点は ${rate} RPS 未満。結果: $result_file"
      exit 0
    fi
  fi

  sleep 15
done

echo ""
if [ -n "${broke_at:-}" ]; then
  echo "最初に基準を割ったのは ${broke_at} RPS。結果: $result_file"
else
  echo "全段階で基準を満たしました。結果: $result_file"
fi
