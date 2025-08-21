# Cross-Cluster Deployment Guide

## Overview

The telemetry-pipeline is designed to support distributed deployments where components can run across different nodes, availability zones, or even separate Kubernetes clusters. This guide covers various deployment scenarios and configurations.

## Architecture for Distributed Deployment

```mermaid
graph TB
    subgraph "Cluster A - Edge/Data Source"
        CSV[CSV Files]
        S1[Streamer 1]
        S2[Streamer N]
        CSV --> S1
        CSV --> S2
    end
    
    subgraph "Shared Infrastructure"
        Redis[(Redis Cluster)]
        S1 --> Redis
        S2 --> Redis
    end
    
    subgraph "Cluster B - Processing/Storage"
        C1[Collector 1]
        C2[Collector N]
        DB[(Database)]
        API[API Gateway]
        
        Redis --> C1
        Redis --> C2
        C1 --> DB
        C2 --> DB
        DB --> API
    end
```

## Deployment Scenarios

### Scenario 1: Different Nodes, Same Cluster
**Use Case**: Resource isolation, GPU node separation

```yaml
# Streamer on GPU nodes
streamer:
  nodeSelector:
    node-type: gpu
  tolerations:
  - key: nvidia.com/gpu
    operator: Exists
    effect: NoSchedule

# Collector on CPU nodes  
collector:
  nodeSelector:
    node-type: cpu
```

### Scenario 2: Different Availability Zones
**Use Case**: High availability, disaster recovery

```yaml
# Streamer in AZ-1
streamer:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: topology.kubernetes.io/zone
            operator: In
            values: ["us-west-2a"]

# Collector in AZ-2
collector:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: topology.kubernetes.io/zone
            operator: In
            values: ["us-west-2b"]
```

### Scenario 3: Separate Kubernetes Clusters
**Use Case**: Edge computing, data locality, security isolation

#### Cluster A (Edge/Source) - Streamers Only
```yaml
# values-edge.yaml
streamer:
  enabled: true
  replicaCount: 3

collector:
  enabled: false  # Disabled in edge cluster

apiGateway:
  enabled: false  # Disabled in edge cluster

postgresql:
  enabled: false  # Disabled in edge cluster

# External Redis configuration
redis:
  enabled: false
  external:
    host: "redis.shared-infra.company.com"
    port: 6379
    password: "${REDIS_PASSWORD}"
    tls: true
```

#### Cluster B (Central/Processing) - Collectors Only
```yaml
# values-central.yaml
streamer:
  enabled: false  # Disabled in central cluster

collector:
  enabled: true
  replicaCount: 5

apiGateway:
  enabled: true
  replicaCount: 2

postgresql:
  enabled: true

# External Redis configuration
redis:
  enabled: false
  external:
    host: "redis.shared-infra.company.com" 
    port: 6379
    password: "${REDIS_PASSWORD}"
    tls: true
```

## Network Configuration

### 1. External Redis Setup

#### Redis Cluster Configuration
```yaml
# redis-cluster.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-cluster
  namespace: shared-infrastructure
spec:
  serviceName: redis-cluster
  replicas: 3
  selector:
    matchLabels:
      app: redis-cluster
  template:
    metadata:
      labels:
        app: redis-cluster
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        - containerPort: 16379  # Cluster bus port
        command:
        - redis-server
        - /etc/redis/redis.conf
        - --cluster-enabled
        - "yes"
        - --cluster-config-file
        - "nodes.conf"
        - --cluster-node-timeout
        - "5000"
        - --appendonly
        - "yes"
        volumeMounts:
        - name: redis-data
          mountPath: /data
        - name: redis-config
          mountPath: /etc/redis
  volumeClaimTemplates:
  - metadata:
      name: redis-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: redis-cluster
  namespace: shared-infrastructure
spec:
  type: LoadBalancer  # Or NodePort for cross-cluster access
  ports:
  - port: 6379
    targetPort: 6379
    name: redis
  - port: 16379
    targetPort: 16379
    name: cluster-bus
  selector:
    app: redis-cluster
```

### 2. Network Policies for Security

```yaml
# Allow streamer to Redis
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: streamer-to-redis
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/component: streamer
  policyTypes:
  - Egress
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          name: shared-infrastructure
    ports:
    - protocol: TCP
      port: 6379

---
# Allow collector from Redis
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: collector-from-redis
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/component: collector
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: shared-infrastructure
    ports:
    - protocol: TCP
      port: 6379
```

### 3. Service Discovery

#### External Service for Cross-Cluster Redis
```yaml
# In each cluster that needs Redis access
apiVersion: v1
kind: Service
metadata:
  name: external-redis
spec:
  type: ExternalName
  externalName: redis.shared-infra.company.com
  ports:
  - port: 6379
    targetPort: 6379
```

## Security Considerations

### 1. TLS Configuration

