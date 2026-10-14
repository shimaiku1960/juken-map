# AWS の層の異常をメールで知らせる CloudWatch アラーム（JUK-188）。
#
# アプリ（Go）の監視は Grafana Cloud が受け持つ（5xx 率・up・外形監視）。Grafana からは見えない
# RDS のディスクと CPU、EC2 のハードと OS の層をここで受け持ち、役割を重ねない。
#
# しきい値は 2026-10-14 にコンソールで見た値から決めた。RDS の CPU は 3〜6%、空き容量は
# 約 19.5 GB / 20 GiB。アラームは10本まで無料。

# security_alerts とは分ける。あちらはトピックポリシーで発行元を EventBridge だけに絞っており、
# CloudWatch アラームからは発行できない。こちらはポリシーを置かず、同じアカウントからの発行を許す既定のままにする。
resource "aws_sns_topic" "ops_alerts" {
  name = "juken-map-ops-alerts"
}

# 宛先は GuardDuty の知らせと同じ。メールの受け取りは、AWS から届く確認メールのリンクを押すまで有効にならない。
resource "aws_sns_topic_subscription" "ops_alerts_email" {
  topic_arn = aws_sns_topic.ops_alerts.arn
  protocol  = "email"
  endpoint  = var.security_alert_email
}

# 普段の10倍を超える状態が15分続いたときだけ。負荷試験（load-testing）では鳴ることがある。
resource "aws_cloudwatch_metric_alarm" "rds_cpu_high" {
  alarm_name          = "juken-map-rds-cpu-high"
  alarm_description   = "RDS の CPU 使用率が 80% を超えた状態が15分続いた（普段は 3〜6%）"
  namespace           = "AWS/RDS"
  metric_name         = "CPUUtilization"
  dimensions          = { DBInstanceIdentifier = aws_db_instance.db.identifier }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 80
  alarm_actions       = [aws_sns_topic.ops_alerts.arn]
  ok_actions          = [aws_sns_topic.ops_alerts.arn]
}

# ストレージの自動拡張（max_allocated_storage）があるので、満杯で止まる前に RDS が広げる。
# それでも、拡張が始まる（空きが1割を切る）より手前で、データの増え方と料金の増え方に気づけるようにする。
resource "aws_cloudwatch_metric_alarm" "rds_free_storage_low" {
  alarm_name          = "juken-map-rds-free-storage-low"
  alarm_description   = "RDS の空き容量が 5 GiB を下回った（確保は 20 GiB）"
  namespace           = "AWS/RDS"
  metric_name         = "FreeStorageSpace"
  dimensions          = { DBInstanceIdentifier = aws_db_instance.db.identifier }
  statistic           = "Minimum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "LessThanThreshold"
  threshold           = 5 * 1024 * 1024 * 1024
  alarm_actions       = [aws_sns_topic.ops_alerts.arn]
  ok_actions          = [aws_sns_topic.ops_alerts.arn]
}

# システム（AWS 側のハード）とインスタンス（OS）のどちらかのステータスチェックが2分続けて失敗したとき。
# ステータスチェックの指標は詳細モニタリングなしでも1分ごとに届く。インスタンスを止めている間は
# 指標が来ず「データ不足」になるだけで、鳴らない。
resource "aws_cloudwatch_metric_alarm" "ec2_status_check_failed" {
  alarm_name          = "juken-map-ec2-status-check-failed"
  alarm_description   = "EC2 のステータスチェック（システムかインスタンス）が2分続けて失敗した"
  namespace           = "AWS/EC2"
  metric_name         = "StatusCheckFailed"
  dimensions          = { InstanceId = aws_instance.app.id }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 2
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 1
  alarm_actions       = [aws_sns_topic.ops_alerts.arn]
  ok_actions          = [aws_sns_topic.ops_alerts.arn]
}
