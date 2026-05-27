terraform {
  # configure with:
  #   tofu init -backend-config=bucket=YOUR_BUCKET -backend-config=key=platform/docdb/terraform.tfstate -backend-config=region=YOUR_REGION
  backend "s3" {}

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.27"
    }
  }
}

provider "aws" {
  region = var.region
}

data "terraform_remote_state" "eks" {
  backend = "s3"
  config = {
    bucket = var.state_bucket
    key    = "${var.state_key_prefix}/eks/terraform.tfstate"
    region = var.region
  }
}

data "aws_eks_cluster_auth" "primary" {
  name = data.terraform_remote_state.eks.outputs.cluster_name
}

provider "kubernetes" {
  host                   = data.terraform_remote_state.eks.outputs.cluster_endpoint
  cluster_ca_certificate = base64decode(data.terraform_remote_state.eks.outputs.cluster_ca_certificate)
  token                  = data.aws_eks_cluster_auth.primary.token
}

# ── DB Subnet Group ───────────────────────────────────────────────────────────

resource "aws_docdb_subnet_group" "main" {
  name       = "${var.cluster_name}-docdb-subnet-group"
  subnet_ids = data.terraform_remote_state.eks.outputs.private_subnet_ids

  tags = {
    Name = "${var.cluster_name}-docdb-subnet-group"
  }
}

# ── Security Group ────────────────────────────────────────────────────────────

data "aws_vpc" "main" {
  id = data.terraform_remote_state.eks.outputs.vpc_id
}

resource "aws_security_group" "docdb" {
  name        = "${var.cluster_name}-docdb-sg"
  description = "Allow DocumentDB access from within VPC"
  vpc_id      = data.terraform_remote_state.eks.outputs.vpc_id

  ingress {
    from_port   = 27017
    to_port     = 27017
    protocol    = "tcp"
    cidr_blocks = [data.aws_vpc.main.cidr_block]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.cluster_name}-docdb-sg"
  }
}

# ── DocumentDB Cluster ────────────────────────────────────────────────────────

resource "aws_docdb_cluster" "main" {
  cluster_identifier      = "${var.cluster_name}-docdb"
  engine                  = "docdb"
  master_username         = "docdbadmin"
  master_password         = var.docdb_password
  db_subnet_group_name    = aws_docdb_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.docdb.id]
  skip_final_snapshot     = true
  storage_encrypted       = true

  tags = {
    Name = "${var.cluster_name}-docdb"
  }
}

resource "aws_docdb_cluster_instance" "main" {
  count              = 1
  identifier         = "${var.cluster_name}-docdb-0"
  cluster_id         = aws_docdb_cluster.main.id
  instance_class     = var.docdb_instance_class
}

# ── Kubernetes Secret ─────────────────────────────────────────────────────────

resource "kubernetes_secret" "docdb_credentials" {
  metadata {
    name      = "docdb-credentials"
    namespace = "app-b"
  }

  data = {
    MONGODB_URI      = "mongodb://docdbadmin:${var.docdb_password}@${aws_docdb_cluster.main.endpoint}:27017/app_b?tls=true&tlsCAFile=/etc/ssl/certs/rds-combined-ca-bundle.pem&replicaSet=rs0&readPreference=secondaryPreferred&retryWrites=false"
    MONGODB_DATABASE = "app_b"
  }
}

# ── Outputs ───────────────────────────────────────────────────────────────────

output "docdb_endpoint" {
  description = "DocumentDB cluster endpoint"
  value       = aws_docdb_cluster.main.endpoint
  sensitive   = true
}
