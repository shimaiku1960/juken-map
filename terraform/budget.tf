# 使いすぎに気づくための予算アラート。
#
# 2026-09-17に無料プランから有料プランへ切り替えた。無料プランには「クレジットが尽きたら
# 止まる」という歯止めがあったが、有料プランにはそれが無い。設定ミスや想定外のアクセスで
# 請求が伸びても、月末の請求書を見るまで気づけない。そこに気づける仕組みを置く。
#
# AWS Budgets 自体は無料（アラートは月2件まで無料）。

variable "budget_alert_email" {
  description = "予算アラートの通知先メールアドレス。公開リポジトリなので値はコミットせず、TF_VAR_budget_alert_email で渡す"
  type        = string
  sensitive   = true
}

variable "monthly_budget_usd" {
  description = "月あたりの予算（ドル）。2026-09時点の実費は約38ドル（RDS 21・EC2 10・公開IPv4 4・その他）"
  type        = number
  default     = 45
}

resource "aws_budgets_budget" "monthly" {
  name         = "juken-map-monthly"
  budget_type  = "COST"
  limit_amount = var.monthly_budget_usd
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # 実費が予算の80%（約36ドル）に達した時点。まだ月内に手を打てる段階で知らせる。
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  # 「このままのペースだと月末に予算を超える」という予測。
  # 実費のアラートだけだと、超えたことは分かっても超える前には分からない。
  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_alert_email]
  }
}
