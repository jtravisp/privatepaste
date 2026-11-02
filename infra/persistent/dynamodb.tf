# The only stateful resource. Pastes live here whether or not a cluster
# exists, so destroying the cluster never loses data.
#
# On-demand billing: no traffic, no cost beyond pennies of storage.
# TTL: DynamoDB deletes items whose `ttl` attribute is in the past (usually
# within a few days; the app also refuses to serve expired items).

resource "aws_dynamodb_table" "pastes" {
  #checkov:skip=CKV_AWS_28:No point-in-time recovery by design: backups would keep burned and deleted pastes for 35 days, defeating burn-after-read
  #checkov:skip=CKV_AWS_119:Encrypted at rest with an AWS-owned key by default, and paste content is already encrypted in the browser; a customer-managed key would protect ciphertext
  name         = var.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  # Destroying the table should be a deliberate two-step act.
  deletion_protection_enabled = true
}