#### Redis with TLS
```yaml
# Redis TLS configuration
redis:
  tls:
    enabled: true
    cert: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----
    key: |
      -----BEGIN PRIVATE KEY-----
      ...
      -----END PRIVATE KEY-----
```

#### Application TLS Configuration
```go
// Update redis_backend.go for TLS support
func NewRedisBackend() (*RedisBackend, error) {
    redisURL := os.Getenv("REDIS_URL")
    if redisURL == "" {
        return nil, fmt.Errorf("REDIS_URL environment variable not set")
    }

    opts, err := redis.ParseURL(redisURL)
    if err != nil {
        return nil, fmt.Errorf("failed to parse Redis URL: %w", err)
    }

    // Add TLS configuration if enabled
    if os.Getenv("REDIS_TLS_ENABLED") == "true" {
        opts.TLSConfig = &tls.Config{
            ServerName: opts.Addr,
            MinVersion: tls.VersionTLS12,
        }
        
        // Load custom CA if provided
        if caCert := os.Getenv("REDIS_CA_CERT"); caCert != "" {
            caCertPool := x509.NewCertPool()
            caCertPool.AppendCertsFromPEM([]byte(caCert))
            opts.TLSConfig.RootCAs = caCertPool
        }
    }

    client := redis.NewClient(opts)
    // ... rest of the function
}
```

### 2. Authentication and Authorization

#### Redis Authentication
```yaml
env:
- name: REDIS_URL
  value: "rediss://username:password@redis.shared-infra.company.com:6379"
- name: REDIS_TLS_ENABLED
  value: "true"
- name: REDIS_CA_CERT
  valueFrom:
    secretKeyRef:
      name: redis-ca-cert
      key: ca.crt
```

## Monitoring Cross-Cluster Deployments

### 1. Network Latency Monitoring
```yaml
# ServiceMonitor for network metrics
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: cross-cluster-network
spec:
  selector:
    matchLabels:
      app: network-probe
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### 2. Custom Metrics for Cross-Cluster Health
```go
// Add to your metrics collection
var (
    crossClusterLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "cross_cluster_latency_seconds",
            Help: "Latency for cross-cluster communication",
        },
        []string{"source_cluster", "target_cluster", "component"},
    )
    
    messageQueueConnectionStatus = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "message_queue_connection_status",
            Help: "Status of message queue connection (1=connected, 0=disconnected)",
        },
        []string{"cluster", "component"},
    )
)
```

## Deployment Commands

### 1. Edge Cluster Deployment (Streamers)
```bash
# Deploy to edge cluster
kubectl config use-context edge-cluster

helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-edge.yaml \
  --set redis.external.host="redis.shared-infra.company.com" \
  --set redis.external.password="${REDIS_PASSWORD}"
```

### 2. Central Cluster Deployment (Collectors)
```bash
# Deploy to central cluster  
kubectl config use-context central-cluster

helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-central.yaml \
  --set postgresql.auth.password="${DB_PASSWORD}" \
  --set redis.external.password="${REDIS_PASSWORD}"
```

### 3. Shared Infrastructure Deployment
```bash
# Deploy Redis cluster
kubectl config use-context shared-infra-cluster

helm upgrade --install redis-cluster bitnami/redis-cluster \
  --namespace shared-infrastructure \
  --create-namespace \
  --set auth.password="${REDIS_PASSWORD}" \
  --set tls.enabled=true \
  --set persistence.enabled=true \
  --set persistence.size=50Gi
```

## Troubleshooting

### Common Issues

1. **Network Connectivity**
```bash
# Test Redis connectivity from streamer pod
kubectl exec -it streamer-pod -- redis-cli -h redis.shared-infra.company.com -p 6379 ping

# Test from collector pod
kubectl exec -it collector-pod -- redis-cli -h redis.shared-infra.company.com -p 6379 ping
```

2. **DNS Resolution**
```bash
# Test DNS resolution
kubectl exec -it streamer-pod -- nslookup redis.shared-infra.company.com
```

3. **Certificate Issues**
```bash
# Check TLS certificate
kubectl exec -it streamer-pod -- openssl s_client -connect redis.shared-infra.company.com:6379 -servername redis.shared-infra.company.com
```

### Monitoring Commands
```bash
# Check cross-cluster metrics
kubectl port-forward svc/prometheus 9090:9090
# Access http://localhost:9090 and query: cross_cluster_latency_seconds

# Check message queue status
kubectl logs -f deployment/streamer -c streamer | grep "Redis"
kubectl logs -f deployment/collector -c collector | grep "Redis"
```

## Best Practices

1. **Network Latency**: Keep Redis geographically close to both clusters
2. **Monitoring**: Monitor cross-cluster network latency and connection health
3. **Security**: Always use TLS for cross-cluster communication
4. **Failover**: Implement Redis clustering for high availability
5. **Resource Planning**: Account for network bandwidth in capacity planning
6. **Testing**: Regularly test failover scenarios and network partitions

This configuration enables flexible deployment patterns while maintaining security and observability across distributed environments.
