resource "aws_security_group" "launch_wizard_1" {
  name        = "launch-wizard-1"
  description = "launch-wizard-1 created 2026-04-04T15:18:57.581Z"
  vpc_id      = "vpc-096363ecaa36fd99b"

  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

data "aws_ec2_managed_prefix_list" "cloudfront_origin_facing" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

resource "aws_security_group" "launch_wizard_2" {
  name        = "launch-wizard-2"
  description = "launch-wizard-2 created 2026-06-23T06:56:51.277Z"
  vpc_id      = "vpc-096363ecaa36fd99b"

  # 2026-10-14 から、利用者は CloudFront（cloudfront.tf）を通ってしか来ない（JUK-185）。
  # 443 は CloudFront がオリジンへつなぐときの IP だけに絞り、80 は閉じた（CloudFront は https-only で
  # つなぎ、http → https の転送も CloudFront が返す）。Let's Encrypt の更新の確認も CloudFront を通って
  # 443 に届く（infra/nginx/snippets/juken-map-acme-challenge.conf）。
  #
  # ⚠️ nginx は X-Forwarded-For の最後の値を利用者の IP として信じている（infra/nginx/conf.d/
  # juken-map-cloudfront-realip.conf）。それが安全なのは、ここで CloudFront 以外を通さないからである。
  # ここを広げるときは、その設定も一緒に見直す。
  #
  # このプレフィックスリストは、セキュリティグループのルール55本分として数えられる（上限は60本）。
  ingress {
    description     = "HTTPS from CloudFront only"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    prefix_list_ids = [data.aws_ec2_managed_prefix_list.cloudfront_origin_facing.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "default" {
  name        = "default"
  description = "default VPC security group"
  vpc_id      = "vpc-096363ecaa36fd99b"

  ingress {
    from_port = 0
    to_port   = 0
    protocol  = "-1"
    self      = true
  }

  ingress {
    from_port       = 3306
    to_port         = 3306
    protocol        = "tcp"
    security_groups = [aws_security_group.launch_wizard_2.id]
    description     = "from EC2 juken-compass"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}
