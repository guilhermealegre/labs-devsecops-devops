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

variable "git_repo_url" {
  description = "Git repository URL for ArgoCD applications"
  type        = string
}

variable "git_branch" {
  description = "Git branch for ArgoCD applications"
  type        = string
  default     = "main"
}
