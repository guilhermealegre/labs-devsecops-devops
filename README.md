# Zero-Touch Kubernetes Platform on DigitalOcean

A complete **DevOps Laboratory** demonstrating a full GitOps lifecycle for 3 distinct multi-tier applications with deep observability (metrics & logs).

![Platform Architecture](docs/architecture.png)

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DigitalOcean Kubernetes (DOKS)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────┐  ┌─────────────────────────────────────────────────────┐  │
│  │   ArgoCD    │  │                  NGINX Ingress                       │  │
│  │  (GitOps)   │  └─────────────────────────────────────────────────────┘  │
│  └─────────────┘                                                            │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────────┐│
│  │                        Observability Stack                              ││
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐           ││
│  │  │Prometheus │  │   Loki    │  │  Grafana  │  │ Promtail  │           ││
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘           ││
│  └─────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐             │
│  │     App A       │  │     App B       │  │     App C       │             │
│  │  Go + Postgres  │  │ Python + RabbitMQ│  │ Rust + Postgres │             │
│  │                 │  │ + Go Worker      │  │                 │             │
│  │  ┌───┐  ┌───┐  │  │  ┌───┐  ┌───┐   │  │  ┌───┐  ┌───┐  │             │
│  │  │ FE│  │ BE│  │  │  │API│  │WKR│   │  │  │ FE│  │ BE│  │             │
│  │  └───┘  └───┘  │  │  └───┘  └───┘   │  │  └───┘  └───┘  │             │
│  └────────┬────────┘  └────────┬────────┘  └────────┬────────┘             │
│           │                    │                    │                       │
│  ┌────────▼────────────────────▼────────────────────▼────────┐             │
│  │                    Infrastructure                          │             │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐                │             │
│  │  │PostgreSQL│  │ MongoDB  │  │ RabbitMQ │                │             │
│  │  │(do-block)│  │(do-block)│  │(do-block)│                │             │
│  │  └──────────┘  └──────────┘  └──────────┘                │             │
│  └───────────────────────────────────────────────────────────┘             │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

## ✨ Features

- **Zero-Touch Deployment**: Single `tofu apply` command bootstraps everything
- **GitOps with ArgoCD**: App of Apps pattern for managing applications
- **Full Observability**: PLG Stack (Prometheus, Loki, Grafana) with auto-provisioned dashboards
- **Three Unique Tech Stacks**:
  - **App A**: Go + PostgreSQL (standard REST API)
  - **App B**: Python + RabbitMQ + Go Worker + MongoDB (async pipeline)
  - **App C**: Rust + PostgreSQL (high-performance API)

## 📁 Repository Structure

```
.
├── main.tf                    # OpenTofu - DOKS cluster + ArgoCD bootstrap
├── variables.tf               # Configurable variables
├── terraform.tfvars.example   # Example configuration
├── bootstrap/
│   └── root-app.yaml          # ArgoCD root application
├── apps/                      # ArgoCD Application definitions
│   ├── infrastructure.yaml    # PLG stack + NGINX Ingress
│   ├── app-a.yaml
│   ├── app-b.yaml
│   └── app-c.yaml
├── src/                       # Application source code
│   ├── app-a/
│   │   ├── backend/           # Go REST API
│   │   └── frontend/          # React (Vite)
│   ├── app-b/
│   │   ├── backend-api/       # Python FastAPI
│   │   ├── worker/            # Go RabbitMQ consumer
│   │   └── frontend/          # React (Vite)
│   └── app-c/
│       ├── backend/           # Rust (Axum)
│       └── frontend/          # React (Vite)
└── manifests/                 # Kubernetes manifests
    ├── infra/                 # PostgreSQL, MongoDB, RabbitMQ
    ├── dashboards/            # Grafana dashboard ConfigMap
    ├── app-a/
    ├── app-b/
    └── app-c/
```

## 🚀 Quick Start

### Prerequisites

