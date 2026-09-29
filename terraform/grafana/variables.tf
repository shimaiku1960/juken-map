variable "grafana_auth" {
  description = "Grafana のサービスアカウント（sa-1-terraform）のトークン。コミットせず、TF_VAR_grafana_auth で渡す"
  type        = string
  sensitive   = true
}

variable "alert_email_addresses" {
  description = "アラートの通知先メールアドレス（複数ならセミコロン区切り）。公開リポジトリなので値はコミットせず、TF_VAR_alert_email_addresses で渡す"
  type        = string
  sensitive   = true
}
