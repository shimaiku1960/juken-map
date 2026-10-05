# AWS アカウントの操作記録と、異常の検知（JUK-37）。
#
# セキュリティ基準 06 の H2（アプリの外の目）と H4（クラウドの操作記録を90日より長く残す）のため。
# アプリが乗っ取られたり止まったりしても動くよう、検知と知らせは AWS の中だけで完結させる
# （Grafana やアプリのログを通さない）。
#
#   CloudTrail（全リージョンの管理イベント）→ S3 に1年残す
#   GuardDuty（東京）→ EventBridge → SNS → メール
#
# CloudTrail の管理イベントは最初の1本の trail なら無料で、かかるのは S3 の保管料だけ（数十MB/月）。
# GuardDuty は30日の無料試用のあと、分析した CloudTrail・VPC フローログ・DNS の量で課金される。
# 見込み（2026-10-05、東京の単価と直近24時間の量から）：操作記録の分析が約3,800件/日で月約$0.5、通信と DNS の
# 分析が1GBあたり$1.18で量は未計測。合わせて月$1〜2ほど。マルウェア検査は疑いが出たときだけ1回約$1.5（30GB）。
# 試用中に GuardDuty の「使用状況」で見込みの額を確かめ、月1万円の予算と照らして続けるか決める。

data "aws_caller_identity" "current" {}

locals {
  account_id    = data.aws_caller_identity.current.account_id
  trail_name    = "juken-map-account"
  trail_arn     = "arn:aws:cloudtrail:ap-northeast-1:${local.account_id}:trail/${local.trail_name}"
  log_retention = 365
}

# ---- CloudTrail ----

resource "aws_s3_bucket" "cloudtrail" {
  # バケット名は世界で一意なので、アカウント ID を入れる（コードには書かず、実行時に読む）。
  bucket = "juken-map-cloudtrail-${local.account_id}"
}

resource "aws_s3_bucket_public_access_block" "cloudtrail" {
  bucket                  = aws_s3_bucket.cloudtrail.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_ownership_controls" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# H4：90日より長く。1年あれば、気づくのが遅れた漏えいでも後から追える。
resource "aws_s3_bucket_lifecycle_configuration" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  rule {
    id     = "expire-after-retention"
    status = "Enabled"
    filter {}
    expiration {
      days = local.log_retention
    }
  }
}

