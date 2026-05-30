resource "aws_ecr_repository" "main" {
  name = "${var.project_name}-repository"
  image_tag_mutability = "IMMUTABLE"

  tags = {
    Name = "${var.project_name}-repository"
  }
}   

resource "aws_ecr_lifecycle_policy" "main" {
  repository = aws_ecr_repository.main.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Expire untagged images after 7 days"
      selection = {
        tagStatus   = "untagged"
        countType   = "sinceImagePushed"
        countUnit   = "days"
        countNumber = 7
      }
      action = { type = "expire" }
    }]
  })
}
