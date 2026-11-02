# --- The EKS control plane --------------------------------------------------
#
# AWS runs the Kubernetes API server and etcd across three availability zones
# and bills $0.10/hour for them. That hourly charge is why this cluster is
# ephemeral: it cannot be paused, only deleted.

data "aws_iam_role" "cluster" {
  name = "${var.cluster_name}-eks-cluster"
}

data "aws_iam_role" "node" {
  name = "${var.cluster_name}-eks-node"
}

data "aws_iam_role" "app" {
  name = "${var.cluster_name}-app"
}

resource "aws_eks_cluster" "this" {
  #checkov:skip=CKV_AWS_39:Public endpoint so GitHub-hosted runners (no fixed IPs) can deploy; every request still needs IAM plus an access entry (docs/architecture.md section 14)
  #checkov:skip=CKV_AWS_38:Same as CKV_AWS_39; runner IPs aren't fixed, so there's no CIDR to restrict to
  #checkov:skip=CKV_AWS_37:Control-plane logs go to CloudWatch at a cost; not worth it for a 2.5-hour cluster. A real account would enable at least audit
  #checkov:skip=CKV_AWS_58:EKS envelope-encrypts Kubernetes API data with an AWS-owned key by default; a customer-managed key adds cost and lifecycle for one Secret whose source of truth is SSM
  #checkov:skip=CKV_AWS_339:False positive: Checkov's version list lags EKS. upgrade_policy STANDARD below makes AWS refuse unsupported versions
  name     = var.cluster_name
  version  = var.kubernetes_version
  role_arn = data.aws_iam_role.cluster.arn

  vpc_config {
    subnet_ids = aws_subnet.public[*].id
    # The API endpoint is public so GitHub-hosted runners (no fixed IPs) can
    # reach it. Every request still needs a valid IAM identity that has an
    # access entry -- the endpoint being reachable is not the same as it
    # being open.
    endpoint_public_access  = true
    endpoint_private_access = true
  }

  access_config {
    # Access entries (EKS API) instead of the legacy aws-auth ConfigMap:
    # who can use kubectl is managed by Terraform and visible in the AWS API.
    authentication_mode = "API"
    # Whoever creates the cluster (the pipeline's role) becomes cluster admin,
    # which is what lets the same workflow deploy to it afterwards.
    bootstrap_cluster_creator_admin_permissions = true
  }

  # Install VPC CNI, kube-proxy and CoreDNS as managed add-ons below, rather
  # than letting EKS install unmanaged copies.
  bootstrap_self_managed_addons = false

  # Fail rather than silently roll into extended support ($0.60/hour instead
  # of $0.10) if the version ages out.
  upgrade_policy {
    support_type = "STANDARD"
  }
}

# --- Worker nodes: a managed node group on Spot ----------------------------
#
# Managed node group: EKS creates the Auto Scaling group and launch template,
# picks the EKS-optimized AMI, and drains nodes gracefully on updates.
# Spot: about 70% cheaper than on-demand; AWS can reclaim an instance with
# two minutes' notice. With two replicas and a PodDisruptionBudget, losing a
# node means a brief reschedule, not an outage -- an acceptable trade for a
# demo cluster, and a good one to be able to explain.

resource "aws_eks_node_group" "default" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "default"
  node_role_arn   = data.aws_iam_role.node.arn
  subnet_ids      = aws_subnet.public[*].id

  capacity_type  = "SPOT"
  instance_types = var.node_instance_types
  ami_type       = "AL2023_x86_64_STANDARD"
  disk_size      = 20

  scaling_config {
    desired_size = var.node_count
    min_size     = var.node_count
    max_size     = var.node_count + 1 # room for a surge node during updates
  }

  update_config {
    max_unavailable = 1
  }

  # Nodes cannot become Ready without the CNI, and pods cannot get AWS
  # credentials without the Pod Identity agent.
  depends_on = [
    aws_eks_addon.vpc_cni,
    aws_eks_addon.kube_proxy,
    aws_eks_addon.pod_identity_agent,
  ]
}

# --- Managed add-ons --------------------------------------------------------
# No addon_version given: EKS picks the default version for the cluster's
# Kubernetes version.

resource "aws_eks_addon" "vpc_cni" {
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "vpc-cni" # gives every pod a real VPC IP address
}

resource "aws_eks_addon" "kube_proxy" {
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "kube-proxy" # programs Service virtual IPs on each node
}

resource "aws_eks_addon" "pod_identity_agent" {
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent" # serves IAM credentials to pods
}

resource "aws_eks_addon" "coredns" {
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "coredns" # cluster DNS; its pods need nodes to run on
  depends_on   = [aws_eks_node_group.default]
}
