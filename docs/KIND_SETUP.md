# Kind Setup Guide for Cross-Cluster Testing

This guide shows you how to use Kind (Kubernetes in Docker) to create multiple clusters and test the cross-cluster deployment of the telemetry pipeline.

## 🚀 Quick Start

### 1. Install Prerequisites

```bash
# Install Kind and kubectl (if not already installed)
./scripts/setup-kind-clusters.sh install-deps
```

### 2. Run Full End-to-End Test

```bash
# This will create clusters, build images, deploy everything, and test the pipeline
./scripts/test-cross-cluster-kind.sh full-test
```

That's it! The script will:
- Create 3 Kind clusters (edge, central, shared)
- Build and load Docker images
- Deploy Redis to shared cluster
- Deploy streamers to edge cluster
- Deploy collectors and API to central cluster
- Test cross-cluster connectivity
- Verify the complete pipeline works

## 📋 Step-by-Step Manual Setup

If you prefer to run each step manually:

### Step 1: Create Kind Clusters

```bash
# Create all three clusters
./scripts/setup-kind-clusters.sh create-all

# Or create them individually
./scripts/setup-kind-clusters.sh create-edge
./scripts/setup-kind-clusters.sh create-central
./scripts/setup-kind-clusters.sh create-shared
```

### Step 2: Build and Load Images

```bash
# Build Docker images
docker build -f deployments/docker/Dockerfile.streamer -t telemetry-pipeline-streamer:latest .
docker build -f deployments/docker/Dockerfile.collector -t telemetry-pipeline-collector:latest .
docker build -f deployments/docker/Dockerfile.api-gateway -t telemetry-pipeline-api-gateway:latest .

# Load images into all clusters
./scripts/setup-kind-clusters.sh load-images
```

### Step 3: Deploy Shared Infrastructure (Redis)

```bash
# Add Helm repo
helm repo add bitnami https://charts.bitnami.com/bitnami
helm repo update

# Deploy Redis to shared cluster
helm upgrade --install redis-cluster bitnami/redis \
  --namespace shared-infrastructure \
  --create-namespace \
  --kube-context kind-telemetry-shared \
  --set auth.password="redis123" \
  --set architecture="standalone" \
  --set master.persistence.enabled=false \
  --set master.service.type=NodePort \
  --set master.service.nodePorts.redis=30379 \
  --wait
```

### Step 4: Deploy Edge Components (Streamers)

```bash
# Get shared cluster IP for cross-cluster communication
SHARED_IP=$(docker inspect telemetry-shared-control-plane | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress')

# Deploy streamers to edge cluster
helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --kube-context kind-telemetry-edge \
  --values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
  --set externalRedis.enabled=true \
  --set externalRedis.host="$SHARED_IP" \
  --set externalRedis.port=30379 \
  --set externalRedis.password="redis123" \
  --wait
```

### Step 5: Deploy Central Components (Collectors + API)

```bash
# Deploy collectors and API to central cluster
helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --kube-context kind-telemetry-central \
  --values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
  --set externalRedis.enabled=true \
  --set externalRedis.host="$SHARED_IP" \
  --set externalRedis.port=30379 \
  --set externalRedis.password="redis123" \
  --set postgresql.auth.password="postgres123" \
  --wait
```

## 🔍 Verification and Testing

### Check Cluster Status

```bash
# Check all clusters
./scripts/setup-kind-clusters.sh status

# Check specific deployments
kubectl --context=kind-telemetry-edge -n telemetry-system get pods
kubectl --context=kind-telemetry-central -n telemetry-system get pods
kubectl --context=kind-telemetry-shared -n shared-infrastructure get pods
```

### Test Cross-Cluster Connectivity

```bash
# Test Redis connectivity from edge cluster
kubectl --context=kind-telemetry-edge -n telemetry-system exec -it \
  $(kubectl --context=kind-telemetry-edge -n telemetry-system get pod -l app.kubernetes.io/component=streamer -o jsonpath='{.items[0].metadata.name}') \
  -- sh -c "apk add --no-cache redis && redis-cli -h $SHARED_IP -p 30379 -a redis123 ping"
```

### Test the API

```bash
# Port forward to API gateway
kubectl --context=kind-telemetry-central -n telemetry-system port-forward svc/telemetry-central-telemetry-pipeline-api-gateway 8080:80 &

# Test API endpoints
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/gpus
```

### View Logs

```bash
# Streamer logs (edge cluster)
kubectl --context=kind-telemetry-edge -n telemetry-system logs -l app.kubernetes.io/component=streamer

# Collector logs (central cluster)
kubectl --context=kind-telemetry-central -n telemetry-system logs -l app.kubernetes.io/component=collector

# API gateway logs (central cluster)
kubectl --context=kind-telemetry-central -n telemetry-system logs -l app.kubernetes.io/component=api-gateway
```

