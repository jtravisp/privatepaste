variable "project_name" {
  description = "Name of the project for tagging resources"
  type        = string
  default     = ""
}

variable "table_arn" {
  description = "ARN of the DynamoDB table for IAM policy"
  type        = string
  default     = ""
}   

variable "github_repo" {
  description = "GitHub repository for OIDC trust relationship (format: owner/repo)"
  type        = string
  default     = ""
}

variable "ecr_repository_arn" {
  description = "ARN of the ECR repository for IAM policy"
  type        = string
  default     = ""
}

variable "ecs_task_execution_role_arn" {
  description = "ARN of the IAM role for ECS task execution"
  type        = string
  default     = ""
}

variable "ecs_task_role_arn" {
  description = "ARN of the IAM role for ECS tasks"
  type        = string
  default     = ""
}
