# The ephemeral stack: one EKS cluster and the VPC it sits in. Created by the
# cluster-up workflow, destroyed by cluster-down or the reaper. Holds no data;
# destroying it loses nothing (pastes live in DynamoDB, in the persistent
# stack).
#
# It reads the persistent stack's resources by NAME (data sources), not
# through terraform_remote_state. That keeps CI's role out of the persistent
# state file entirely.

terraform {
  required_version = ">= 1.11.0"

  backend "s3" {
    bucket       = "privatepaste-terraform-state"
    key          = "eks/terraform.tfstate"
    region       = "us-east-1"
    use_lockfile = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.100"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      project    = "privatepaste"
      stack      = "eks"
      managed-by = "terraform"
      # On every taggable resource, so Cost Explorer and the console show
      # when this stack is meant to be gone.
      expires-at = var.expires_at
    }
  }
}
