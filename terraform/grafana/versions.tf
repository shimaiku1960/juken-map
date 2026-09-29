# Grafana Cloud のアラートと通知先（JUK-83）。手順は README.md。
#
# 本番 AWS（../）とは state を分けてある。Grafana の API が落ちていても AWS の plan が止まらないように、
# また AWS の変更に Grafana の差分が混ざらないようにするため。コマンドは
# `terraform -chdir=terraform/grafana` で動かす。
terraform {
  required_version = ">= 1.6"

  required_providers {
    grafana = {
      source  = "grafana/grafana"
      version = "~> 4.46"
    }
  }
}

provider "grafana" {
  url  = "https://kindcrest3516.grafana.net"
  auth = var.grafana_auth
}
