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
  }

  required_version = ">= 1.11.0"
}

provider "aws" {
  region  = var.aws_region
  profile = "privatepaste"
}

module "networking" {
  source = "./modules/networking"

  vpc_cidr = var.vpc_cidr
  project_name = var.project_name
}