resource "aws_s3_bucket_policy" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "CloudTrailAclCheck"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:GetBucketAcl"
        Resource  = aws_s3_bucket.cloudtrail.arn
        Condition = { StringEquals = { "aws:SourceArn" = local.trail_arn } }
      },
      {
        Sid       = "CloudTrailWrite"
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.cloudtrail.arn}/AWSLogs/${local.account_id}/*"
        Condition = { StringEquals = { "aws:SourceArn" = local.trail_arn } }
      },
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource  = [aws_s3_bucket.cloudtrail.arn, "${aws_s3_bucket.cloudtrail.arn}/*"]
        Condition = { Bool = { "aws:SecureTransport" = "false" } }
      },
    ]
  })
  depends_on = [aws_s3_bucket_public_access_block.cloudtrail]
}

resource "aws_cloudtrail" "account" {
  name           = local.trail_name
  s3_bucket_name = aws_s3_bucket.cloudtrail.id

  # 使っていないリージョンでの操作（乗っ取ったキーでほかのリージョンにサーバーを立てる、など）も残す。
  is_multi_region_trail         = true
  include_global_service_events = true
  # ログのファイルが書き換えられていないことを、後から `aws cloudtrail validate-logs` で確かめられる。
  enable_log_file_validation = true

  # 管理イベント（誰がどの設定を変えたか）だけ。S3 のオブジェクト単位などのデータイベントは
  # 量が多く有料なので入れない。
  event_selector {
    read_write_type           = "All"
    include_management_events = true
  }

  depends_on = [aws_s3_bucket_policy.cloudtrail]
}

# ---- GuardDuty ----

resource "aws_guardduty_detector" "main" {
  enable = true
}

# 基本の分析（CloudTrail の管理イベント・VPC フローログ・DNS）は外せない。ほかの機能は明示して、
# 使っていないものを有料で動かさない。
locals {
  guardduty_features = {
    S3_DATA_EVENTS         = "ENABLED" # S3 の不審な読み出し。バケットは CloudTrail のものなど少数で、量は小さい
    EBS_MALWARE_PROTECTION = "ENABLED" # EC2 に疑いが出たときだけディスクを調べる（調べた量で課金）
    # RDS への不審なログイン。MySQL 8.4.8 以上なら動くが、vCPU 数で課金され（2 vCPU で月約$2.7）、GuardDuty の費用の
    # 大半を占める。RDS はインターネットから直接つなげず、入れるのは EC2 からだけなので、そこまで来た時点で EC2 側の
    # 検出（不審な通信・マルウェア）で先に気づける見込み。費用に見合わないので切る（2026-10-05 ユーザー判断）。
    RDS_LOGIN_EVENTS    = "DISABLED"
    EKS_AUDIT_LOGS      = "DISABLED" # EKS は使っていない
    LAMBDA_NETWORK_LOGS = "DISABLED" # Lambda は使っていない
    RUNTIME_MONITORING  = "DISABLED" # EC2 にエージェントを入れる必要があり、t3.micro のメモリに余裕が無い
  }
}

resource "aws_guardduty_detector_feature" "main" {
  for_each    = local.guardduty_features
  detector_id = aws_guardduty_detector.main.id
  name        = each.key
  status      = each.value
}

# ---- 知らせ ----

variable "security_alert_email" {
  description = "GuardDuty の検出結果を受け取るメールアドレス。公開リポジトリなので値はコミットせず、TF_VAR_security_alert_email で渡す"
  type        = string
  sensitive   = true
}

resource "aws_sns_topic" "security_alerts" {
  name = "juken-map-security-alerts"
}

resource "aws_sns_topic_policy" "security_alerts" {
  arn = aws_sns_topic.security_alerts.arn
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid       = "EventBridgePublish"
      Effect    = "Allow"
      Principal = { Service = "events.amazonaws.com" }
      Action    = "sns:Publish"
      Resource  = aws_sns_topic.security_alerts.arn
      Condition = { ArnEquals = { "aws:SourceArn" = aws_cloudwatch_event_rule.guardduty_findings.arn } }
    }]
  })
}

# メールの受け取りは、AWS から届く確認メールのリンクを押すまで有効にならない。
resource "aws_sns_topic_subscription" "security_alerts_email" {
  topic_arn = aws_sns_topic.security_alerts.arn
  protocol  = "email"
  endpoint  = var.security_alert_email
}

# 重大度で絞らない。ルートでのログイン（Policy:IAMUser/RootCredentialUsage）や CloudTrail の停止
# （Stealth:IAMUser/CloudTrailLoggingDisabled）は「低」に分類されるが、このアカウントでは起きること
# 自体がおかしい。利用者がほぼ1人のアカウントなので、件数は少ない見込み。
resource "aws_cloudwatch_event_rule" "guardduty_findings" {
  name        = "juken-map-guardduty-findings"
  description = "GuardDuty の検出結果をメールで知らせる（JUK-37）"
  event_pattern = jsonencode({
    source        = ["aws.guardduty"]
    "detail-type" = ["GuardDuty Finding"]
  })
}

resource "aws_cloudwatch_event_target" "guardduty_findings_email" {
  rule = aws_cloudwatch_event_rule.guardduty_findings.name
  arn  = aws_sns_topic.security_alerts.arn

  # JSON のままだと読みにくいので、メールの本文を組み立てる。
  input_transformer {
    input_paths = {
      type     = "$.detail.type"
      severity = "$.detail.severity"
      title    = "$.detail.title"
      region   = "$.detail.region"
      id       = "$.detail.id"
      time     = "$.detail.updatedAt"
    }
    input_template = <<-EOT
      "[GuardDuty] <title>"
      "種類: <type> / 重大度: <severity>（7以上=高、4〜6.9=中、4未満=低）"
      "リージョン: <region> / 更新: <time>"
      "詳細: https://<region>.console.aws.amazon.com/guardduty/home?region=<region>#/findings?macros=current&fId=<id>"
      "対応の手順: docs/incident-response.md"
    EOT
  }
}
