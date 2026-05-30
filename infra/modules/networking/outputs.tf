output "vpc_id" {
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

output "alb_security_group_id" {
  value = aws_security_group.alb.id
}

output "ecs_tasks_security_group_id" {
  value = aws_security_group.ecs_tasks.id
}
