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
          # 引き受けられるのは main のワークフローだけ（deploy.yml の push と手動実行）。
          # repo:...:* のままだと、どのブランチに置いたワークフローからでも本番へ届く。
          StringEquals = {
            "token.actions.githubusercontent.com:aud" = "sts.amazonaws.com"
            "token.actions.githubusercontent.com:sub" = "repo:shimaiku1960/juken-map:ref:refs/heads/main"
          }
        }
      }
    ]
  })
}

# CI が ECR でやるのは、ログインと juken-map リポジトリへのイメージの push だけ
# （BatchGetImage は push のときにマニフェストの有無を確かめるために使う）。
# 以前は AWS 管理ポリシー AmazonEC2ContainerRegistryPowerUser（全リポジトリの読み書き）だった。
# 本番の EC2 が pull するのは EC2 のロール（iam_ec2.tf）なので、ここに pull は要らない。
resource "aws_iam_role_policy" "github_actions_ecr_push" {
  name = "juken-map-ecr-push"
  role = aws_iam_role.github_actions_ecr.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        # ログイン用のトークンはリポジトリ単位に絞れない。
        Effect   = "Allow"
        Action   = "ecr:GetAuthorizationToken"
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ecr:BatchCheckLayerAvailability",
          "ecr:BatchGetImage",
          "ecr:InitiateLayerUpload",
          "ecr:UploadLayerPart",
          "ecr:CompleteLayerUpload",
          "ecr:PutImage",
        ]
        Resource = aws_ecr_repository.juken_map.arn
      }
    ]
  })
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
