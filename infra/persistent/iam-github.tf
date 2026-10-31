# --- GitHub Actions -> AWS, with no stored credentials ----------------------
#
# GitHub issues each workflow run a short-lived OIDC token. AWS trusts GitHub
# as an identity provider, and the role below accepts tokens only from this
# repo's main branch. The workflow exchanges the token for about an hour of
# AWS credentials (sts:AssumeRoleWithWebIdentity). Nothing long-lived exists
# to leak.

resource "aws_iam_openid_connect_provider" "github" {
  url            = "https://token.actions.githubusercontent.com"
  client_id_list = ["sts.amazonaws.com"]
  # AWS no longer validates these for GitHub (it trusts the issuer's CA), but
  # the API still accepts them.
  thumbprint_list = ["6938fd4d98bab03faadb97b34396831e3780aea1", "1c58a3a8518e8759bf075b76b750d4f2df264fcd"]
}

data "aws_iam_policy_document" "github_trust" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    # Only workflows running on main. A workflow on a feature branch, or a
    # pull request from a fork, presents a different `sub` and is refused.
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:sub"
      values   = ["repo:${var.github_repo}:ref:refs/heads/main"]
    }
  }
}

resource "aws_iam_role" "github_deploy" {
  name                 = "${var.project_name}-gha-deploy"
  description          = "Assumed by GitHub Actions to create, deploy to and destroy the ephemeral EKS cluster."
  assume_role_policy   = data.aws_iam_policy_document.github_trust.json
  max_session_duration = 3600
}

locals {
  account_id  = data.aws_caller_identity.current.account_id
  eks_cluster = var.project_name
  eks_arn     = "arn:aws:eks:${var.aws_region}:${local.account_id}"
}

# What the pipeline may do. The shape of this policy is the design: every IAM
# role the cluster needs is created HERE, by a human, so the pipeline never
# needs iam:CreateRole -- it can only pass these specific roles to EKS. A
# compromised workflow can create and destroy one cluster; it cannot grant
# itself new permissions.
data "aws_iam_policy_document" "github_deploy" {
  # Terraform state: the eks/ prefix only. CI cannot read or alter this
  # persistent stack's state.
  statement {
    sid       = "StateList"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::privatepaste-terraform-state"]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["eks/*"]
    }
  }
  statement {
    sid       = "StateReadWrite"
    actions   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
    resources = ["arn:aws:s3:::privatepaste-terraform-state/eks/*"]
  }

  # The cluster itself, by name. Covers its node groups, add-ons, access
  # entries and Pod Identity associations (all ARNs under the cluster name).
  statement {
    sid     = "EksThisCluster"
    actions = ["eks:*"]
    resources = [
      "${local.eks_arn}:cluster/${local.eks_cluster}",
      "${local.eks_arn}:nodegroup/${local.eks_cluster}/*",
      "${local.eks_arn}:addon/${local.eks_cluster}/*",
      "${local.eks_arn}:access-entry/${local.eks_cluster}/*",
      "${local.eks_arn}:podidentityassociation/${local.eks_cluster}/*",
    ]
  }
  statement {
    sid       = "EksReadAnything"
    actions   = ["eks:Describe*", "eks:List*"]
    resources = ["*"]
  }

  # Networking for the cluster's VPC. EC2 resource-level permissions for VPC
  # creation are awkward (a new VPC has no ARN to name in advance), so this is
  # scoped by action family and region rather than by resource. Tag-based
  # conditions would tighten it further.
  statement {
    sid = "Ec2Networking"
    actions = [
      "ec2:*Vpc*", "ec2:*Subnet*", "ec2:*InternetGateway*", "ec2:*RouteTable*", "ec2:*Route",
      "ec2:*SecurityGroup*", "ec2:CreateTags", "ec2:DeleteTags", "ec2:Describe*",
      "ec2:*NetworkInterface*",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = [var.aws_region]
    }
  }

  # Hand the pre-made roles to EKS, and nothing else.
  statement {
    sid       = "PassClusterRoles"
    actions   = ["iam:PassRole", "iam:GetRole"]
    resources = [aws_iam_role.eks_cluster.arn, aws_iam_role.eks_node.arn, aws_iam_role.app.arn]
  }
  # Lets the EKS stack look up the human SSO admin role to grant it kubectl
  # access, without hardcoding its generated name.
  statement {
    sid       = "FindAdminRole"
    actions   = ["iam:ListRoles"]
    resources = ["*"]
  }

  # The tunnel connector token, read at deploy time.
  statement {
    sid       = "TunnelToken"
    actions   = ["ssm:GetParameter"]
    resources = [aws_ssm_parameter.tunnel_token.arn]
  }
  statement {
    sid       = "DecryptTunnelToken"
    actions   = ["kms:Decrypt"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${var.aws_region}.amazonaws.com"]
    }
  }
}

resource "aws_iam_role_policy" "github_deploy" {
  name   = "ephemeral-eks"
  role   = aws_iam_role.github_deploy.id
  policy = data.aws_iam_policy_document.github_deploy.json
}
