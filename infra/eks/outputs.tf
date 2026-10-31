output "cluster_name" {
  value = aws_eks_cluster.this.name
}

output "expires_at" {
  value = var.expires_at
}

output "kubeconfig_command" {
  value = "aws eks update-kubeconfig --region ${var.aws_region} --name ${aws_eks_cluster.this.name}"
}
