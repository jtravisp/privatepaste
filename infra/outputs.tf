output "alb_dns_name" {
  description = "DNS name of the ALB — use this to access the app"
  value       = module.alb.alb_dns_name
}
