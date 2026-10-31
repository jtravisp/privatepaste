# --- Public entry point: Cloudflare Tunnel ----------------------------------
#
# Instead of a public load balancer, a `cloudflared` Deployment inside the
# cluster dials OUT to Cloudflare and holds the connection open. Requests to
# privatepasteapp.com hit Cloudflare's edge (TLS terminates there) and travel
# down that connection to the in-cluster Service.
#
# Consequences:
#   - no inbound ports, no load balancer, no AWS cost for ingress
#   - Kubernetes creates nothing that Terraform cannot see, so destroying the
#     cluster cannot orphan a load balancer that keeps billing
#   - when no cluster is running, Cloudflare answers with error 1033 (HTTP 530)
#
# The tunnel, its routing and the DNS record are permanent and free. Only the
# connector (the pods) comes and goes with the cluster.

data "cloudflare_zone" "this" {
  name = var.domain_name
}

resource "random_id" "tunnel_secret" {
  byte_length = 32
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "this" {
  account_id = data.cloudflare_zone.this.account_id
  name       = var.project_name
  secret     = random_id.tunnel_secret.b64_std
  # Routing is managed here (remotely), not by a config file in the pod.
  config_src = "cloudflare"
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "this" {
  account_id = data.cloudflare_zone.this.account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.this.id

  config {
    ingress_rule {
      hostname = var.domain_name
      # Cluster-internal DNS name of the app's Service (deploy/k8s/base).
      service = "http://privatepaste.privatepaste.svc.cluster.local:80"
    }
    # Required catch-all.
    ingress_rule {
      service = "http_status:404"
    }
  }
}

resource "cloudflare_record" "app" {
  zone_id = data.cloudflare_zone.this.id
  name    = var.domain_name
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.this.id}.cfargotunnel.com"
  proxied = true
  # Replaces the hand-made CNAME that pointed at the old ALB.
  allow_overwrite = true
}

# The connector token, for the cloudflared pods. Stored as an SSM SecureString
# so the deploy workflow can fetch it at deploy time with its AWS role -- no
# copy lives in GitHub secrets or in the repo. (It is also in this stack's
# state file, one reason the state bucket is private.)
resource "aws_ssm_parameter" "tunnel_token" {
  name        = "/${var.project_name}/cloudflared/token"
  description = "Cloudflare Tunnel connector token for the in-cluster cloudflared Deployment."
  type        = "SecureString"
  value       = cloudflare_zero_trust_tunnel_cloudflared.this.tunnel_token
}
