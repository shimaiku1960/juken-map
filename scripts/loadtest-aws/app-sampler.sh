#!/usr/bin/env bash
# 試験中の複製 EC2 の様子（コンテナごとの CPU・メモリ、ホストのメモリと Swap）を2秒ごとに記録する。
# CloudWatch の EC2 の指標にはメモリが無いので、ここで取る。ssm-run.sh app --script で送る。
#
#   app-sampler.sh start            記録を始める（裏で動き続ける）
#   app-sampler.sh summary FROM TO  FROM〜TO（UTC、例 2026-09-25T01:00:00Z）の最大値を JSON 1行で出す
#   app-sampler.sh stop
set -euo pipefail

LOG="/var/log/juken-map-loadtest-sampler.log"
PID_FILE="/run/juken-map-loadtest-sampler.pid"
SELF="/usr/local/sbin/juken-map-loadtest-sampler"

case "${1:-}" in
  start)
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
      echo "すでに動いています"
      exit 0
    fi
    # ssm-run.sh は一時ファイルとして送るので、残る場所へ自分を写してから裏で回す。
    install -m 755 "$0" "$SELF"
    nohup "$SELF" loop >> "$LOG" 2>&1 &
    echo $! > "$PID_FILE"
    echo "記録を始めました（${LOG}）"
    ;;

  loop)
    while true; do
      ts="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
      # 例: juken-map 12.34% 150.3MiB / 905.9MiB
      docker stats --no-stream --format '{{.Name}} {{.CPUPerc}} {{.MemUsage}}' \
        | while read -r name cpu mem _; do echo "$ts $name cpu=${cpu%\%} mem=$mem"; done
      free -m | awk -v ts="$ts" '
        /^Mem:/  { used = $3; avail = $7 }
        /^Swap:/ { swap = $3 }
        END      { print ts, "host", "used_mb=" used, "avail_mb=" avail, "swap_mb=" swap }'
      sleep 2
    done
    ;;

  summary)
    from="${2:?FROM}"
    to="${3:?TO}"
    awk -v from="$from" -v to="$to" '
      function mib(v) {
        if (v ~ /GiB$/) return substr(v, 1, length(v) - 3) * 1024
        if (v ~ /MiB$/) return substr(v, 1, length(v) - 3) + 0
        if (v ~ /KiB$/) return substr(v, 1, length(v) - 3) / 1024
        return 0
      }
      $1 >= from && $1 <= to {
        if ($2 == "host") {
          for (i = 3; i <= NF; i++) { split($i, kv, "="); v[kv[1]] = kv[2] + 0 }
          if (v["used_mb"] > used) used = v["used_mb"]
          if (avail == "" || v["avail_mb"] < avail) avail = v["avail_mb"]
          if (v["swap_mb"] > swap) swap = v["swap_mb"]
        } else {
          split($3, c, "="); split($4, m, "=")
          if (c[2] + 0 > cpu[$2]) cpu[$2] = c[2] + 0
          if (mib(m[2]) > mem[$2]) mem[$2] = mib(m[2])
        }
      }
      END {
        printf "{\"host_mem_used_mb_max\":%s,\"host_mem_avail_mb_min\":%s,\"host_swap_mb_max\":%s,\"containers\":{", \
          (used == "" ? "null" : used), (avail == "" ? "null" : avail), (swap == "" ? "null" : swap)
        sep = ""
        for (n in cpu) { printf "%s\"%s\":{\"cpu_percent_max\":%.1f,\"mem_mb_max\":%.0f}", sep, n, cpu[n], mem[n]; sep = "," }
        print "}}"
      }' "$LOG"
    ;;

  stop)
    [ -f "$PID_FILE" ] && kill "$(cat "$PID_FILE")" 2>/dev/null || true
    rm -f "$PID_FILE"
    echo "止めました"
    ;;

  *)
    echo "start / summary FROM TO / stop" >&2
    exit 1
    ;;
esac
