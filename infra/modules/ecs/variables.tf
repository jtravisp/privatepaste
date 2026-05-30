variable "project_name" {
  description = "Name of the project for tagging resources"
  type        = string
  default     = "privatepaste"
}       

variable "app_image" {
  description = "URI of the Docker image for the application"
  type        = string
  default     = ""
}

variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "subnet_ids" {
  description = "List of subnet IDs for the ECS tasks"
  type        = list(string)
}   

variable "ecs_tasks_security_group_id" {
  description = "Security group ID for the ECS tasks"
  type        = string
}

variable "ecs_task_execution_role_arn" {
  description = "ARN of the IAM role for ECS task execution"
  type        = string
}

variable "ecs_task_role_arn" {
  description = "ARN of the IAM role for ECS tasks"
  type        = string
}

variable "dynamo_table_name" {
  description = "Name of the DynamoDB table for environment variable"
  type        = string
}

variable "target_group_arn" {
  description = "ARN of the ALB target group for ECS service"
  type        = string
}   
