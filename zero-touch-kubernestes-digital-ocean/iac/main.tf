# =============================================================================
# Zero-Touch Kubernetes Platform on DigitalOcean
# =============================================================================
# This is the ONLY manual command needed. It provisions the DOKS cluster,
# installs ArgoCD, and bootstraps the GitOps pipeline.
#
# Usage:
#   export DIGITALOCEAN_TOKEN="your-token-here"
#   tofu init
#   tofu apply
# =============================================================================

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    digitalocean = {
      source  = "digitalocean/digitalocean"
      version = "~> 2.34"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.25"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.12"
    }
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.0"
    }
  }
}

# =============================================================================
# Provider Configuration
# =============================================================================

provider "digitalocean" {
  # Token is read from DIGITALOCEAN_TOKEN environment variable
}

provider "kubernetes" {
  host                   = digitalocean_kubernetes_cluster.primary.endpoint
  token                  = digitalocean_kubernetes_cluster.primary.kube_config[0].token
  cluster_ca_certificate = base64decode(digitalocean_kubernetes_cluster.primary.kube_config[0].cluster_ca_certificate)
}

provider "helm" {
  kubernetes {
    host                   = digitalocean_kubernetes_cluster.primary.endpoint
    token                  = digitalocean_kubernetes_cluster.primary.kube_config[0].token
    cluster_ca_certificate = base64decode(digitalocean_kubernetes_cluster.primary.kube_config[0].cluster_ca_certificate)
  }
}

provider "kubectl" {
  host                   = digitalocean_kubernetes_cluster.primary.endpoint
  token                  = digitalocean_kubernetes_cluster.primary.kube_config[0].token
  cluster_ca_certificate = base64decode(digitalocean_kubernetes_cluster.primary.kube_config[0].cluster_ca_certificate)
  load_config_file       = false
}

# =============================================================================
# DigitalOcean Kubernetes Cluster
# =============================================================================

resource "digitalocean_kubernetes_cluster" "primary" {
  name    = var.cluster_name
  region  = var.region
  version = var.kubernetes_version

  node_pool {
    name       = "default-pool"
    size       = var.node_size
    node_count = var.node_count

    labels = {
      "environment" = "lab"
      "managed-by"  = "opentofu"
    }
  }

  maintenance_policy {
    start_time = "04:00"
    day        = "sunday"
  }

  tags = ["devops-lab", "zero-touch"]
}

# =============================================================================
# ArgoCD Installation
# =============================================================================

resource "kubernetes_namespace" "argocd" {
  metadata {
    name = "argocd"
  }

  depends_on = [digitalocean_kubernetes_cluster.primary]
}

resource "helm_release" "argocd" {
  name       = "argocd"
  repository = "https://argoproj.github.io/argo-helm"
  chart      = "argo-cd"
  version    = "5.51.6"
  namespace  = kubernetes_namespace.argocd.metadata[0].name

  values = [<<-EOT
    server:
      service:
        type: LoadBalancer
      extraArgs:
        - --insecure
      config:
        repositories: |
          - type: git
            url: ${var.git_repo_url}
    configs:
      params:
        server.insecure: true
      cm:
        application.resourceTrackingMethod: annotation
    EOT
  ]

  wait = true

  depends_on = [kubernetes_namespace.argocd]
}

# =============================================================================
# Application Namespaces
# =============================================================================

resource "kubernetes_namespace" "apps" {
  for_each = toset(["app-a", "app-b", "app-c", "infra", "monitoring"])

  metadata {
    name = each.key
    labels = {
      "managed-by" = "argocd"
    }
  }

  depends_on = [digitalocean_kubernetes_cluster.primary]
}

# =============================================================================
# Bootstrap: Root Application (App of Apps)
# =============================================================================

resource "kubectl_manifest" "root_application" {
  yaml_body = <<-YAML
    apiVersion: argoproj.io/v1alpha1
    kind: Application
    metadata:
      name: root-app
      namespace: argocd
      finalizers:
        - resources-finalizer.argocd.argoproj.io
    spec:
      project: default
      source:
        repoURL: ${var.git_repo_url}
        targetRevision: ${var.git_branch}
        path: apps
      destination:
        server: https://kubernetes.default.svc
        namespace: argocd
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
        syncOptions:
          - CreateNamespace=true
  YAML

  depends_on = [helm_release.argocd]
}

# =============================================================================
# Output: Cluster Access Information
# =============================================================================

output "cluster_endpoint" {
  description = "Kubernetes cluster endpoint"
  value       = digitalocean_kubernetes_cluster.primary.endpoint
  sensitive   = true
}

output "kubeconfig" {
  description = "kubectl configuration"
  value       = digitalocean_kubernetes_cluster.primary.kube_config[0].raw_config
  sensitive   = true
}

output "argocd_server_url" {
  description = "ArgoCD Server URL (get LoadBalancer IP after deployment)"
  value       = "Run: kubectl get svc -n argocd argocd-server -o jsonpath='{.status.loadBalancer.ingress[0].ip}'"
}

output "argocd_initial_password" {
  description = "Command to get ArgoCD initial admin password"
  value       = "Run: kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d"
}

output "grafana_password" {
  description = "Command to get Grafana admin password"
  value       = "Run: kubectl -n monitoring get secret kube-prometheus-stack-grafana -o jsonpath='{.data.admin-password}' | base64 -d"
}
