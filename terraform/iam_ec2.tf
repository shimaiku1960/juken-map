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

# ロールにECR読み取り権限（AWS管理ポリシー）を付与
resource "aws_iam_role_policy_attachment" "ec2_ecr_read" {
  role       = aws_iam_role.ec2_ecr.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

# EC2にロールを貼るための「インスタンスプロファイル」でロールを包む
resource "aws_iam_instance_profile" "ec2_ecr" {
  name = "juken-map-ec2-ecr"
  role = aws_iam_role.ec2_ecr.name
}