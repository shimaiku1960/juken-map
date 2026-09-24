locals {
  name     = "juken-map-loadtest"
  vpc_cidr = "10.50.0.0/16"
}

# 本番の既定 VPC（172.31.0.0/16）とは別の VPC。10.50.0.0/16 は load-tests/limit.js と
# scripts/run-loadtest-limit.sh が「試験環境の宛先」として許す範囲と揃えてある。
resource "aws_vpc" "this" {
  cidr_block           = local.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = local.name }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = local.name }
}

# 負荷をかける EC2 だけが置かれる。セットアップ（apt・git・pnpm）にインターネットが要る。
resource "aws_subnet" "public" {
  vpc_id                  = aws_vpc.this.id
  cidr_block              = "10.50.0.0/24"
  availability_zone       = var.primary_az
  map_public_ip_on_launch = true

  tags = { Name = "${local.name}-public" }
}

# 複製 EC2 と RDS。インターネットへの経路を持たない。
# 複製 EC2 は本番の設定（秘密情報を含む）のまま起動するので、ここから外へ出られないことが
# 「本番の DB・Resend・Grafana Cloud に届かない」ことの保証になる。SG の出口も絞ってある（二重の鍵）。
resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.this.id
  cidr_block        = "10.50.10.0/24"
  availability_zone = var.primary_az

  tags = { Name = "${local.name}-private-a" }
}

resource "aws_subnet" "private_b" {
  vpc_id            = aws_vpc.this.id
  cidr_block        = "10.50.11.0/24"
  availability_zone = var.secondary_az

  tags = { Name = "${local.name}-private-b" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = { Name = "${local.name}-public" }
}

resource "aws_route_table_association" "public" {
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# VPC の中だけ（local）の経路しか持たない。
resource "aws_route_table" "private" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${local.name}-private" }
}

resource "aws_route_table_association" "private_a" {
  subnet_id      = aws_subnet.private_a.id
  route_table_id = aws_route_table.private.id
}

resource "aws_route_table_association" "private_b" {
  subnet_id      = aws_subnet.private_b.id
  route_table_id = aws_route_table.private.id
}

# インターネットに出られない複製 EC2 を SSM で操作するための入口
# （Run Command と Parameter Store の読み取り）。
resource "aws_vpc_endpoint" "ssm" {
  for_each = toset(["ssm", "ssmmessages", "ec2messages"])

  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.ap-northeast-1.${each.key}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [aws_subnet.private_a.id]
  security_group_ids  = [aws_security_group.endpoints.id]
  private_dns_enabled = true

  tags = { Name = "${local.name}-${each.key}" }
}