- [OpenTofu](https://opentofu.org/) or Terraform >= 1.6
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Docker](https://docs.docker.com/get-docker/) (for building images)
- DigitalOcean account with API token

### 1. Clone and Configure

```bash
git clone https://github.com/YOUR_USERNAME/devops-lab.git
cd devops-lab

# Copy and edit configuration
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your settings
```

### 2. Update Git Repository URL

Update the `git_repo_url` in `terraform.tfvars` and the `repoURL` in all ArgoCD application files:

```bash
# Update all references
sed -i 's|YOUR_USERNAME/devops-lab|your-actual-username/devops-lab|g' \
  terraform.tfvars \
  bootstrap/root-app.yaml \
  apps/*.yaml
```

### 3. Build and Push Docker Images

Before deploying, build and push your images to a container registry:

```bash
# Example using Docker Hub
export REGISTRY=your-dockerhub-username

# App A
docker build -t $REGISTRY/app-a-backend:latest ./src/app-a/backend
docker build -t $REGISTRY/app-a-frontend:latest ./src/app-a/frontend
docker push $REGISTRY/app-a-backend:latest
docker push $REGISTRY/app-a-frontend:latest

# App B
docker build -t $REGISTRY/app-b-api:latest ./src/app-b/backend-api
docker build -t $REGISTRY/app-b-worker:latest ./src/app-b/worker
docker build -t $REGISTRY/app-b-frontend:latest ./src/app-b/frontend
docker push $REGISTRY/app-b-api:latest
docker push $REGISTRY/app-b-worker:latest
docker push $REGISTRY/app-b-frontend:latest

# App C
docker build -t $REGISTRY/app-c-backend:latest ./src/app-c/backend
docker build -t $REGISTRY/app-c-frontend:latest ./src/app-c/frontend
docker push $REGISTRY/app-c-backend:latest
docker push $REGISTRY/app-c-frontend:latest
```

Update the image references in `manifests/app-*/` to match your registry.

### 4. Deploy

```bash
# Set DigitalOcean token
export DIGITALOCEAN_TOKEN="your-token-here"

# Initialize and deploy
tofu init
tofu apply
```

### 5. Access the Cluster

```bash
# Get kubeconfig
tofu output -raw kubeconfig > ~/.kube/devops-lab-config
export KUBECONFIG=~/.kube/devops-lab-config

# Verify cluster
kubectl get nodes
```

### 6. Access Services

```bash
# ArgoCD
kubectl get svc -n argocd argocd-server -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
# Get initial password
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d

# Grafana
kubectl get svc -n monitoring kube-prometheus-stack-grafana -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
# Default: admin / admin123
```

## 📊 Metrics Exposed

### App A (Go + PostgreSQL)
| Metric | Type | Description |
|--------|------|-------------|
| `http_request_duration_seconds` | Histogram | HTTP request latency |
| `db_query_duration_seconds` | Histogram | Database query latency |
| `app_a_users_total` | Gauge | Total users in database |

### App B (Python + RabbitMQ + Go Worker)
| Metric | Type | Description |
|--------|------|-------------|
| `http_requests_total` | Counter | Total HTTP requests |
| `jobs_submitted_total` | Counter | Jobs submitted to queue |
| `jobs_processed_total` | Counter | Jobs processed by worker |
| `job_processing_duration_seconds` | Histogram | Job processing time |
| `jobs_in_progress` | Gauge | Jobs currently processing |

### App C (Rust + PostgreSQL)
| Metric | Type | Description |
|--------|------|-------------|
| `request_duration_seconds` | Histogram | Request latency |
| `active_connections` | Gauge | Active DB connections |
| `thread_usage` | GaugeVec | Thread pool metrics |

## 🔐 Security Notes

> ⚠️ **Important**: This is a learning laboratory. For production use:

1. **Secrets Management**: Replace Kubernetes Opaque Secrets with:
   - [External Secrets Operator](https://external-secrets.io/)
   - [Sealed Secrets](https://sealed-secrets.netlify.app/)
   - [HashiCorp Vault](https://www.vaultproject.io/)

2. **Database**: Consider DigitalOcean Managed Databases instead of in-cluster StatefulSets

3. **Ingress**: Add TLS certificates with cert-manager

4. **Network Policies**: Implement NetworkPolicies for pod-to-pod traffic control

## 🧹 Cleanup

```bash
# Destroy all resources
tofu destroy
```

## 📝 License

MIT License - See [LICENSE](LICENSE) for details.

---

Built with ❤️ for learning DevOps and Platform Engineering
# labs-devsecops-devops
