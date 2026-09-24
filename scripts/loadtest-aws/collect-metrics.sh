#!/usr/bin/env bash
# 限界点の結果（limit-*.jsonl）の各段階に、その時間帯の EC2・RDS・コンテナの様子を足す（手元で実行）。
#
#   collect-metrics.sh load-tests/results/aws/limit-XXXX.jsonl
#   → 同じ場所に limit-XXXX.metrics.jsonl ができる
#
# CloudWatch は数分遅れて揃うので、試験が終わって5分ほど置いてから流す。
# CPU クレジットは5分単位でしか出ないので、短い段階では前後の値になる。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REGION="ap-northeast-1"
input="${1:?limit-*.jsonl}"
output="${input%.jsonl}.metrics.jsonl"

app_id="$(terraform -chdir="$ROOT/terraform/loadtest" output -raw app_instance_id)"
db_id="$(terraform -chdir="$ROOT/terraform/loadtest" output -raw db_identifier)"

# 1つの指標の、時間帯の中の最大（または最小）を返す。点が無ければ null。
cw() {
  local namespace="$1" metric="$2" dim_name="$3" dim_value="$4" stat="$5" period="$6" from="$7" to="$8"
  local pick="max"
  [ "$stat" = Minimum ] && pick="min"
  aws cloudwatch get-metric-statistics --region "$REGION" \
    --namespace "$namespace" --metric-name "$metric" \
    --dimensions "Name=$dim_name,Value=$dim_value" \
    --start-time "$from" --end-time "$to" --period "$period" --statistics "$stat" \
    --query "Datapoints[].$stat" --output json | jq "$pick // null"
}

: > "$output"
while IFS= read -r line; do
  from="$(jq -r '.started_at' <<<"$line")"
  to="$(jq -r '.ended_at' <<<"$line")"
  label="$(jq -r 'if .scenario == "spike" then "spike" else "\(.rate_target) RPS" end' <<<"$line")"
  echo "${label}（${from}〜${to}）" >&2

  ec2="$(jq -n \
    --argjson cpu "$(cw AWS/EC2 CPUUtilization InstanceId "$app_id" Maximum 60 "$from" "$to")" \
    --argjson credit "$(cw AWS/EC2 CPUCreditBalance InstanceId "$app_id" Minimum 300 "$from" "$to")" \
    --argjson surplus "$(cw AWS/EC2 CPUSurplusCreditBalance InstanceId "$app_id" Maximum 300 "$from" "$to")" \
    '{cpu_percent_max: $cpu, credit_balance_min: $credit, surplus_credit_max: $surplus}')"

  rds="$(jq -n \
    --argjson cpu "$(cw AWS/RDS CPUUtilization DBInstanceIdentifier "$db_id" Maximum 60 "$from" "$to")" \
    --argjson mem "$(cw AWS/RDS FreeableMemory DBInstanceIdentifier "$db_id" Minimum 60 "$from" "$to")" \
    --argjson swap "$(cw AWS/RDS SwapUsage DBInstanceIdentifier "$db_id" Maximum 60 "$from" "$to")" \
    --argjson rio "$(cw AWS/RDS ReadIOPS DBInstanceIdentifier "$db_id" Maximum 60 "$from" "$to")" \
    --argjson wio "$(cw AWS/RDS WriteIOPS DBInstanceIdentifier "$db_id" Maximum 60 "$from" "$to")" \
    --argjson burst "$(cw AWS/RDS BurstBalance DBInstanceIdentifier "$db_id" Minimum 60 "$from" "$to")" \
    --argjson conn "$(cw AWS/RDS DatabaseConnections DBInstanceIdentifier "$db_id" Maximum 60 "$from" "$to")" \
    '{cpu_percent_max: $cpu, freeable_memory_mb_min: (if $mem then ($mem / 1048576 | floor) else null end),
      swap_mb_max: (if $swap then ($swap / 1048576 | floor) else null end),
      read_iops_max: $rio, write_iops_max: $wio, burst_balance_min: $burst, connections_max: $conn}')"

  app="$("$ROOT/scripts/loadtest-aws/ssm-run.sh" app --script "$ROOT/scripts/loadtest-aws/app-sampler.sh" summary "$from" "$to" | tail -1)"
  jq -e . >/dev/null <<<"$app" || app="null"

  jq -c --argjson ec2 "$ec2" --argjson rds "$rds" --argjson app "$app" \
    '. + {aws: {ec2: $ec2, rds: $rds, app: $app}}' <<<"$line" >> "$output"
done < "$input"

echo "書き出しました: $output" >&2
