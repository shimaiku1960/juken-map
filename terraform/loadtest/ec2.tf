# ---- 複製 EC2（本番 EC2 の AMI から起動）----
# OS・nginx・Swap・Docker・Alloy・デプロイ済みのイメージまで本番と同じ。
# 本番の設定のまま動き出すが、置き場所（private_a）と SG で外へ出られない。
# 試験用の設定への差し替えは scripts/loadtest-aws/app-prepare.sh を SSM で送って行う。
resource "aws_instance" "app" {
  ami                         = var.source_ami_id
  instance_type               = "t3.micro"
  subnet_id                   = aws_subnet.private_a.id
  vpc_security_group_ids      = [aws_security_group.app.id]
  iam_instance_profile        = aws_iam_instance_profile.app.name
  associate_public_ip_address = false

  # 本番は credit_specification を指定していない（既定の Unlimited）。明示して揃える。
  credit_specification {
    cpu_credits = "unlimited"
  }
  # 段階ごとの CPU を1分単位で見る。
  monitoring = true

  instance_initiated_shutdown_behavior = "terminate"
  user_data                            = <<-EOT
    #!/bin/bash
    shutdown -h +${var.auto_terminate_minutes} "juken-map-loadtest: auto terminate"
  EOT

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    delete_on_termination = true
  }
  volume_tags = {
    Name    = "${local.name}-app"
    Project = "juken-map-loadtest"
  }

  tags = { Name = "${local.name}-app" }

  # RDS より先に動き出しても害は無いが、app-prepare.sh が DB を待たずに済むようにする。
  depends_on = [aws_db_instance.db, aws_vpc_endpoint.ssm]
}

# ---- 負荷をかける EC2 ----
data "aws_ssm_parameter" "ubuntu" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

resource "aws_instance" "driver" {
  ami                    = data.aws_ssm_parameter.ubuntu.insecure_value
  instance_type          = var.driver_instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.driver.id]
  iam_instance_profile   = aws_iam_instance_profile.driver.name
  monitoring             = true

  instance_initiated_shutdown_behavior = "terminate"
  user_data_replace_on_change          = true
  user_data = join("\n", [
    "#!/bin/bash",
    "export REPO_REF='${var.repo_ref}' PARAM_NAME='${aws_ssm_parameter.env.name}' RESULTS_BUCKET='${aws_s3_bucket.results.bucket}' AUTO_TERMINATE_MINUTES='${var.auto_terminate_minutes}'",
    file("${path.module}/../../scripts/loadtest-aws/driver-setup.sh"),
  ])

  metadata_options {
    http_tokens = "required"
  }

  root_block_device {
    volume_size           = 30
    volume_type           = "gp3"
    delete_on_termination = true
  }
  volume_tags = {
    Name    = "${local.name}-driver"
    Project = "juken-map-loadtest"
  }

  tags = { Name = "${local.name}-driver" }
}
