terraform {
  # configure with:
  #   tofu init -backend-config=bucket=YOUR_BUCKET -backend-config=key=platform/observability/terraform.tfstate -backend-config=region=YOUR_REGION
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
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.12"
    }
    kubectl = {
      source  = "gavinbunney/kubectl"
      version = "~> 1.14"
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

provider "helm" {
  kubernetes {
    host                   = data.terraform_remote_state.eks.outputs.cluster_endpoint
    cluster_ca_certificate = base64decode(data.terraform_remote_state.eks.outputs.cluster_ca_certificate)
    token                  = data.aws_eks_cluster_auth.primary.token
  }
}

provider "kubectl" {
  host                   = data.terraform_remote_state.eks.outputs.cluster_endpoint
  cluster_ca_certificate = base64decode(data.terraform_remote_state.eks.outputs.cluster_ca_certificate)
  token                  = data.aws_eks_cluster_auth.primary.token
  load_config_file       = false
}

# ── Namespace ─────────────────────────────────────────────────────────────────

resource "kubernetes_namespace" "monitoring" {
  metadata {
    name = "monitoring"
  }

  lifecycle {
    ignore_changes = [metadata]
  }
}

# ── kube-prometheus-stack ─────────────────────────────────────────────────────

resource "helm_release" "kube_prometheus_stack" {
  name       = "kube-prometheus-stack"
  repository = "https://prometheus-community.github.io/helm-charts"
  chart      = "kube-prometheus-stack"
  version    = "56.6.2"
  namespace  = kubernetes_namespace.monitoring.metadata[0].name

  timeout = 600

  values = [
    yamlencode({
      prometheus = {
        prometheusSpec = {
          serviceMonitorSelectorNilUsesHelmValues = false
          storageSpec = {
            volumeClaimTemplate = {
              spec = {
                storageClassName = "gp2"
                accessModes      = ["ReadWriteOnce"]
                resources = {
                  requests = {
                    storage = "10Gi"
                  }
                }
              }
            }
          }
          additionalScrapeConfigs = []
        }
      }
      grafana = {
        enabled = true
        adminPassword = var.grafana_password
        service = {
          type = "LoadBalancer"
          annotations = {
            "service.beta.kubernetes.io/aws-load-balancer-type"   = "nlb"
            "service.beta.kubernetes.io/aws-load-balancer-scheme" = "internet-facing"
          }
        }
        additionalDataSources = [
          {
            name   = "Loki"
            type   = "loki"
            url    = "http://loki.monitoring.svc.cluster.local:3100"
            access = "proxy"
          }
        ]
      }
      alertmanager = {
        alertmanagerSpec = {
          storage = {
            volumeClaimTemplate = {
              spec = {
                storageClassName = "gp2"
                accessModes      = ["ReadWriteOnce"]
                resources = {
                  requests = {
                    storage = "5Gi"
                  }
                }
              }
            }
          }
        }
      }
    })
  ]
}

# ── Loki ──────────────────────────────────────────────────────────────────────

resource "helm_release" "loki" {
  name       = "loki"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "loki"
  version    = "5.42.0"
  namespace  = kubernetes_namespace.monitoring.metadata[0].name

  values = [
    yamlencode({
      loki = {
        auth_enabled = false
        commonConfig = {
          replication_factor = 1
        }
        storage = {
          type = "filesystem"
        }
      }
      singleBinary = {
        replicas = 1
        persistence = {
          enabled          = true
          storageClass     = "gp2"
          size             = "10Gi"
        }
      }
      monitoring = {
        selfMonitoring = {
          enabled = false
          grafanaAgent = {
            installOperator = false
          }
        }
        lokiCanary = {
          enabled = false
        }
      }
      test = {
        enabled = false
      }
    })
  ]
}

# ── Promtail ──────────────────────────────────────────────────────────────────

resource "helm_release" "promtail" {
  name       = "promtail"
  repository = "https://grafana.github.io/helm-charts"
  chart      = "promtail"
  version    = "6.15.3"
  namespace  = kubernetes_namespace.monitoring.metadata[0].name

  values = [
    yamlencode({
      config = {
        clients = [
          {
            url = "http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push"
          }
        ]
      }
    })
  ]

  depends_on = [helm_release.loki]
}

# ── OpenTelemetry Operator ────────────────────────────────────────────────────

resource "helm_release" "opentelemetry_operator" {
  name       = "opentelemetry-operator"
  repository = "https://open-telemetry.github.io/opentelemetry-helm-charts"
  chart      = "opentelemetry-operator"
  version    = "0.43.0"
  namespace  = kubernetes_namespace.monitoring.metadata[0].name

  values = [
    yamlencode({
      manager = {
        collectorImage = {
          repository = "otel/opentelemetry-collector-contrib"
        }
      }
    })
  ]
}

# ── OpenTelemetryCollector CR ─────────────────────────────────────────────────

resource "kubectl_manifest" "otel_collector" {
  yaml_body = <<-YAML
    apiVersion: opentelemetry.io/v1alpha1
    kind: OpenTelemetryCollector
    metadata:
      name: platform-collector
      namespace: monitoring
    spec:
      config: |
        receivers:
          otlp:
            protocols:
              grpc:
                endpoint: 0.0.0.0:4317
              http:
                endpoint: 0.0.0.0:4318
          prometheus:
            config:
              scrape_configs:
                - job_name: kubernetes-pods
                  kubernetes_sd_configs:
                    - role: pod
                  relabel_configs:
                    - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
                      action: keep
                      regex: "true"
          k8s_cluster:
            auth_type: serviceAccount
            node_conditions_to_report: [Ready, MemoryPressure, DiskPressure]
          filelog:
            include: [/var/log/pods/*/*/*.log]
            include_file_path: true
            operators:
              - type: json_parser
                timestamp:
                  parse_from: attributes.time
                  layout: '%Y-%m-%dT%H:%M:%S.%LZ'
        processors:
          batch: {}
          memory_limiter:
            check_interval: 5s
            limit_percentage: 80
            spike_limit_percentage: 25
          k8sattributes:
            extract:
              metadata: [k8s.namespace.name, k8s.pod.name, k8s.deployment.name, k8s.node.name]
        exporters:
          prometheus:
            endpoint: "0.0.0.0:8889"
          loki:
            endpoint: http://loki.monitoring.svc.cluster.local:3100/loki/api/v1/push
          logging:
            verbosity: normal
        service:
          pipelines:
            metrics:
              receivers: [otlp, prometheus, k8s_cluster]
              processors: [memory_limiter, k8sattributes, batch]
              exporters: [prometheus]
            logs:
              receivers: [otlp, filelog]
              processors: [memory_limiter, k8sattributes, batch]
              exporters: [loki]
  YAML

  depends_on = [helm_release.opentelemetry_operator]
}

# ── Outputs ───────────────────────────────────────────────────────────────────

output "otel_collector_endpoint" {
  description = "OpenTelemetry collector HTTP endpoint"
  value       = "http://platform-collector-collector.monitoring.svc.cluster.local:4318"
}
