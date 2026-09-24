data "aws_caller_identity" "current" {}

# 試験の DB。本番と同じ種類・版・容量・ディスクの種類（db.t4g.micro・MySQL 8.4.9・gp2 20GB）、
# 既定のパラメータグループ。中身は空で、合成データを入れる（本番のスナップショットは使わない）。
resource "random_password" "db" {
  length  = 32
  special = false
}

# 複製 EC2 のアプリと、セッションを発行する負荷をかける EC2 で同じ値を使う。本番の値とは別物。
resource "random_password" "auth_secret" {
  length  = 48
  special = false
}

resource "aws_db_subnet_group" "this" {
  name       = local.name
  subnet_ids = [aws_subnet.private_a.id, aws_subnet.private_b.id]
}

resource "aws_db_instance" "db" {
  identifier             = "${local.name}-db"
  engine                 = "mysql"
  engine_version         = "8.4.9"
  instance_class         = "db.t4g.micro"
  allocated_storage      = 20
  storage_type           = "gp2"
  db_name                = "juken_map"
  username               = "admin"
  password               = random_password.db.result
  multi_az               = false
  publicly_accessible    = false
  availability_zone      = var.primary_az
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  storage_encrypted      = true

  # 使い捨てなので、自動バックアップも最後のスナップショットも取らない
  # （本番は1日保持。作成直後の初回バックアップが投入中の IO に混ざるのも避ける）。
  backup_retention_period  = 0
  skip_final_snapshot      = true
  delete_automated_backups = true
  deletion_protection      = false
  apply_immediately        = true
}

# 2台に渡す設定。値は state に入る（terraform/loadtest の state は gitignore 済み）。
resource "aws_ssm_parameter" "env" {
  name        = "/${local.name}/env"
  description = "juken-map loadtest: DATABASE_URL and BETTER_AUTH_SECRET (not production values)"
  type        = "SecureString"
  value       = <<-EOT
    DATABASE_URL=mysql://admin:${random_password.db.result}@${aws_db_instance.db.address}:3306/juken_map
    BETTER_AUTH_SECRET=${random_password.auth_secret.result}
  EOT
}

# 結果の置き場。EC2 は試験後に消えるので、jsonl をここへ上げてから手元へ落とす。
resource "aws_s3_bucket" "results" {
  bucket        = "${local.name}-results-${data.aws_caller_identity.current.account_id}"
  force_destroy = true
}

resource "aws_s3_bucket_public_access_block" "results" {
  bucket                  = aws_s3_bucket.results.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}
