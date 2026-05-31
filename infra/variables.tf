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

variable "image_tag" {
   description = "Tag for the Docker image to deploy"
   type        = string
   default     = "v1"
}

variable "domain_name" {
  description = "Domain name for the application"
  type        = string
}

variable "hosted_zone_id" {
  description = "Route 53 hosted zone ID for the domain"
  type        = string
}

variable "cloudflare_api_token" {
  description = "Cloudflare API token"
  type        = string
  sensitive   = true
}

variable "github_repo" {
  description = "GitHub repository for the application (owner/repo)"
  type        = string
}
