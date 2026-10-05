# EC2が「このロールを引き受けてよい」と定義する信頼ポリシー
resource "aws_iam_role" "ec2_ecr" {
  name = "juken-map-ec2-ecr"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })
}

# デプロイ（deploy-ec2.sh）で juken-map-go を pull するだけの権限。
# 画面のビルドイメージ（juken-map）は CI が Go のイメージへ写すだけで、EC2 は pull しない（JUK-125・JUK-130）。
# AWS 管理ポリシー AmazonEC2ContainerRegistryReadOnly は全リポジトリの一覧・読み取りまで含むので使わない（JUK-82）。
resource "aws_iam_role_policy" "ec2_ecr_pull" {
  name = "juken-map-ecr-pull"
  role = aws_iam_role.ec2_ecr.id

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
          "ecr:GetDownloadUrlForLayer",
        ]
        Resource = [aws_ecr_repository.juken_map_go.arn]
      }
    ]
  })
}

# EC2にロールを貼るための「インスタンスプロファイル」でロールを包む
resource "aws_iam_instance_profile" "ec2_ecr" {
  name = "juken-map-ec2-ecr"
  role = aws_iam_role.ec2_ecr.name
}

# EC2をSystems Manager Session Managerから操作するための権限
resource "aws_iam_role_policy_attachment" "ec2_ssm" {
  role       = aws_iam_role.ec2_ecr.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

# アプリ起動時に本番用シークレット1件だけを取得できる最小権限
resource "aws_iam_role_policy" "ec2_runtime_secret_read" {
  name = "juken-map-runtime-secret-read"
  role = aws_iam_role.ec2_ecr.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect   = "Allow"
        Action   = ["secretsmanager:GetSecretValue"]
        Resource = aws_secretsmanager_secret.app_runtime.arn
      }
    ]
  })
}
