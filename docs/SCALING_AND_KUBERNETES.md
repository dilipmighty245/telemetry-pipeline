# Scaling and Kubernetes Deployment Guide

## Overview

This document describes the scaling capabilities and Kubernetes deployment options for the Telemetry Pipeline. The system supports both local scaling for development and production Kubernetes deployment using Helm charts.

## 🚀 Local Scaling

### Quick Start

```bash
# Scale all components to 3 instances each
make scale-local INSTANCES=3

# Check status
make scale-status

# Stop all components
make scale-stop
```

### Individual Component Scaling

```bash
# Scale only streamers
make scale-streamers STREAMER_INSTANCES=5

# Scale only collectors  
make scale-collectors COLLECTOR_INSTANCES=3

# Scale only gateways
make scale-gateways API_GW_INSTANCES=2
```

### Monitoring Scaled Components

```bash
# Show current status
make scale-status

# Follow logs from all components
make scale-logs

# View specific component logs
tail -f logs/streamer-*.log
tail -f logs/collector-*.log
tail -f logs/gateway-*.log
```

### Local Scaling Features

- **Multiple Instances**: Run 2-10 instances of each component
- **Port Management**: Gateways run on ports 8080, 8081, 8082, etc.
- **Unique IDs**: Each instance gets a unique identifier
- **Process Management**: Automatic PID tracking and cleanup
- **Log Separation**: Separate log files for each instance
- **Health Monitoring**: Individual health checks per instance

## 🎯 Kubernetes Deployment

### Prerequisites

```bash
# Required tools
kubectl version
helm version

# Ensure cluster connection
kubectl cluster-info
```

### Quick Deployment

```bash
# Build and deploy to Kubernetes
make docker-build-nexus
make k8s-deploy-nexus

# Check deployment status
make k8s-status-nexus

# Access the API
make k8s-port-forward
```

### Helm Chart Configuration

The system uses Helm charts located in `deployments/helm/telemetry-pipeline/` with the following key files:

- `Chart.yaml` - Chart metadata and dependencies
- `values-nexus.yaml` - Nexus-specific configuration
- `templates/nexus-*.yaml` - Kubernetes resource templates

### Scaling in Kubernetes

```bash
# Deploy with custom scaling
STREAMER_INSTANCES=5 COLLECTOR_INSTANCES=3 API_GW_INSTANCES=2 make k8s-deploy-nexus

# Or use Helm directly
helm upgrade --install telemetry-pipeline deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --values deployments/helm/telemetry-pipeline/values-nexus.yaml \
  --set nexusStreamer.replicaCount=5 \
  --set nexusCollector.replicaCount=3 \
  --set nexusGateway.replicaCount=2
```

### Kubernetes Features

- **Horizontal Pod Autoscaler (HPA)** - Automatic scaling based on CPU/memory
- **Pod Disruption Budgets** - Ensure availability during updates
- **Health Checks** - Liveness and readiness probes
- **Service Discovery** - Automatic load balancing
- **ConfigMaps** - Configuration management
- **Persistent Volumes** - CSV data storage
- **Ingress** - External access configuration

## 📊 Architecture Components

### Nexus Streamer
- **Purpose**: Reads CSV data and streams to etcd message queue
- **Scaling**: Multiple instances process data in parallel
- **Configuration**: Batch size, streaming interval, CSV source

### Nexus Collector  
- **Purpose**: Consumes from message queue and persists data
- **Scaling**: Multiple instances for parallel processing
- **Configuration**: Processing interval, batch size, Nexus integration

### Nexus Gateway
- **Purpose**: Multi-protocol API (REST, GraphQL, WebSocket)
- **Scaling**: Load-balanced instances for high availability
- **Configuration**: Port, protocol enablement, health checks

### etcd
- **Purpose**: Message queue and data storage backend
- **Scaling**: 3-node cluster for high availability
- **Configuration**: Persistence, resources, authentication

## 🔧 Configuration Options

### Environment Variables

All components support these common environment variables:

```bash
CLUSTER_ID=k8s-cluster          # Cluster identifier
ETCD_ENDPOINTS=etcd:2379        # etcd connection string
LOG_LEVEL=info                  # Logging level
```

### Component-Specific Variables

**Streamer:**
```bash
CSV_FILE=/data/telemetry.csv    # CSV data source
BATCH_SIZE=100                  # Records per batch
STREAM_INTERVAL=1s              # Streaming frequency
```

**Collector:**
```bash
ENABLE_NEXUS=true              # Enable Nexus integration
PROCESSING_INTERVAL=5s         # Processing frequency
BATCH_SIZE=50                  # Processing batch size
```

**Gateway:**
```bash
PORT=8080                      # HTTP port
ENABLE_GRAPHQL=true           # Enable GraphQL endpoint
ENABLE_WEBSOCKET=true         # Enable WebSocket support
ENABLE_CORS=true              # Enable CORS
```

