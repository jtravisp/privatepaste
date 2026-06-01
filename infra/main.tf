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
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }

  required_version = ">= 1.11.0"
}

provider "aws" {
  region  = var.aws_region
  profile = "privatepaste"
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}

module "networking" {
  source = "./modules/networking"

  vpc_cidr = var.vpc_cidr
  project_name = var.project_name
}

module "dynamodb" {
  source = "./modules/dynamo"

  table_name = var.table_name
  project_name = var.project_name
} 

module "ecr" {
  source = "./modules/ecr"

  project_name = var.project_name
} 

module "iam" {
  source = "./modules/iam"

  project_name = var.project_name
  table_arn    = module.dynamodb.table_arn
  github_repo = var.github_repo
  ecr_repository_arn = module.ecr.repository_arn
} 

module "ecs" {
  source = "./modules/ecs"

  project_name = var.project_name
  aws_region = var.aws_region
  subnet_ids = module.networking.subnet_ids
  app_image = "${module.ecr.repository_url}:placeholder"
  ecs_tasks_security_group_id = module.networking.ecs_tasks_security_group_id
  ecs_task_execution_role_arn = module.iam.ecs_task_execution_role_arn
  ecs_task_role_arn = module.iam.ecs_task_role_arn
  dynamo_table_name = module.dynamodb.table_name
  target_group_arn = module.alb.target_group_arn
} 

module "dns" {
  source = "./modules/dns"

  project_name   = var.project_name
  domain_name    = var.domain_name
}

module "alb" {
  source = "./modules/alb"

  project_name          = var.project_name
  subnet_ids            = module.networking.subnet_ids
  alb_security_group_id = module.networking.alb_security_group_id
  vpc_id                = module.networking.vpc_id
  certificate_arn       = module.dns.certificate_arn
}
