#!/usr/bin/env bash
# 本番 EC2 から、負荷試験の複製に使う AMI を作る（手元で実行）。
#
# ⚠️ --no-reboot は外さない。外すと AWS が本番 EC2 を再起動してサービスが止まる。
# ⚠️ AMI には本番の秘密情報（.env）・証明書の鍵・ログが写る。試験が終わったら
#    scripts/loadtest-aws/leftover-check.sh --delete-images で消す。
#
# 出力の最後の行が AMI の ID（terraform の source_ami_id に渡す）。
set -euo pipefail

PROD_INSTANCE_ID="i-0eeb166295363e11d"
REGION="ap-northeast-1"
TAG="{Key=Project,Value=juken-map-loadtest}"
name="juken-map-loadtest-src-$(date -u +%Y%m%d-%H%M%S)"

ami_id="$(aws ec2 create-image \
  --region "$REGION" \
  --instance-id "$PROD_INSTANCE_ID" \
  --name "$name" \
  --description "JUK-46 loadtest source. Contains production secrets; delete after the test." \
  --no-reboot \
  --tag-specifications "ResourceType=image,Tags=[$TAG]" "ResourceType=snapshot,Tags=[$TAG]" \
  --query ImageId --output text)"
echo "AMI $ami_id を作成中（使えるようになるまで待ちます。初回は数十分かかることがある）" >&2

# aws ec2 wait image-available は10分で諦めるので、自分で待つ（最大60分）。
for _ in $(seq 1 120); do
  state="$(aws ec2 describe-images --region "$REGION" --image-ids "$ami_id" \
    --query 'Images[0].State' --output text)"
  case "$state" in
    available) echo "$ami_id"; exit 0 ;;
    failed|invalid|error) echo "AMI の作成に失敗しました: $state" >&2; exit 1 ;;
  esac
  sleep 30
done
echo "60分待っても available になりませんでした: $ami_id" >&2
exit 1
