#!/usr/bin/env bash
# 試験環境の EC2 で、スクリプトかコマンドを SSM Run Command で動かし、出力を表示する（手元で実行）。
#
#   ssm-run.sh app    --script scripts/loadtest-aws/app-prepare.sh --init-disk
#   ssm-run.sh driver --cmd 'bash scripts/run-loadtest-limit.sh'
#
# app＝本番の AMI から複製した EC2（root で動く）。driver＝負荷をかける EC2
# （--cmd は ubuntu で、~/juken-map を起点に、ログインシェルとして動く）。
# 送り先は terraform/loadtest の出力から取り、タグ Project=juken-map-loadtest が付いた EC2 にしか送らない。
# 長く掛かるもの（合成データの投入など）は、--cmd の中で nohup にしてログを後から読む
# （Run Command の出力は 24,000 字で切れる）。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REGION="ap-northeast-1"

role="${1:?app か driver}"
mode="${2:?--script か --cmd}"
shift 2
case "$role" in app|driver) ;; *) echo "app か driver: $role" >&2; exit 1 ;; esac

instance_id="$(terraform -chdir="$ROOT/terraform/loadtest" output -raw "${role}_instance_id")"
project="$(aws ec2 describe-instances --region "$REGION" --instance-ids "$instance_id" \
  --query "Reservations[0].Instances[0].Tags[?Key=='Project'].Value | [0]" --output text)"
if [ "$project" != "juken-map-loadtest" ]; then
  echo "$instance_id は試験環境の EC2 ではありません（Project=${project}）。送りません" >&2
  exit 1
fi

case "$mode" in
  --script)
    script="${1:?スクリプトのパス}"
    shift
    args=""
    [ $# -gt 0 ] && args="$(printf '%q ' "$@")"
    b64="$(base64 < "$script" | tr -d '\n')"
    remote="f=\$(mktemp) && echo '$b64' | base64 -d > \"\$f\" && bash \"\$f\" $args"
    if [ "$role" = driver ]; then
      remote="sudo -u ubuntu -H bash -lc $(printf '%q' "cd ~/juken-map && $remote")"
    fi
    ;;
  --cmd)
    command_text="${1:?コマンド}"
    if [ "$role" = driver ]; then
      remote="sudo -u ubuntu -H bash -lc $(printf '%q' "cd ~/juken-map && $command_text")"
    else
      remote="$command_text"
    fi
    ;;
  *) echo "--script か --cmd: $mode" >&2; exit 1 ;;
esac

params="$(jq -cn --arg c "$remote" '{commands: [$c], executionTimeout: ["7200"]}')"
command_id="$(aws ssm send-command --region "$REGION" \
  --instance-ids "$instance_id" \
  --document-name AWS-RunShellScript \
  --parameters "$params" \
  --timeout-seconds 600 \
  --comment "juken-map loadtest ($role)" \
  --query Command.CommandId --output text)"

# 最大2時間待つ。
status="Pending"
for _ in $(seq 1 1440); do
  status="$(aws ssm get-command-invocation --region "$REGION" --command-id "$command_id" \
    --instance-id "$instance_id" --query Status --output text 2>/dev/null || echo Pending)"
  case "$status" in Success|Failed|TimedOut|Cancelled) break ;; esac
  sleep 5
done

result="$(aws ssm get-command-invocation --region "$REGION" --command-id "$command_id" \
  --instance-id "$instance_id" --output json)"
jq -r '.StandardOutputContent' <<<"$result"
jq -r '.StandardErrorContent' <<<"$result" >&2
[ "$status" = Success ]
