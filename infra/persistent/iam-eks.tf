# --- Roles the EKS cluster uses ---------------------------------------------
# Created here, once, so the ephemeral stack only references them by name.
# See the github_deploy policy for why that matters.

# The control plane's role: lets the EKS service manage ENIs, security groups
# and so on in this account on the cluster's behalf.
resource "aws_iam_role" "eks_cluster" {
  name = "${var.project_name}-eks-cluster"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "eks.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })
}

resource "aws_iam_role_policy_attachment" "eks_cluster" {
  role       = aws_iam_role.eks_cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

# The worker nodes' role (an EC2 instance profile). Kept minimal: join the
# cluster, let the VPC CNI assign pod IPs, and pull the add-on images
# (kube-proxy, VPC CNI, CoreDNS) that AWS hosts in ECR. It does NOT get
# DynamoDB access -- that belongs to the app's pods alone, via Pod Identity.
resource "aws_iam_role" "eks_node" {
  name = "${var.project_name}-eks-node"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "eks_node" {
  for_each = toset([
    "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
    "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
    "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
  ])
  role       = aws_iam_role.eks_node.name
  policy_arn = each.value
}

# The app's role, assumed by the privatepaste pods through EKS Pod Identity.
# The Pod Identity agent (a DaemonSet on every node) hands these credentials
# only to pods running as the service account named in the association
# (infra/eks), so other pods on the same node cannot use them.
resource "aws_iam_role" "app" {
  name = "${var.project_name}-app"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "pods.eks.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })
}

resource "aws_iam_role_policy" "app_dynamodb" {
  name = "pastes-table"
  role = aws_iam_role.app.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      # Exactly what the code calls. DescribeTable backs the readiness probe.
      Action   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem", "dynamodb:DescribeTable"]
      Resource = aws_dynamodb_table.pastes.arn
    }]
  })
}

# Service-linked roles EKS would otherwise create on first use. Creating them
# here means the pipeline's role never needs iam:CreateServiceLinkedRole.
resource "aws_iam_service_linked_role" "this" {
  for_each         = toset(["eks.amazonaws.com", "eks-nodegroup.amazonaws.com", "autoscaling.amazonaws.com"])
  aws_service_name = each.value
}
