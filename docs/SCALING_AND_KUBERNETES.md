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
- **Persistent Volumes** - Upload storage for CSV files (optional)
- **Ingress** - External access configuration

## 🌐 Multi-Cluster Deployments

In multi-cluster setups, streamers may run in edge clusters while collectors and gateways run centrally. Ensure cross-cluster reachability and correct service address registration.

### Service Address Resolution (Priority)

1. `SERVICE_ADDRESS` (explicit override)
2. `EXTERNAL_SERVICE_ADDRESS` (recommended for cross-cluster)
3. Pod IP (`POD_IP` via Downward API)
4. Kubernetes hostname resolution
5. First non-loopback IPv4
6. Localhost (dev only)

### Deployment Options

- **LoadBalancer (recommended)**: Assign external IP to `nexus-streamer` and set `EXTERNAL_SERVICE_ADDRESS`.
- **NodePort**: Expose a stable port on nodes and publish the node external IP.
- **Ingress + DNS**: Publish a DNS hostname and set `EXTERNAL_SERVICE_ADDRESS` accordingly.

### Core Environment Variables

```bash
CLUSTER_ID=cluster-a
ETCD_ENDPOINTS=etcd-cluster.shared.svc.cluster.local:2379
EXTERNAL_SERVICE_ADDRESS=203.0.113.50  # or streamer.cluster-a.example.com
HTTP_PORT=8081
POD_IP=$(status.podIP)
```

### Troubleshooting

```bash
# Verify service registered address in logs
kubectl logs deployment/nexus-streamer | grep "Service will register with address"

# Inspect service discovery in etcd
etcdctl get /services/nexus-streamer/ --prefix

# Test cross-cluster reachability
curl -I http://203.0.113.50:8081/health
```

### Security & Performance Notes

- Use TLS for cross-cluster traffic and secure etcd access.
- Apply network policies to restrict etcd ports (2379/2380).
- Place etcd centrally with low-latency connectivity; monitor consensus latency.

## 📊 Architecture Components & Scaling Strategies

### Nexus Streamer
- **Purpose**: Ingests CSV data uploaded via HTTP and streams to etcd message queue
- **Scaling Strategy**: 
  - **Horizontal**: 1-10 instances (per requirements)
  - **Load Distribution**: Each instance processes different upload streams or chunked batches
  - **Resource Pattern**: CPU-intensive during CSV parsing, memory for buffering uploads
  - **Bottlenecks**: Upload I/O, CSV parsing, network to etcd
- **Configuration**: Batch size (100-1000), streaming interval (1-5s), HTTP upload settings
- **Scaling Metrics**: CPU usage >70%, memory >80%, message queue depth

### Nexus Collector  
- **Purpose**: Consumes from message queue and persists data
- **Scaling Strategy**:
  - **Horizontal**: 1-10 instances (per requirements)
  - **Work Distribution**: Shared message queue with atomic operations
  - **Resource Pattern**: Memory-intensive for message buffering, CPU for processing
  - **Bottlenecks**: Message processing rate, etcd write throughput
- **Configuration**: Processing interval (1-10s), batch size (50-200), worker count (4-16)
- **Scaling Metrics**: Message queue depth, processing latency, memory usage

### Nexus Gateway
- **Purpose**: Multi-protocol API (REST, GraphQL, WebSocket)
- **Scaling Strategy**:
  - **Horizontal**: 2-5 instances (stateless, load-balanced)
  - **Load Balancing**: Round-robin or least-connections
  - **Resource Pattern**: CPU for query processing, memory for response caching
  - **Bottlenecks**: etcd read throughput, complex query processing
- **Configuration**: Port (8080), protocol enablement, cache settings
- **Scaling Metrics**: Request rate, response time >100ms, CPU >60%

### etcd Cluster
- **Purpose**: Message queue and data storage backend
- **Scaling Strategy**:
  - **Vertical**: Increase CPU/memory per node
  - **Horizontal**: 3-5 node cluster (odd numbers for consensus)
  - **Resource Pattern**: Memory for data, CPU for consensus, disk I/O
  - **Bottlenecks**: Disk I/O, network latency between nodes
- **Configuration**: Persistence, compaction, authentication, TLS
- **Scaling Metrics**: Disk usage >80%, memory >85%, consensus latency

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
ENABLE_HTTP=true               # Enable HTTP upload server (API-only mode)
HTTP_PORT=8081                 # Upload server port
UPLOAD_DIR=/tmp/telemetry-uploads  # Temporary upload storage
MAX_UPLOAD_SIZE=104857600      # Max upload size (bytes)
MAX_MEMORY=33554432            # Max in-memory form parsing (bytes)
BATCH_SIZE=100                 # Records per batch
STREAM_INTERVAL=1s             # Streaming frequency
```

**Collector:**
```bash
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

### Scaling Decision Matrix

| Component | Min Instances | Max Instances | Resource/Instance | When to Scale Up | When to Scale Down |
|-----------|---------------|---------------|-------------------|------------------|-------------------|
| **Streamer** | 1 | 10 | 100m CPU, 128Mi RAM | • CPU >70% sustained<br/>• Message queue depth growing<br/>• CSV processing backlog | • CPU <30% sustained<br/>• Message queue empty<br/>• No CSV processing |
| **Collector** | 1 | 10 | 200m CPU, 256Mi RAM | • Message queue depth >1000<br/>• Processing latency >5s<br/>• Memory >80% | • Message queue depth <100<br/>• Processing latency <1s<br/>• Memory <40% |
| **Gateway** | 2 | 5 | 200m CPU, 256Mi RAM | • Response time >100ms<br/>• Request rate increasing<br/>• CPU >60% | • Response time <50ms<br/>• Low request rate<br/>• CPU <30% |
| **etcd** | 3 | 5 | 1 CPU, 2Gi RAM | • Disk usage >80%<br/>• Memory >85%<br/>• High query latency | • Vertical scaling only<br/>• Monitor and optimize |

### Auto-Scaling Configuration

#### Horizontal Pod Autoscaler (HPA) Settings

**Nexus Streamer HPA:**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: nexus-streamer-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nexus-streamer
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

**Nexus Collector HPA:**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: nexus-collector-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nexus-collector
  minReplicas: 1
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 75
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

**Nexus Gateway HPA:**
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: nexus-gateway-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: nexus-gateway
  minReplicas: 2
  maxReplicas: 5
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60
```

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
- [Production Deployment in README](../README.md#production-deployment) - Setup instructions

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
