# Telemetry Pipeline Deployment Modes

This document provides a comprehensive guide for deploying the telemetry pipeline in different configurations.

## 📋 Overview

The telemetry pipeline supports two primary deployment patterns:

1. **Same Cluster Deployment** - All components in one cluster
2. **Cross-Cluster Deployment** - Components distributed across multiple clusters

## 🔧 Configuration Files

| File | Purpose | Components |
|------|---------|------------|
| `values.yaml` | Default development configuration | All components (basic settings) |
| `values-same-cluster.yaml` | Single cluster production deployment | All components (optimized for single cluster) |
| `values-edge-cluster.yaml` | Edge cluster deployment | Streamers only |
| `values-central-cluster.yaml` | Central cluster deployment | Collectors, API Gateway, Database |

## 🚀 Deployment Commands Quick Reference

### Same Cluster Deployment

```bash
# Basic deployment
make helm-install-same-cluster

# With custom configuration
helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-same-cluster.yaml \
  --set image.tag=v1.0.0 \
  --set postgresql.auth.password="secure-password"

# Validation
make validate-same-cluster
make validate-health
```

### Cross-Cluster Deployment

#### Method 1: Using Makefile (Recommended)

```bash
# Set environment variables
export EDGE_CONTEXT="edge-cluster"
export CENTRAL_CONTEXT="central-cluster"
export REDIS_HOST="redis.shared-infra.company.com"
export REDIS_PASSWORD="secure-redis-password"
export DB_PASSWORD="secure-db-password"

# Deploy all components
make helm-install-cross-cluster-all

# Or deploy individually
make helm-install-cross-cluster-edge
make helm-install-cross-cluster-central

# Validation
make validate-cross-cluster
make validate-connectivity
```

#### Method 2: Using Deployment Script

```bash
# Deploy all components
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-cluster \
  --central-context central-cluster \
  --redis-host redis.company.com \
  --redis-password "secure-password" \
  --db-password "secure-db-password" \
  deploy-all

# Check status
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-cluster \
  --central-context central-cluster \
  status
```

## 🏗️ Architecture Diagrams

### Same Cluster Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                       │
│                                                             │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │  Streamer   │    │  Streamer   │    │  Streamer   │     │
│  │    Pod      │    │    Pod      │    │    Pod      │     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
│         │                   │                   │          │
│         └───────────────────┼───────────────────┘          │
│                             │                              │
│                    ┌─────────────┐                         │
│                    │    Redis    │                         │
│                    │   Service   │                         │
│                    └─────────────┘                         │
│                             │                              │
│         ┌───────────────────┼───────────────────┐          │
│         │                   │                   │          │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐     │
│  │ Collector   │    │ Collector   │    │ Collector   │     │
│  │    Pod      │    │    Pod      │    │    Pod      │     │
│  └─────────────┘    └─────────────┘    └─────────────┘     │
│         │                   │                   │          │
│         └───────────────────┼───────────────────┘          │
│                             │                              │
│                    ┌─────────────┐                         │
│                    │ PostgreSQL  │                         │
│                    │  Database   │                         │
│                    └─────────────┘                         │
│                             │                              │
│                    ┌─────────────┐                         │
│                    │ API Gateway │                         │
│                    │   Service   │                         │
│                    └─────────────┘                         │
└─────────────────────────────────────────────────────────────┘
```

### Cross-Cluster Architecture

```
┌─────────────────────────────────┐    ┌─────────────────────────────────┐
│         Edge Cluster            │    │       Central Cluster           │
│                                 │    │                                 │
│  ┌─────────────┐                │    │                ┌─────────────┐  │
│  │  Streamer   │                │    │                │ Collector   │  │
│  │    Pod      │                │    │                │    Pod      │  │
│  └─────────────┘                │    │                └─────────────┘  │
│  ┌─────────────┐                │    │                ┌─────────────┐  │
│  │  Streamer   │                │    │                │ Collector   │  │
│  │    Pod      │                │    │                │    Pod      │  │
│  └─────────────┘                │    │                └─────────────┘  │
│         │                       │    │                       │         │
└─────────┼───────────────────────┘    └───────────────────────┼─────────┘
          │                                                    │
          │              ┌─────────────────────┐               │
          └──────────────┤   External Redis    ├───────────────┘
                         │  Message Queue      │
                         └─────────────────────┘
                                                    │
                         ┌─────────────────────────────────┐
                         │       Central Cluster           │
                         │                                 │
                         │        ┌─────────────┐          │
                         │        │ PostgreSQL  │          │
                         │        │  Database   │          │
                         │        └─────────────┘          │
                         │               │                 │
                         │        ┌─────────────┐          │
                         │        │ API Gateway │          │
                         │        │   Service   │          │
                         │        └─────────────┘          │
                         └─────────────────────────────────┘
