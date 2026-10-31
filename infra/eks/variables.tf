variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "cluster_name" {
  description = "Must match the name the pipeline's IAM policy is scoped to (persistent stack)."
  type        = string
  default     = "privatepaste"
}

variable "kubernetes_version" {
  description = "Keep this in standard support; extended support costs 6x per hour."
  type        = string
  default     = "1.36"
}

variable "expires_at" {
  description = "RFC 3339 UTC time after which the reaper destroys this stack, e.g. 2026-11-01T02:30:00Z."
  type        = string

  validation {
    condition     = can(formatdate("YYYY", var.expires_at))
    error_message = "expires_at must be an RFC 3339 timestamp, e.g. 2026-11-01T02:30:00Z."
  }
}

variable "vpc_cidr" {
  type    = string
  default = "10.20.0.0/16"
}

variable "node_instance_types" {
  description = "Several similar types, so a Spot shortage in one does not block the node group."
  type        = list(string)
  default     = ["t3.medium", "t3a.medium"]
}

variable "node_count" {
  description = "Two nodes, so the app's two replicas can sit on different machines."
  type        = number
  default     = 2
}
