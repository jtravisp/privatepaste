variable "table_name" {
   description = "Name of the DynamoDB table"
   type        = string
   default     = "privatepaste-prod"
}

variable "project_name" {
   description = "Name of the project for tagging resources"
   type        = string
   default     = ""
}
