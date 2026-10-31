# The only stateful resource. Pastes live here whether or not a cluster
# exists, so destroying the cluster never loses data.
#
# On-demand billing: no traffic, no cost beyond pennies of storage.
# TTL: DynamoDB deletes items whose `ttl` attribute is in the past (usually
# within a few days; the app also refuses to serve expired items).

resource "aws_dynamodb_table" "pastes" {
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
