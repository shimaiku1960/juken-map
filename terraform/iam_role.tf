resource "aws_iam_role" "github_actions_ecr" {
  name = "github-actions-juken-map-ecr"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Federated = "arn:aws:iam::961457613174:oidc-provider/token.actions.githubusercontent.com"
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
          }
          StringLike = {
            "token.actions.githubusercontent.com:sub" = "repo:shimaiku1960/juken-map:*"
          }
        }
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "github_actions_ecr" {
  role       = aws_iam_role.github_actions_ecr.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPowerUser"
}

# GitHub Actionsから本番EC2へ、SSM Run Commandでデプロイを実行するための権限。
# 実行可能なドキュメントと対象インスタンスを固定し、任意のEC2への実行を防ぐ。
resource "aws_iam_role_policy" "github_actions_ssm_deploy" {
  name = "juken-map-ssm-deploy"
  role = aws_iam_role.github_actions_ecr.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = "ssm:SendCommand"
        Resource = [
          "arn:aws:ssm:ap-northeast-1::document/AWS-RunShellScript",
          "arn:aws:ec2:ap-northeast-1:961457613174:instance/i-0eeb166295363e11d"
        ]
      },
      {
        Effect   = "Allow"
        Action   = "ssm:GetCommandInvocation"
        Resource = "*"
      }
    ]
  })
}
