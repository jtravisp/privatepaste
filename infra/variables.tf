 variable "aws_region" {
    type    = string
    default = "us-east-1"
 }
 
variable "vpc_cidr" {
   description = "CIDR block for the VPC"
   type        = string
   default     = "10.0.0.0/16"
}

variable "project_name" {
   description = "Name of the project for tagging resources"
   type        = string
   default     = ""
}

variable "table_name" {
   description = "Name of the DynamoDB table"
   type        = string
   default     = ""
}  
