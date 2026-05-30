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
