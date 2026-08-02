resource "aws_db_instance" "db" {
  identifier             = "juken-map-db"
  engine                 = "mysql"
  engine_version         = "8.4.9"
  instance_class         = "db.t4g.micro"
  allocated_storage      = 20
  storage_type           = "gp2"
  db_name                = "juken_map"
  username               = "admin"
  multi_az               = false
  publicly_accessible    = false
  db_subnet_group_name   = "default-vpc-096363ecaa36fd99b"
  vpc_security_group_ids = [aws_security_group.default.id]

  # 破棄時にスナップショットを取らない設定（planには影響しない/destroy時の挙動）
  skip_final_snapshot   = true
  storage_encrypted     = true
  copy_tags_to_snapshot = true
  max_allocated_storage = 1000

}