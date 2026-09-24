#!/usr/bin/env bash
# 試験環境が何も残っていないかを確かめる（手元で実行。terraform destroy の後に使う）。
#
#   leftover-check.sh                 残っているものを数える。0件なら終了コード0
#   leftover-check.sh --delete-images 試験用の AMI とスナップショット（本番の秘密情報を含む）を
#                                     消してから数える。AMI は Terraform の外で作るので destroy では消えない
set -euo pipefail

REGION="ap-northeast-1"
NAME="juken-map-loadtest"
TAG_FILTER="Name=tag:Project,Values=$NAME"
account_id="$(aws sts get-caller-identity --query Account --output text)"

if [ "${1:-}" = "--delete-images" ]; then
  for ami in $(aws ec2 describe-images --region "$REGION" --owners self --filters "$TAG_FILTER" \
    --query 'Images[].ImageId' --output text); do
    snapshots="$(aws ec2 describe-images --region "$REGION" --image-ids "$ami" \
      --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' --output text)"
    aws ec2 deregister-image --region "$REGION" --image-id "$ami"
    echo "AMI を消しました: $ami"
    for snapshot in $snapshots; do
      aws ec2 delete-snapshot --region "$REGION" --snapshot-id "$snapshot"
      echo "スナップショットを消しました: $snapshot"
    done
  done
fi

left=0
report() {
  local what="$1" found="$2"
  if [ -n "$found" ] && [ "$found" != "None" ]; then
    echo "残っている: $what → $found"
    left=$((left + 1))
  else
    echo "なし: $what"
  fi
}

report "EC2" "$(aws ec2 describe-instances --region "$REGION" --filters "$TAG_FILTER" \
  "Name=instance-state-name,Values=pending,running,stopping,stopped,shutting-down" \
  --query 'Reservations[].Instances[].InstanceId' --output text)"
report "EBS ボリューム" "$(aws ec2 describe-volumes --region "$REGION" --filters "$TAG_FILTER" \
  --query 'Volumes[].VolumeId' --output text)"
report "AMI" "$(aws ec2 describe-images --region "$REGION" --owners self --filters "$TAG_FILTER" \
  --query 'Images[].ImageId' --output text)"
report "スナップショット" "$(aws ec2 describe-snapshots --region "$REGION" --owner-ids self --filters "$TAG_FILTER" \
  --query 'Snapshots[].SnapshotId' --output text)"
report "VPC" "$(aws ec2 describe-vpcs --region "$REGION" --filters "$TAG_FILTER" \
  --query 'Vpcs[].VpcId' --output text)"
report "VPC エンドポイント" "$(aws ec2 describe-vpc-endpoints --region "$REGION" --filters "$TAG_FILTER" \
  --query 'VpcEndpoints[?State!=`deleted`].VpcEndpointId' --output text)"
report "RDS" "$(aws rds describe-db-instances --region "$REGION" --db-instance-identifier "$NAME-db" \
  --query 'DBInstances[].DBInstanceStatus' --output text 2>/dev/null || true)"
report "S3 バケット" "$(aws s3api head-bucket --bucket "$NAME-results-$account_id" 2>/dev/null && echo "$NAME-results-$account_id" || true)"
report "SSM パラメータ" "$(aws ssm get-parameter --region "$REGION" --name "/$NAME/env" \
  --query Parameter.Name --output text 2>/dev/null || true)"
report "IAM ロール" "$(for r in app driver; do aws iam get-role --role-name "$NAME-$r" --query Role.RoleName --output text 2>/dev/null || true; done | xargs)"

echo ""
if [ "$left" -eq 0 ]; then
  echo "何も残っていません。"
else
  echo "$left 種類が残っています。" >&2
  exit 1
fi
