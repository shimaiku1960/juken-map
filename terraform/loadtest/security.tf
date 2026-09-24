# Terraform は SG を作るときに AWS 既定の「出口すべて許可」を消す。
# ここに書いた規則だけが通る。

# ---- 複製 EC2（本番の AMI から起動）----
resource "aws_security_group" "app" {
  name        = "${local.name}-app"
  description = "juken-map loadtest: app copied from production AMI. No internet egress."
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${local.name}-app" }
}

resource "aws_vpc_security_group_ingress_rule" "app_https_from_driver" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.driver.id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "k6 from the driver"
}

# 出口は試験用 RDS と SSM のエンドポイントだけ。
resource "aws_vpc_security_group_egress_rule" "app_to_db" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.db.id
  ip_protocol                  = "tcp"
  from_port                    = 3306
  to_port                      = 3306
  description                  = "loadtest RDS only"
}

resource "aws_vpc_security_group_egress_rule" "app_to_endpoints" {
  security_group_id            = aws_security_group.app.id
  referenced_security_group_id = aws_security_group.endpoints.id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "SSM endpoints only"
}

# ---- 負荷をかける EC2 ----
resource "aws_security_group" "driver" {
  name        = "${local.name}-driver"
  description = "juken-map loadtest: k6 driver. No ingress (SSM only)."
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${local.name}-driver" }
}

resource "aws_vpc_security_group_egress_rule" "driver_all" {
  security_group_id = aws_security_group.driver.id
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "-1"
  description       = "setup (apt, git, pnpm) and k6"
}

# ---- 試験用 RDS ----
resource "aws_security_group" "db" {
  name        = "${local.name}-db"
  description = "juken-map loadtest: RDS"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${local.name}-db" }
}

resource "aws_vpc_security_group_ingress_rule" "db_from_app" {
  security_group_id            = aws_security_group.db.id
  referenced_security_group_id = aws_security_group.app.id
  ip_protocol                  = "tcp"
  from_port                    = 3306
  to_port                      = 3306
  description                  = "app"
}

# 合成データの投入・マイグレーション・試験中の SHOW STATUS。
resource "aws_vpc_security_group_ingress_rule" "db_from_driver" {
  security_group_id            = aws_security_group.db.id
  referenced_security_group_id = aws_security_group.driver.id
  ip_protocol                  = "tcp"
  from_port                    = 3306
  to_port                      = 3306
  description                  = "seed and status from the driver"
}

# ---- VPC エンドポイント ----
resource "aws_security_group" "endpoints" {
  name        = "${local.name}-endpoints"
  description = "juken-map loadtest: SSM interface endpoints"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${local.name}-endpoints" }
}

resource "aws_vpc_security_group_ingress_rule" "endpoints_https" {
  security_group_id = aws_security_group.endpoints.id
  cidr_ipv4         = local.vpc_cidr
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  description       = "from inside the loadtest VPC"
}
