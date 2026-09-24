# 負荷試験のために一時的に建てて壊す環境（JUK-46）。手順は README.md。
#
# ⚠️ 本番（../）とは state を分けてある。本番のリソース・ID・state を参照しないこと。
#    混ぜると destroy が本番へ届く恐れがある。必ず `terraform -chdir=terraform/loadtest` で動かす。
terraform {
  required_version = ">= 1.6"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = "ap-northeast-1"

  # 壊した後に「何も残っていないか」をこのタグで確かめる（scripts/loadtest-aws/leftover-check.sh）。
  default_tags {
    tags = {
      Project   = "juken-map-loadtest"
      ManagedBy = "terraform/loadtest"
    }
  }
}
