#!/bin/bash
# ホストの層の障害注入（カオス段階2、JUK-174）。SSM ドキュメント juken-map-chaos-host（terraform/chaos.tf）の中身で、
# 予告なしのくじ（apps/api/internal/feature/chaos/schedule.go）が SSM Run Command で本番の EC2 に root で流す。
#
# 安全のための決まり:
#   - 障害はどれも systemd の一時的なユニット（juken-map-chaos-*）として動かし、RuntimeMaxSec で終わる時刻に必ず止める。
#     止まったら ExecStopPost で元に戻すので、API のプロセスや SSM Agent が落ちても戻る。EC2 を再起動しても tc・stress-ng は消える
#   - 同時には1つだけ。ディスクは空きを MIN_FREE_MB より減らさない。データ（RDS・ボリューム）には触らない
#   - 引数は SSM ドキュメントの allowedValues・allowedPattern で絞り、ここでも種類ごとの値（apps/api/internal/hostfault の Levels）
#     と突き合わせる
#   - revert は全部を戻す。デプロイ（.github/scripts/deploy-ec2.sh の stop_chaos）も同じ2つ（ユニットを止める・ファイルを消す）を行う
#
# SSM は二重の波かっこで囲んだ名前をパラメーターに置き換えるので、このファイルではすぐ下の5つのほかに二重の波かっこを書かない。
set -euo pipefail

action='{{ Action }}'
kind='{{ Kind }}'
duration='{{ Seconds }}'
level='{{ Level }}'
db_host='{{ DatabaseHost }}'

unit=juken-map-chaos
fill_file=/var/tmp/juken-map-chaos-fill
container=juken-map-go
min_free_mb=512

revert() {
  # ユニットを止めると、ExecStopPost が tc の設定とディスクを埋めたファイルを戻す。
  systemctl stop "$unit-*.timer" "$unit-*.service" 2>/dev/null || true
  systemctl reset-failed "$unit-*" 2>/dev/null || true
  rm -f "$fill_file"
}

fail() {
  echo "$*" >&2
  exit 1
}

if [ "$action" = revert ]; then
  revert
  echo reverted
  exit 0
fi
[ "$action" = start ] || fail "unknown action: $action"

case "$kind" in
  process_kill) allowed="0" ;;
  cpu) allowed="50 80 100" ;;
  memory) allowed="50 70 90" ;;
  disk) allowed="90 95 98" ;;
  db_delay) allowed="100 300 1000" ;;
  db_loss) allowed="10 30 100" ;;
  *) fail "unknown kind: $kind" ;;
esac
case " $allowed " in
  *" $level "*) ;;
  *) fail "level $level is not allowed for $kind" ;;
esac
[ "$duration" -ge 60 ] && [ "$duration" -le 1800 ] || fail "seconds must be 60..1800"

if systemctl list-units --all --plain --no-legend "$unit-*" | grep -q .; then
  fail "another host fault is running"
fi

# run は名前 $1 のユニットで残りのコマンドを動かし、$duration 秒で止める。
run() {
  local name=$1
  shift
  systemd-run --quiet --collect --unit="$unit-$name" -p RuntimeMaxSec="$duration" "$@"
}

need_stress_ng() {
  command -v stress-ng >/dev/null && return
  DEBIAN_FRONTEND=noninteractive apt-get install -y -q stress-ng >/dev/null
}

case "$kind" in
  process_kill)
    # コンテナの外から main のプロセスを落とす（docker kill は手で止めた扱いになり、再起動の方針が働かない）。
    # SSM に成功を返してから落とすよう、10秒あとにする。
    pid="$(docker container inspect "$container" | grep -m1 '"Pid":' | tr -dc '0-9')"
    [ -n "$pid" ] && [ "$pid" -gt 1 ] || fail "$container is not running"
    systemd-run --quiet --collect --unit="$unit-process-kill" --on-active=10s /bin/kill -KILL "$pid"
    ;;
  cpu)
    need_stress_ng
    run cpu stress-ng --cpu 0 --cpu-load "$level" --timeout "${duration}s"
    ;;
  memory)
    need_stress_ng
    # 使えるメモリの level % を確保し続ける。
    run memory stress-ng --vm 1 --vm-bytes "${level}%" --vm-keep --timeout "${duration}s"
    ;;
  disk)
    read -r size_kb used_kb avail_kb < <(df --output=size,used,avail -k /var/tmp | tail -1)
    fill_kb=$((size_kb * level / 100 - used_kb))
    max_kb=$((avail_kb - min_free_mb * 1024))
    [ "$fill_kb" -le "$max_kb" ] || fill_kb=$max_kb
    [ "$fill_kb" -gt 0 ] || fail "disk is already full enough"
    fallocate -l "${fill_kb}K" "$fill_file"
    run disk -p ExecStopPost="/bin/rm -f $fill_file" /bin/sleep "$duration" || {
      rm -f "$fill_file"
      exit 1
    }
    ;;
  db_delay | db_loss)
    # RDS への MySQL の通信（宛先の IP とポート）だけを4つ目の帯に分け、そこに netem をかける。
    # ほかの通信は prio の既定の振り分け（1〜3の帯）のまま。
    db_ip="$(getent ahostsv4 "$db_host" | awk 'NR == 1 { print $1 }')"
    [ -n "$db_ip" ] || fail "cannot resolve $db_host"
    dev="$(ip -4 route get "$db_ip" | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')"
    [ -n "$dev" ] || fail "no route to $db_ip"
    if [ "$kind" = db_delay ]; then netem="delay ${level}ms"; else netem="loss ${level}%"; fi
    modprobe sch_netem
    tc qdisc replace dev "$dev" root handle 1: prio bands 4
    # shellcheck disable=SC2086 # netem は「delay 100ms」のように2語で渡す
    if ! tc qdisc add dev "$dev" parent 1:4 handle 40: netem $netem ||
      ! tc filter add dev "$dev" parent 1:0 protocol ip prio 1 u32 \
        match ip dst "$db_ip/32" match ip dport 3306 0xffff flowid 1:4 ||
      ! run network -p ExecStopPost="/usr/sbin/tc qdisc del dev $dev root" /bin/sleep "$duration"; then
      tc qdisc del dev "$dev" root || true
      exit 1
    fi
    ;;
esac
echo started
