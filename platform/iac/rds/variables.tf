variable "region" {
  description = "AWS region"
  type        = string
  default     = "eu-west-1"
}

variable "cluster_name" {
  description = "EKS cluster name"
  type        = string
  default     = "devops-lab"
}

variable "state_bucket" {
  description = "S3 bucket name for remote state"
  type        = string
}

variable "state_key_prefix" {
  description = "Key prefix for remote state paths"
  type        = string
  default     = "platform"
}

variable "db_password" {
  description = "Master password for RDS instances"
  type        = string
  sensitive   = true
}

variable "db_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.micro"
}
