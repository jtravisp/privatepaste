terraform {
  backend "s3" {
    bucket  = "privatepaste-terraform-state"
    key     = "privatepaste/terraform.tfstate"
    region  = "us-east-1"
    profile = "privatepaste"
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.0"
    }
  }

  required_version = ">= 1.11.0"
}

provider "aws" {
  region  = var.aws_region
  profile = "privatepaste"
}