```

## 📊 Deployment Comparison

| Aspect | Same Cluster | Cross-Cluster |
|--------|-------------|---------------|
| **Complexity** | Low | High |
| **Setup Time** | 5-10 minutes | 30-60 minutes |
| **Network Latency** | Very Low | Variable |
| **Fault Tolerance** | Single point of failure | High availability |
| **Scalability** | Limited by cluster size | Unlimited |
| **Cost** | Lower | Higher (multiple clusters) |
| **Maintenance** | Simple | Complex |
| **Use Case** | Dev/Test/Small Scale | Production/Large Scale |

## 🔍 Validation Checklist

### Same Cluster Validation
- [ ] All pods are running: `kubectl get pods -n telemetry-system`
- [ ] Services are accessible: `kubectl get svc -n telemetry-system`
- [ ] Health endpoints respond: `make validate-health`
- [ ] Auto-scaling works: `make validate-scaling`
- [ ] API endpoints work: `curl http://localhost:8080/health`

### Cross-Cluster Validation
- [ ] Edge streamers are running: `kubectl get pods -n telemetry-system --context=$EDGE_CONTEXT`
- [ ] Central collectors are running: `kubectl get pods -n telemetry-system --context=$CENTRAL_CONTEXT`
- [ ] Redis connectivity works: `make validate-connectivity`
- [ ] Database connectivity works: `make validate-connectivity`
- [ ] Cross-cluster communication: Monitor logs for successful message flow

## 🛠️ Customization Examples

### Custom Node Selection

```yaml
# Edge cluster - GPU nodes
streamer:
  nodeSelector:
    accelerator: nvidia-gpu
  tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule

# Central cluster - High memory nodes
collector:
  nodeSelector:
    node-type: high-memory
  tolerations:
  - key: high-memory
    operator: Exists
    effect: NoSchedule
```

### Custom Resource Limits

```yaml
# High-throughput configuration
streamer:
  replicaCount: 10
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 500m
      memory: 512Mi

collector:
  replicaCount: 8
  resources:
    limits:
      cpu: 2000m
      memory: 4Gi
    requests:
      cpu: 1000m
      memory: 2Gi
```

### External Services Configuration

```yaml
# External database
postgresql:
  enabled: false

externalDatabase:
  host: "postgres.company.com"
  port: 5432
  username: "telemetry_user"
  password: "secure-password"
  database: "telemetry_prod"
  sslMode: "require"

# External Redis
redis:
  enabled: false

externalRedis:
  enabled: true
  host: "redis.company.com"
  port: 6379
  password: "redis-password"
  tls:
    enabled: true
```

## 📈 Performance Tuning

### Same Cluster Tuning

```yaml
# Optimized for single cluster
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 12
  targetCPUUtilizationPercentage: 70

postgresql:
  primary:
    extendedConfiguration: |
      max_connections = 200
      shared_buffers = 512MB
      effective_cache_size = 2GB
      work_mem = 8MB
```

### Cross-Cluster Tuning

```yaml
# Edge cluster - Conservative resources
autoscaling:
  minReplicas: 2
  maxReplicas: 8

# Central cluster - Aggressive scaling
autoscaling:
  minReplicas: 5
  maxReplicas: 20
  targetCPUUtilizationPercentage: 60
```

## 🔐 Security Considerations

### Network Security

```yaml
networkPolicy:
  enabled: true
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
  egress:
  - to: []
    ports:
    - protocol: TCP
      port: 443  # HTTPS only
```

### TLS Configuration

```yaml
externalRedis:
  tls:
    enabled: true
    skipVerify: false
    caCert: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----

externalDatabase:
  sslMode: "require"
```

## 🎯 Best Practices

1. **Resource Planning**: Always set resource requests and limits
2. **Security**: Enable TLS for cross-cluster communication
3. **Monitoring**: Enable ServiceMonitor for Prometheus integration
4. **Backup**: Configure automated database backups
5. **Testing**: Validate deployments using provided scripts
6. **Documentation**: Keep deployment configurations in version control

## 🚨 Common Pitfalls

1. **Resource Starvation**: Not setting appropriate resource limits
2. **Network Connectivity**: Firewall rules blocking cross-cluster communication
3. **DNS Issues**: Incorrect service discovery configuration
4. **Certificate Problems**: Expired or misconfigured TLS certificates
5. **Version Mismatch**: Different image versions across clusters
6. **Secret Management**: Hardcoded passwords in configuration files

## 🔄 Migration Guide

### From Same Cluster to Cross-Cluster

1. **Backup Data**: Export existing data from PostgreSQL
2. **Setup External Redis**: Deploy Redis cluster in shared infrastructure
3. **Deploy Central Cluster**: Deploy collectors and API gateway
4. **Deploy Edge Clusters**: Deploy streamers with external Redis configuration
5. **Migrate Data**: Import data to central cluster database
6. **Validate**: Ensure all components are communicating correctly
7. **Cleanup**: Remove old same-cluster deployment

### From Cross-Cluster to Same Cluster

1. **Backup Data**: Export data from central cluster database
2. **Deploy Same Cluster**: Use `values-same-cluster.yaml`
3. **Import Data**: Load data into new database
4. **Update Streamers**: Reconfigure to use local Redis
5. **Validate**: Test all functionality
6. **Cleanup**: Remove cross-cluster deployments

---

For detailed troubleshooting and advanced configuration, see the main [README.md](README.md) and [docs/CROSS_CLUSTER_DEPLOYMENT.md](docs/CROSS_CLUSTER_DEPLOYMENT.md).
