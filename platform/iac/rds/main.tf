terraform {
  # configure with:
  #   tofu init -backend-config=bucket=YOUR_BUCKET -backend-config=key=platform/rds/terraform.tfstate -backend-config=region=YOUR_REGION
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

resource "aws_db_subnet_group" "main" {
  name       = "${var.cluster_name}-rds-subnet-group"
  subnet_ids = data.terraform_remote_state.eks.outputs.private_subnet_ids

  tags = {
    Name = "${var.cluster_name}-rds-subnet-group"
  }
}

# ── Security Group ────────────────────────────────────────────────────────────

data "aws_vpc" "main" {
  id = data.terraform_remote_state.eks.outputs.vpc_id
}

resource "aws_security_group" "rds" {
  name        = "${var.cluster_name}-rds-sg"
  description = "Allow PostgreSQL access from within VPC"
  vpc_id      = data.terraform_remote_state.eks.outputs.vpc_id

  ingress {
    from_port   = 5432
    to_port     = 5432
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
    Name = "${var.cluster_name}-rds-sg"
  }
}

# ── RDS: app-a ────────────────────────────────────────────────────────────────

resource "aws_db_instance" "app_a" {
  identifier              = "${var.cluster_name}-app-a"
  db_name                 = "app_a"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = var.db_instance_class
  allocated_storage       = 20
  storage_type            = "gp3"
  username                = "postgres"
  password                = var.db_password
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.rds.id]
  publicly_accessible     = false
  skip_final_snapshot     = true
  deletion_protection     = false

  tags = {
    Name = "${var.cluster_name}-app-a"
  }
}

# ── RDS: app-c ────────────────────────────────────────────────────────────────

resource "aws_db_instance" "app_c" {
  identifier              = "${var.cluster_name}-app-c"
  db_name                 = "app_c"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = var.db_instance_class
  allocated_storage       = 20
  storage_type            = "gp3"
  username                = "postgres"
  password                = var.db_password
  db_subnet_group_name    = aws_db_subnet_group.main.name
  vpc_security_group_ids  = [aws_security_group.rds.id]
  publicly_accessible     = false
  skip_final_snapshot     = true
  deletion_protection     = false

  tags = {
    Name = "${var.cluster_name}-app-c"
  }
}

# ── Kubernetes Secrets ────────────────────────────────────────────────────────

resource "kubernetes_secret" "db_credentials_app_a" {
  metadata {
    name      = "db-credentials"
    namespace = "app-a"
  }

  data = {
    DB_HOST     = aws_db_instance.app_a.address
    DB_PORT     = tostring(aws_db_instance.app_a.port)
    DB_USER     = aws_db_instance.app_a.username
    DB_PASSWORD = var.db_password
    DB_NAME     = aws_db_instance.app_a.db_name
    DB_SSLMODE  = "require"
  }
}

resource "kubernetes_secret" "db_credentials_app_c" {
  metadata {
    name      = "db-credentials"
    namespace = "app-c"
  }

  data = {
    DB_HOST     = aws_db_instance.app_c.address
    DB_PORT     = tostring(aws_db_instance.app_c.port)
    DB_USER     = aws_db_instance.app_c.username
    DB_PASSWORD = var.db_password
    DB_NAME     = aws_db_instance.app_c.db_name
    DB_SSLMODE  = "require"
  }
}

# ── Outputs ───────────────────────────────────────────────────────────────────

output "app_a_db_host" {
  description = "app-a RDS endpoint"
  value       = aws_db_instance.app_a.address
  sensitive   = true
}

output "app_c_db_host" {
  description = "app-c RDS endpoint"
  value       = aws_db_instance.app_c.address
  sensitive   = true
}