## 🐳 Docker Images

### Building Images

```bash
# Build all Nexus images
make docker-build-nexus

# Build individual images
docker build -f deployments/docker/Dockerfile.nexus-streamer -t nexus-streamer .
docker build -f deployments/docker/Dockerfile.nexus-collector -t nexus-collector .
docker build -f deployments/docker/Dockerfile.nexus-gateway -t nexus-gateway .
```

### Image Features

- **Multi-stage builds** - Optimized image sizes
- **Non-root user** - Security best practices
- **Health checks** - Container health monitoring
- **Alpine Linux base** - Minimal attack surface
- **Build-time optimization** - Static linking, stripped binaries

## 📈 Performance Considerations

### Local Scaling

- **Resource Usage**: Each instance uses ~100-200MB RAM
- **CPU Usage**: Scales with data processing volume
- **Network**: Multiple ports for gateways (8080+)
- **Storage**: Shared etcd backend, separate log files

### Kubernetes Scaling

- **Resource Limits**: Configurable CPU/memory limits
- **Node Placement**: Affinity and anti-affinity rules
- **Storage**: Persistent volumes for CSV data
- **Networking**: Service mesh compatible

### Scaling Guidelines

| Component | Min Instances | Max Instances | Resource/Instance |
|-----------|---------------|---------------|-------------------|
| Streamer  | 1             | 10            | 100m CPU, 128Mi RAM |
| Collector | 1             | 10            | 200m CPU, 256Mi RAM |
| Gateway   | 2             | 5             | 200m CPU, 256Mi RAM |

## 🔍 Monitoring and Troubleshooting

### Local Monitoring

```bash
# Component status
make scale-status

# Process monitoring
ps aux | grep nexus

# Log analysis
tail -f logs/*.log | grep ERROR

# Resource usage
top -p $(pgrep nexus)
```

### Kubernetes Monitoring

```bash
# Pod status
kubectl get pods -n telemetry-system

# Resource usage
kubectl top pods -n telemetry-system

# Logs
kubectl logs -f -l app.kubernetes.io/name=telemetry-pipeline

# Events
kubectl get events -n telemetry-system --sort-by='.lastTimestamp'
```

### Common Issues

**Local Scaling:**
- Port conflicts: Check if ports 8080+ are available
- etcd connection: Ensure etcd is running (`make setup-etcd`)
- Resource limits: Monitor system resources with multiple instances

**Kubernetes:**
- Image pull errors: Verify image registry and credentials
- Resource constraints: Check node capacity and resource requests
- Network policies: Ensure etcd connectivity

## 🧪 Testing

### Load Testing

```bash
# Test multiple gateway instances
for port in 8080 8081 8082; do
  curl -s http://localhost:$port/api/v1/gpus | jq '.count'
done

# Stress test
ab -n 1000 -c 10 http://localhost:8080/api/v1/gpus
```

### Integration Testing

```bash
# Run the demo script
./scripts/demo-scaling.sh

# Manual testing
./scripts/verify-record-count.sh
```

## 📝 Best Practices

### Development

1. **Start Small**: Begin with 2-3 instances for development
2. **Monitor Resources**: Watch CPU/memory usage during scaling
3. **Log Management**: Regularly clean up log files
4. **Port Management**: Use sequential ports for gateways

### Production

1. **Resource Planning**: Calculate resource requirements based on load
2. **High Availability**: Deploy across multiple nodes
3. **Monitoring**: Set up comprehensive monitoring and alerting
4. **Backup**: Ensure etcd data is backed up regularly
5. **Security**: Use network policies and RBAC

### Scaling Strategy

1. **Horizontal First**: Scale instances before vertical scaling
2. **Bottleneck Analysis**: Identify and address performance bottlenecks
3. **Gradual Scaling**: Increase instances incrementally
4. **Load Testing**: Test scaling under realistic load conditions

## 🔗 Related Documentation

- [API Specification](API_SPECIFICATION.md) - REST API documentation
- [Nexus Integration Guide](NEXUS_INTEGRATION_GUIDE.md) - Nexus framework details
- [Deployment Guide](CONSOLIDATED_SETUP_COMMANDS.md) - Setup instructions

## 🚀 Quick Reference

```bash
# Local Development
make scale-local INSTANCES=3     # Scale locally
make scale-status               # Check status
make scale-stop                # Stop all

# Kubernetes Production  
make docker-build-nexus        # Build images
make k8s-deploy-nexus         # Deploy to K8s
make k8s-status-nexus         # Check K8s status
make k8s-port-forward         # Access API

# Monitoring
make scale-logs               # Local logs
make k8s-logs-nexus          # K8s logs
```
