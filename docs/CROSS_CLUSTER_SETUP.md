# Cross-Cluster Deployment Guide

This guide explains how to deploy the telemetry-pipeline components across different Kubernetes nodes, availability zones, or separate clusters for distributed GPU telemetry processing.

## 🏗️ Architecture Overview

The telemetry-pipeline is designed with a **decoupled architecture** that enables flexible deployment patterns:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Edge Cluster  │    │ Shared Infra    │    │ Central Cluster │
│                 │    │                 │    │                 │
│  ┌─────────────┐│    │ ┌─────────────┐ │    │ ┌─────────────┐ │
│  │  Streamers  ││────┤ │    Redis    │ ├────┤ │ Collectors  │ │
│  │             ││    │ │   Cluster   │ │    │ │             │ │
│  └─────────────┘│    │ └─────────────┘ │    │ └─────────────┘ │
│                 │    │                 │    │ ┌─────────────┐ │
│  ┌─────────────┐│    │                 │    │ │ API Gateway │ │
│  │  CSV Data   ││    │                 │    │ │             │ │
│  └─────────────┘│    │                 │    │ └─────────────┘ │
└─────────────────┘    └─────────────────┘    │ ┌─────────────┐ │
                                              │ │  Database   │ │
                                              │ └─────────────┘ │
                                              └─────────────────┘
```

## 🚀 Quick Start

### Prerequisites

1. **Multiple Kubernetes clusters** (or node groups) with kubectl access
2. **Helm 3.x** installed
3. **Redis cluster** accessible from all deployment locations
4. **Network connectivity** between clusters (for Redis communication)

### 1. Basic Cross-Cluster Deployment

```bash
# Clone the repository
git clone <repository-url>
cd telemetry-pipeline

# Deploy to edge cluster (streamers only)
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-k8s-context \
  --redis-host redis.company.com \
  --redis-password "your-redis-password" \
  deploy-edge

# Deploy to central cluster (collectors + API)
./scripts/deploy-cross-cluster.sh \
  --central-context central-k8s-context \
  --redis-host redis.company.com \
  --redis-password "your-redis-password" \
  --db-password "your-db-password" \
  deploy-central
```

### 2. Full Automated Deployment

```bash
# Deploy everything in one command
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-k8s \
  --central-context central-k8s \
  --shared-context shared-k8s \
  --redis-host redis.company.com \
  --redis-password "redis123" \
  --db-password "db123" \
  deploy-all
```

## 📋 Deployment Scenarios

### Scenario 1: Different Nodes, Same Cluster

**Use Case**: Separate GPU nodes from CPU processing nodes

```yaml
# Deploy streamers on GPU nodes
helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
  --set streamer.nodeSelector.node-type=gpu \
  --set streamer.tolerations[0].key=nvidia.com/gpu \
  --set collector.enabled=false

# Deploy collectors on CPU nodes  
helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
  --set collector.nodeSelector.node-type=cpu \
  --set streamer.enabled=false
```

### Scenario 2: Different Availability Zones

**Use Case**: High availability across AZs

```yaml
# Streamers in us-west-2a
helm upgrade --install telemetry-streamers ./deployments/helm/telemetry-pipeline \
  --values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
  --set streamer.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].values[0]=us-west-2a

# Collectors in us-west-2b  
helm upgrade --install telemetry-collectors ./deployments/helm/telemetry-pipeline \
  --values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
  --set collector.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].values[0]=us-west-2b
```

### Scenario 3: Separate Kubernetes Clusters

**Use Case**: Edge computing, data locality, security isolation

#### Edge Cluster (Data Sources)
```bash
kubectl config use-context edge-cluster

helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
  --set externalRedis.enabled=true \
  --set externalRedis.host=redis.shared-infra.company.com \
  --set externalRedis.password="your-redis-password" \
  --set externalRedis.tls.enabled=true
```

#### Central Cluster (Processing)
```bash
kubectl config use-context central-cluster

helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
  --set externalRedis.enabled=true \
  --set externalRedis.host=redis.shared-infra.company.com \
  --set externalRedis.password="your-redis-password" \
  --set postgresql.auth.password="your-db-password"
```

## 🔧 Configuration

### External Redis Configuration

For cross-cluster deployments, you need an external Redis instance accessible from all clusters:

```yaml
externalRedis:
  enabled: true
  host: "redis.shared-infra.company.com"
  port: 6379
  password: "your-redis-password"
  database: 0
  tls:
    enabled: true
    skipVerify: false
    caCert: |
      -----BEGIN CERTIFICATE-----
      ...your CA certificate...
      -----END CERTIFICATE-----
  auth:
    enabled: true
    existingSecret: "redis-credentials"
    existingSecretPasswordKey: "password"
```

### Component-Specific Configuration

#### Streamers (Edge)
```yaml
streamer:
  enabled: true
  replicaCount: 3
  config:
    batchSize: 100
    streamInterval: "1s"
    loopMode: true
  nodeSelector:
    node-type: "edge"
  tolerations:
  - key: "edge-node"
    operator: "Exists"
    effect: "NoSchedule"
```

#### Collectors (Central)
```yaml
collector:
  enabled: true
  replicaCount: 5
  config:
    batchSize: 200
    pollInterval: "1s"
    bufferSize: 2000
  nodeSelector:
    node-type: "cpu"
  affinity:
    podAntiAffinity:
      preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
            - key: app.kubernetes.io/component
              operator: In
              values: ["collector"]
          topologyKey: kubernetes.io/hostname
