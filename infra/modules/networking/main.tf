terraform {
  required_providers {
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.0"
    }
  }
}
data "cloudflare_ip_ranges" "cloudflare" {}

resource "aws_vpc" "main" {
  cidr_block = var.vpc_cidr
  enable_dns_hostnames = true

  tags = {
    Name = "${var.project_name}-vpc"
  }
}   

locals {
  azs = ["us-east-1a", "us-east-1b"]
}

resource "aws_subnet" "main" {
  count = 2
  vpc_id = aws_vpc.main.id
  cidr_block = "10.0.${count.index + 1}.0/24"
  availability_zone = local.azs[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name = "${var.project_name}-subnet-${count.index + 1}"
  }
}   

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id
  
  tags = {
    Name = "${var.project_name}-igw"
  }
}

resource "aws_route_table" "main" {
  vpc_id = aws_vpc.main.id
  
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name = "${var.project_name}-rt"
  }
}   

resource "aws_route_table_association" "main" {
  count = 2
  subnet_id = aws_subnet.main[count.index].id
  route_table_id = aws_route_table.main.id
}

resource "aws_security_group" "alb" {
  name        = "${var.project_name}-sg"
  description = "Allow traffic from Cloudflare only"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = data.cloudflare_ip_ranges.cloudflare.ipv4_cidr_blocks
  }

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = data.cloudflare_ip_ranges.cloudflare.ipv4_cidr_blocks
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.project_name}-alb-sg"
  }
}   

resource "aws_security_group" "ecs_tasks" {
  name        = "${var.project_name}-ecs-sg"
  description = "Allow traffic from ALB to ECS tasks"
  vpc_id      = aws_vpc.main.id

  ingress {
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    security_groups = [aws_security_group.alb.id] 
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]         
  }

  tags = {
  Name = "${var.project_name}-ecs-sg"
  }
}
