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
