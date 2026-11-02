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
  #checkov:skip=CKV_AWS_356:Remaining "*" resources can't be scoped: Describe/List calls and iam:ListRoles don't support resource ARNs, new VPC resources have no ARN yet (actions and region limit them instead), and kms:Decrypt is limited by kms:ViaService
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

  # Networking for the cluster's VPC (infra/eks/network.tf).
  #
  # Resource is "*" because a VPC, subnet or route table that doesn't exist
  # yet has no ARN to name. What limits this statement instead:
  #   - Explicit actions: exactly the calls Terraform makes for the resource
  #     types in network.tf. Wildcards like ec2:*Vpc* also matched VPC
  #     peering and VPC endpoint services, which can open the network to
  #     other accounts.
  #   - Region.
  # Not used: tag conditions (aws:ResourceTag/project). AWS creates the
  # default security group and main route table untagged, and a role that
  # may tag resources can tag its way into the condition, so they add
  # complexity for little gain here. The next step is evidence: generate a
  # policy from CloudTrail with IAM Access Analyzer after a cluster-up/down
  # cycle, and compare it with this list.
  statement {
    sid = "Ec2Networking"
    actions = [
      # aws_vpc
      "ec2:CreateVpc", "ec2:ModifyVpcAttribute", "ec2:DeleteVpc",
      # aws_internet_gateway
      "ec2:CreateInternetGateway", "ec2:AttachInternetGateway",
      "ec2:DetachInternetGateway", "ec2:DeleteInternetGateway",
      # aws_subnet
      "ec2:CreateSubnet", "ec2:ModifySubnetAttribute", "ec2:DeleteSubnet",
      # aws_route_table, aws_route_table_association
      "ec2:CreateRouteTable", "ec2:DeleteRouteTable",
      "ec2:CreateRoute", "ec2:ReplaceRoute", "ec2:DeleteRoute",
      "ec2:AssociateRouteTable", "ec2:DisassociateRouteTable",
      "ec2:ReplaceRouteTableAssociation",
      # aws_default_security_group: strip the default rules
      "ec2:RevokeSecurityGroupIngress", "ec2:RevokeSecurityGroupEgress",
      # Tags, including tag-on-create and the expires-at tag on re-apply
      "ec2:CreateTags", "ec2:DeleteTags",
      # The AWS provider removes leftover ENIs that block subnet deletion
      "ec2:DetachNetworkInterface", "ec2:DeleteNetworkInterface",
    ]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = [var.aws_region]
    }
  }
  # Read-only, and EC2 Describe calls don't support resource-level scoping.
  statement {
    sid       = "Ec2Describe"
    actions   = ["ec2:Describe*"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:RequestedRegion"
      values   = [var.aws_region]
    }
  }

  # Hand the pre-made roles to EKS. PassRole is limited to these three roles.
  statement {
    sid       = "PassClusterRoles"
    actions   = ["iam:PassRole", "iam:GetRole"]
    resources = [aws_iam_role.eks_cluster.arn, aws_iam_role.eks_node.arn, aws_iam_role.app.arn]
  }
  # CreateNodegroup checks the attached policies of both the node role and
  # the cluster role, and EKS makes those IAM calls with OUR credentials
  # (forward access), not its own. Read-only, and on those two roles only.
  statement {
    sid       = "ValidateEksRoles"
    actions   = ["iam:ListAttachedRolePolicies"]
    resources = [aws_iam_role.eks_cluster.arn, aws_iam_role.eks_node.arn]
  }
  # CreateNodegroup also checks, with our credentials, that the node group
  # service-linked role exists. It already does (AWS created it once), so
  # read access is enough; the pipeline still can't create or change it.
  statement {
    sid       = "CheckNodegroupServiceRole"
    actions   = ["iam:GetRole"]
    resources = ["arn:aws:iam::${local.account_id}:role/aws-service-role/eks-nodegroup.amazonaws.com/AWSServiceRoleForAmazonEKSNodegroup"]
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