## 🏗️ Architecture Overview

```
┌─────────────────────┐    ┌─────────────────────┐    ┌─────────────────────┐
│   Edge Cluster      │    │  Shared Cluster     │    │  Central Cluster    │
│  kind-telemetry-    │    │  kind-telemetry-    │    │  kind-telemetry-    │
│  edge               │    │  shared             │    │  central            │
│                     │    │                     │    │                     │
│  ┌─────────────┐    │    │  ┌─────────────┐    │    │  ┌─────────────┐    │
│  │  Streamers  │────┼────┤  │    Redis    │    ├────┼──│ Collectors  │    │
│  │             │    │    │  │   Cluster   │    │    │  │             │    │
│  └─────────────┘    │    │  └─────────────┘    │    │  └─────────────┘    │
│                     │    │                     │    │                     │
│  ┌─────────────┐    │    │                     │    │  ┌─────────────┐    │
│  │  CSV Data   │    │    │                     │    │  │ PostgreSQL  │    │
│  └─────────────┘    │    │                     │    │  └─────────────┘    │
│                     │    │                     │    │                     │
└─────────────────────┘    └─────────────────────┘    │  ┌─────────────┐    │
                                                      │  │ API Gateway │    │
                                                      │  └─────────────┘    │
                                                      └─────────────────────┘
```

## 🧹 Cleanup

### Clean Up Deployments Only

```bash
# Remove all Helm deployments but keep clusters
./scripts/test-cross-cluster-kind.sh cleanup
```

### Delete All Clusters

```bash
# Delete all Kind clusters
./scripts/setup-kind-clusters.sh delete-all
```

## 🔧 Customization

### Cluster Configuration

You can customize the clusters by modifying the setup script parameters:

```bash
# Create clusters with more nodes
./scripts/setup-kind-clusters.sh --edge-nodes 3 --central-nodes 4 create-all

# Use custom registry port
./scripts/setup-kind-clusters.sh --registry-port 5002 create-all

# Skip local registry setup
./scripts/setup-kind-clusters.sh --skip-registry create-all
```

### Application Configuration

Modify the Helm values files to customize the deployment:

- `values-edge-cluster.yaml` - Edge cluster configuration
- `values-central-cluster.yaml` - Central cluster configuration
- `values.yaml` - Default configuration

### Network Configuration

For more advanced networking scenarios, you can:

1. Use different Docker networks for isolation
2. Configure custom port mappings
3. Set up load balancers between clusters
4. Implement service mesh (Istio/Linkerd)

## 🐛 Troubleshooting

### Common Issues

1. **Clusters not accessible**
   ```bash
   # Check if clusters exist
   kind get clusters
   
   # Check if nodes are ready
   kubectl --context=kind-telemetry-edge get nodes
   ```

2. **Images not found**
   ```bash
   # Rebuild and reload images
   ./scripts/setup-kind-clusters.sh load-images
   ```

3. **Cross-cluster connectivity issues**
   ```bash
   # Check Docker network
   docker network ls | grep kind
   
   # Check cluster IPs
   docker inspect telemetry-shared-control-plane | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress'
   ```

4. **Redis connection failures**
   ```bash
   # Check Redis service
   kubectl --context=kind-telemetry-shared -n shared-infrastructure get svc
   
   # Test Redis directly
   kubectl --context=kind-telemetry-shared -n shared-infrastructure exec -it \
     $(kubectl --context=kind-telemetry-shared -n shared-infrastructure get pod -l app.kubernetes.io/name=redis -o jsonpath='{.items[0].metadata.name}') \
     -- redis-cli -a redis123 ping
   ```

### Debug Mode

Run scripts with debug mode for more verbose output:

```bash
./scripts/test-cross-cluster-kind.sh --debug full-test
```

### Logs and Monitoring

```bash
# Check all pod logs
kubectl --context=kind-telemetry-edge -n telemetry-system logs -l app.kubernetes.io/name=telemetry-pipeline
kubectl --context=kind-telemetry-central -n telemetry-system logs -l app.kubernetes.io/name=telemetry-pipeline

# Monitor resource usage
kubectl --context=kind-telemetry-edge top pods -n telemetry-system
kubectl --context=kind-telemetry-central top pods -n telemetry-system
```

## 🎯 Next Steps

After setting up Kind clusters, you can:

1. **Test different deployment patterns**: Same cluster vs cross-cluster
2. **Experiment with scaling**: Add more streamers or collectors
3. **Test failure scenarios**: Kill pods and see recovery
4. **Monitor performance**: Add Prometheus and Grafana
5. **Implement security**: Add network policies and TLS

This Kind setup provides a perfect environment to validate that your telemetry pipeline works identically whether deployed in the same cluster or distributed across multiple clusters!
