variable "project_name" {
  description = "Name of the project for tagging resources"
  type        = string
}

variable "domain_name" {
  description = "Domain name for the ACM certificate and Route 53 records"
  type        = string
}

variable "hosted_zone_id" {
  description = "Route 53 hosted zone ID for the domain"
  type        = string
}
