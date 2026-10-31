# --- VPC: two public subnets, no NAT gateway --------------------------------
#
# The usual production layout is nodes in private subnets reaching the
# internet through a NAT gateway. A NAT gateway costs ~$0.045/hour plus data,
# which would be the single biggest line in this project. Instead, nodes sit
# in public subnets with public IPs and reach ECR, GHCR, DynamoDB and
# Cloudflare directly.
#
# What keeps that safe: nothing listens publicly. The node security group
# (created by EKS) admits traffic only from the cluster itself, there is no
# load balancer, and the only path in from the internet is the outbound
# Cloudflare Tunnel. A public IP with no open port is reachable by nobody.

data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  # EKS requires subnets in at least two availability zones.
  azs = slice(data.aws_availability_zones.available.names, 0, 2)
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true # required for nodes to register with EKS

  tags = { Name = "${var.cluster_name}-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.cluster_name}-igw" }
}

resource "aws_subnet" "public" {
  count = length(local.azs)

  vpc_id                  = aws_vpc.this.id
  availability_zone       = local.azs[count.index]
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index) # two /20s
  map_public_ip_on_launch = true

  tags = { Name = "${var.cluster_name}-public-${local.azs[count.index]}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }

  tags = { Name = "${var.cluster_name}-public" }
}

resource "aws_route_table_association" "public" {
  count          = length(aws_subnet.public)
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}
