terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    # DNS（ドメイン登録とゾーン）は Cloudflare にある。CloudFront の前に置くレコードだけを
    # ここで管理する（dns.tf）。トークンは CLOUDFLARE_API_TOKEN で渡し、リポジトリには置かない。
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 5.20"
    }
  }
}

provider "aws" {
  region = "ap-northeast-1"
}

# CloudFront に付ける ACM の証明書は us-east-1 にしか置けない（AWS の仕様）。
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"
}

provider "cloudflare" {}
