# The long-lived stack: everything that must survive between clusters.
# Applied by a human from a laptop (AWS_PROFILE=privatepaste), rarely.
# CI never touches this state -- its IAM role can only read/write eks/*.

terraform {
  required_version = ">= 1.11.0"

  backend "s3" {
    bucket       = "privatepaste-terraform-state"
    key          = "persistent/terraform.tfstate"
    region       = "us-east-1"
    use_lockfile = true # S3-native state locking; no DynamoDB lock table
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.100"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.52"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      project    = var.project_name
      stack      = "persistent"
      managed-by = "terraform"
    }
  }
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

data "aws_caller_identity" "current" {}
