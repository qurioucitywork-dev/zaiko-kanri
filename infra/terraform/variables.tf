variable "project_name" {
  description = "Resource name prefix."
  type        = string
  default     = "zaiko-kanri"
}

variable "environment" {
  description = "Deployment environment name."
  type        = string
  default     = "staging"
}

variable "aws_region" {
  description = "AWS region for ECS, RDS, and S3."
  type        = string
  default     = "ap-northeast-1"
}

variable "vpc_id" {
  description = "Existing VPC ID."
  type        = string
}

variable "public_subnet_ids" {
  description = "Public subnets for the ALB."
  type        = list(string)
}

variable "private_subnet_ids" {
  description = "Private subnets with NAT access for ECS and RDS."
  type        = list(string)
}

variable "container_image" {
  description = "Immutable ECR image URI including tag or digest."
  type        = string
}

variable "enable_ecs_service" {
  description = "Keep false until the PostgreSQL cutover checklist is complete."
  type        = bool
  default     = false
}

variable "desired_count" {
  description = "Number of Fargate tasks after service enablement."
  type        = number
  default     = 2
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_multi_az" {
  description = "Enable Multi-AZ for production."
  type        = bool
  default     = false
}

variable "deletion_protection" {
  description = "Protect RDS and S3 from accidental deletion in production."
  type        = bool
  default     = true
}
