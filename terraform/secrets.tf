resource "aws_secretsmanager_secret" "app_runtime" {
  name                    = "juken-map/production/runtime"
  description             = "Runtime secrets injected into the juken-map production container"
  recovery_window_in_days = 7

  tags = {
    Application = "juken-map"
    Environment = "production"
  }
}

# シークレット値はTerraform stateへ保存しない。AWSコンソールまたはCLIで別途登録する。
