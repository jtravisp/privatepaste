variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "project_name" {
  description = "Prefix for resource names. The EKS stack and workflows assume 'privatepaste'."
  type        = string
  default     = "privatepaste"
}

variable "table_name" {
  type    = string
  default = "privatepaste-prod"
}

variable "github_repo" {
  description = "owner/repo allowed to assume the deploy role via GitHub OIDC."
  type        = string
  default     = "jtravisp/privatepaste"
}

variable "domain_name" {
  description = "Public hostname; must be a zone in the Cloudflare account."
  type        = string
  default     = "privatepasteapp.com"
}

variable "cloudflare_api_token" {
  description = "Needs Zone:Zone:Read and Zone:DNS:Edit on the zone, and Account:Cloudflare Tunnel:Edit."
  type        = string
  sensitive   = true
}

variable "budget_email" {
  description = "Where AWS Budgets sends cost alerts."
  type        = string
}

variable "monthly_budget_usd" {
  description = "Alert threshold. Normal use (a few cluster sessions a week) is a few dollars."
  type        = number
  default     = 10
}