```

## 🔒 Security Considerations

### 1. Network Security

```yaml
# Network policy for streamers
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: streamer-network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/component: streamer
  policyTypes:
  - Egress
  egress:
  # Allow Redis access
  - to: []
    ports:
    - protocol: TCP
      port: 6379
  # Allow DNS
  - to: []
    ports:
    - protocol: TCP
      port: 53
    - protocol: UDP
      port: 53
```

### 2. TLS Configuration

```yaml
# Enable TLS for Redis connections
externalRedis:
  tls:
    enabled: true
    skipVerify: false
    caCert: |
      -----BEGIN CERTIFICATE-----
      MIIDXTCCAkWgAwIBAgIJAKoK/heBjcOuMA0GCSqGSIb3DQEBBQUAMEUxCzAJBgNV
      ...
      -----END CERTIFICATE-----
```

### 3. Secret Management

```bash
# Create Redis credentials secret
kubectl create secret generic redis-credentials \
  --from-literal=password="your-redis-password" \
  --namespace=telemetry-system

# Create database credentials secret  
kubectl create secret generic database-credentials \
  --from-literal=password="your-db-password" \
  --namespace=telemetry-system
```

## 📊 Monitoring Cross-Cluster Deployments

### 1. Prometheus Metrics

Key metrics to monitor for cross-cluster deployments:

```promql
# Cross-cluster latency
cross_cluster_latency_seconds{source_cluster="edge",target_cluster="central"}

# Message queue connection status
message_queue_connection_status{cluster="edge",component="streamer"}
message_queue_connection_status{cluster="central",component="collector"}

# Network partitions
up{job="redis-exporter",cluster="shared"}
```

### 2. Grafana Dashboard

Create dashboards to visualize:
- Cross-cluster network latency
- Message queue lag per cluster
- Component health across clusters
- Data flow rates between clusters

### 3. Alerting Rules

```yaml
groups:
- name: cross-cluster-alerts
  rules:
  - alert: CrossClusterHighLatency
    expr: cross_cluster_latency_seconds > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High latency between clusters"
      
  - alert: RedisConnectionLost
    expr: message_queue_connection_status == 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "Redis connection lost from {{ $labels.cluster }}"
```

## 🔍 Troubleshooting

### Common Issues

#### 1. Network Connectivity
```bash
# Test Redis connectivity from streamer pod
kubectl exec -it streamer-pod -- redis-cli -h redis.company.com -p 6379 ping

# Test from collector pod
kubectl exec -it collector-pod -- redis-cli -h redis.company.com -p 6379 ping
```

#### 2. DNS Resolution
```bash
# Test DNS resolution
kubectl exec -it streamer-pod -- nslookup redis.company.com
kubectl exec -it streamer-pod -- dig redis.company.com
```

#### 3. TLS Certificate Issues
```bash
# Check TLS certificate
kubectl exec -it streamer-pod -- openssl s_client -connect redis.company.com:6379 -servername redis.company.com

# Verify certificate in pod
kubectl exec -it streamer-pod -- cat /etc/ssl/certs/redis-ca.crt
```

#### 4. Authentication Problems
```bash
# Check Redis authentication
kubectl exec -it streamer-pod -- redis-cli -h redis.company.com -p 6379 -a "password" auth "password"

# Verify secret exists
kubectl get secret redis-credentials -o yaml
```

### Debugging Commands

```bash
# Check deployment status across clusters
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-k8s \
  --central-context central-k8s \
  status

# View logs from specific components
kubectl logs -f deployment/streamer -c streamer --context=edge-k8s
kubectl logs -f deployment/collector -c collector --context=central-k8s

# Check network policies
kubectl describe networkpolicy --all-namespaces --context=edge-k8s

# Monitor resource usage
kubectl top pods --context=edge-k8s
kubectl top pods --context=central-k8s
```

## 🎯 Best Practices

### 1. **Network Latency**
- Keep Redis geographically close to both clusters
- Use Redis clustering for better performance
- Monitor cross-cluster network latency continuously

### 2. **Security**
- Always use TLS for cross-cluster communication
- Implement network policies to restrict traffic
- Use Kubernetes secrets for sensitive data
- Regular security audits of cross-cluster communications

### 3. **Monitoring**
- Monitor cross-cluster network latency and connection health
- Set up alerts for network partitions
- Track message queue lag per cluster
- Monitor resource utilization across clusters

### 4. **Reliability**
- Implement Redis clustering for high availability
- Use multiple availability zones
- Regular backup of Redis and database
- Test failover scenarios regularly

### 5. **Performance**
- Optimize batch sizes based on network latency
- Use connection pooling for Redis
- Monitor and tune garbage collection in Go applications
- Account for network bandwidth in capacity planning

### 6. **Operations**
- Automate deployments using CI/CD pipelines
- Use GitOps for configuration management
- Regular testing of disaster recovery procedures
- Document runbooks for common operational tasks

## 📚 Additional Resources

- [High-Level Design](docs/HIGH_LEVEL_DESIGN.md)
- [Deployment Strategy](docs/DEPLOYMENT_STRATEGY.md)
- [Message Queue Design](docs/MESSAGE_QUEUE_DESIGN.md)
- [API Specification](docs/API_SPECIFICATION.md)

## 🤝 Contributing

When contributing to cross-cluster functionality:

1. Test with multiple cluster configurations
2. Ensure backward compatibility with single-cluster deployments
3. Update documentation for new configuration options
4. Add monitoring metrics for new cross-cluster features
5. Include security considerations in design reviews

## 📞 Support

For cross-cluster deployment issues:

1. Check the troubleshooting section above
2. Review component logs from all clusters
3. Verify network connectivity between clusters
4. Check Redis cluster health and connectivity
5. Open an issue with detailed environment information
