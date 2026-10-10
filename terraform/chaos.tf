# ホストの層の障害注入（カオス段階2、JUK-174）。予告なしのくじ（apps/api/internal/feature/chaos/schedule.go）が、
# CHAOS_HOST=on のときにこのドキュメントを自分の EC2 へ送る。中身は chaos/host-fault.sh（障害はどれも終わる時刻に自分で戻る）。
#
# 引数はここで絞る。Kind・Action は決めた値だけ、Seconds は 60〜1800、DatabaseHost は本番の RDS だけ。
# 強さ（Level）は種類ごとにスクリプトが確かめる。
resource "aws_ssm_document" "chaos_host" {
  name            = "juken-map-chaos-host"
  document_type   = "Command"
  document_format = "JSON"

  content = jsonencode({
    schemaVersion = "2.2"
    description   = "juken-map: host-level fault injection (JUK-174). Every fault reverts itself when its time is up."
    parameters = {
      Action = {
        type          = "String"
        allowedValues = ["start", "revert"]
      }
      Kind = {
        type          = "String"
        default       = "none"
        allowedValues = ["none", "process_kill", "cpu", "memory", "disk", "db_delay", "db_loss"]
      }
      Seconds = {
        type           = "String"
        default        = "60"
        allowedPattern = "^(6[0-9]|[7-9][0-9]|[1-9][0-9]{2}|1[0-7][0-9]{2}|1800)$"
      }
      Level = {
        type           = "String"
        default        = "0"
        allowedPattern = "^[0-9]{1,4}$"
      }
      DatabaseHost = {
        type          = "String"
        default       = aws_db_instance.db.address
        allowedValues = [aws_db_instance.db.address]
      }
    }
    mainSteps = [
      {
        action = "aws:runShellScript"
        name   = "hostFault"
        inputs = {
          runCommand     = split("\n", file("${path.module}/chaos/host-fault.sh"))
          timeoutSeconds = "600"
        }
      }
    ]
  })
}

# EC2 のロール（API のコンテナが IMDS から使う）に、上のドキュメントを自分のインスタンスへ送る権限だけを足す。
# ほかのドキュメント（AWS-RunShellScript など）やほかのインスタンスへは送れない。
resource "aws_iam_role_policy" "ec2_chaos_host" {
  name = "juken-map-chaos-host"
  role = aws_iam_role.ec2_ecr.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = "ssm:SendCommand"
        Resource = [aws_ssm_document.chaos_host.arn, aws_instance.app.arn]
      },
      {
        # コマンドの結果（始められたか）を待つ。コマンド ID 単位に絞れない。
        Effect   = "Allow"
        Action   = "ssm:GetCommandInvocation"
        Resource = "*"
      }
    ]
  })
}
