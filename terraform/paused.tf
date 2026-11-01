# 本番の休止（JUK-218、2026-11-02 ユーザー判断）。費用をほぼ $0 にするため、お金のかかるものを消してある。
#
# 消すもの（paused = true で count = 0）：EC2（EBS ごと）・RDS・Secrets Manager・CloudWatch アラームのうち
#   EC2 と RDS を見る3本・origin.juken-map.com の DNS・カオスの SSM ドキュメント・CloudTrail とその S3・GuardDuty
# 残すもの（無料かほぼ無料）：IAM・Identity Center・予算アラート・SNS・CloudFront・ACM・ECR・SG・DNS の apex と www
# Terraform の外で消したもの：Elastic IP（13.196.28.52、解放したので同じ IP には戻らない）
#
# 休止中、https://juken-map.com は CloudFront の 502 になる（転送先の origin が無い）。
#
# 再開の手順（中身を作り直す作業がいちばん重い。EC2 の中身は手作業の設定で、user_data は無い）：
#   1. Secrets Manager は削除の待ち期間（7日）が明けてから同じ名前で作れる。paused = false で apply し、
#      手元のバックアップ（~/juken-map-backup-2026-11-02/secret-runtime.json）の値を入れる
#   2. EC2 に Elastic IP を付け直し、docker・nginx・certbot・Swap・systemd timer・SSM エージェントを入れる。
#      設定の控えは ec2-config.tgz（/etc/nginx・systemd・fstab）と infra/nginx・infra/systemd、.env は ec2.env
#   3. RDS に juken_map.sql を流し込む（SSM のポート転送で EC2 経由）。ユーザー（juken_app・juken_migrate）も作り直す
#   4. GitHub の deploy・simulation・lighthouse のワークフローを有効に戻し、deploy を手で1回動かす
#   5. Grafana の通知を戻す（terraform/grafana の paused = false）
variable "paused" {
  description = "true のあいだ、お金のかかる本番のリソースを作らない（JUK-218）"
  type        = bool
  default     = true
}

locals {
  running = var.paused ? 0 : 1
}
