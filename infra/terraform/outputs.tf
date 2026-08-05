output "s3_bucket" {
  value = aws_s3_bucket.uploads.id
}

output "rds_endpoint" {
  value = aws_db_instance.main.address
}

output "ecs_cluster" {
  value = aws_ecs_cluster.main.name
}

output "alb_dns_name" {
  value = var.enable_ecs_service ? aws_lb.app[0].dns_name : null
}

output "service_enablement_note" {
  value = var.enable_ecs_service ? "ECS service enabled" : "Blocked intentionally: complete docs/target-architecture.md cutover checklist first"
}
