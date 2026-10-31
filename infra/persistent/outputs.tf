output "github_deploy_role_arn" {
  description = "Role the workflows assume (referenced in .github/workflows)."
  value       = aws_iam_role.github_deploy.arn
}

output "table_name" {
  value = aws_dynamodb_table.pastes.name
}

output "tunnel_id" {
  value = cloudflare_zero_trust_tunnel_cloudflared.this.id
}
