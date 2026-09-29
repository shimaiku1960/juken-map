resource "aws_ecr_repository" "juken_map" {
  name = "juken-map"
}

resource "aws_ecr_lifecycle_policy" "juken_map" {
  repository = aws_ecr_repository.juken_map.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images, expire older"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}

# Go の API（apps/api-go、JUK-72）のイメージ。Node のイメージと同じコミットのタグで並べて置く。
# 同じリポジトリにタグを分けて置くと、下の「最新10件」を Node と Go で分け合うことになり、
# 戻せるデプロイの数が半分になるので分ける。
resource "aws_ecr_repository" "juken_map_go" {
  name = "juken-map-go"
}

resource "aws_ecr_lifecycle_policy" "juken_map_go" {
  repository = aws_ecr_repository.juken_map_go.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Keep last 10 images, expire older"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = 10
        }
        action = {
          type = "expire"
        }
      }
    ]
  })
}
