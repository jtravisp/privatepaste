# --- Who can use kubectl, and what the app can do in AWS -------------------

# The human admin: the account's IAM Identity Center (SSO) AdministratorAccess
# role. Its name has a generated suffix, so look it up instead of hardcoding.
#
# Gotcha: SSO role ARNs carry a path (/aws-reserved/sso.amazonaws.com/), but
# EKS access entries must use the ARN WITHOUT the path, or kubectl gets
# "Unauthorized". Hence the replace().
data "aws_iam_roles" "sso_admin" {
  name_regex  = "^AWSReservedSSO_AdministratorAccess_.*"
  path_prefix = "/aws-reserved/sso.amazonaws.com/"
}

locals {
  admin_principal_arns = [
    for arn in data.aws_iam_roles.sso_admin.arns :
    replace(arn, "/aws-reserved/sso.amazonaws.com/", "/")
  ]
}

resource "aws_eks_access_entry" "admin" {
  for_each      = toset(local.admin_principal_arns)
  cluster_name  = aws_eks_cluster.this.name
  principal_arn = each.value
}

resource "aws_eks_access_policy_association" "admin" {
  for_each      = toset(local.admin_principal_arns)
  cluster_name  = aws_eks_cluster.this.name
  principal_arn = each.value
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_access_entry.admin]
}

# EKS Pod Identity: pods running as service account privatepaste/privatepaste
# receive temporary credentials for the app role (DynamoDB on one table).
#
# How it works: the agent DaemonSet listens on a link-local address on each
# node; EKS injects AWS_CONTAINER_CREDENTIALS_FULL_URI and a projected token
# into matching pods; the AWS SDK's default credential chain finds them. The
# app code has no credential handling at all.
#
# Compared with IRSA (the older mechanism): no per-cluster OIDC provider, and
# the role's trust policy does not name the cluster -- which matters when the
# cluster is recreated every session with a new OIDC issuer URL.
resource "aws_eks_pod_identity_association" "app" {
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "privatepaste"
  service_account = "privatepaste"
  role_arn        = data.aws_iam_role.app.arn
}
