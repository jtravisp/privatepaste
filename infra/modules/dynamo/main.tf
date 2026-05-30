resource "aws_dynamodb_table" "main" {
  name           = var.table_name
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "id"
  attribute {
    name = "id"
    type = "S"
  }
  ttl {
  attribute_name = "ttl"
  enabled        = true
 }

  tags = {
    Name = "${var.project_name}-dynamodb-table"
  }
}
