output "id" {
  value = aws_vpc.main.id
}   

output "subnet_ids" {
  value = aws_subnet.main[*].id
}   

output "igw_id" {
  value = aws_internet_gateway.main.id
}

output "route_table_id" {
  value = aws_route_table.main.id
}
