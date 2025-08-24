# Multi-Cluster Configuration Guide

This guide explains how to configure the telemetry pipeline services for multi-cluster Kubernetes deployments where streamers can run in different clusters from collectors.

## Problem Statement

In multi-cluster environments, services need to register themselves with addresses that are accessible across cluster boundaries. The default behavior of using Pod IPs or localhost addresses will not work for cross-cluster communication.

## Service Address Resolution

The telemetry pipeline uses a priority-based approach to determine the correct service address for registration:

### Priority Order

1. **`SERVICE_ADDRESS`** - Explicit override (highest priority)
2. **`EXTERNAL_SERVICE_ADDRESS`** - Cross-cluster communication address
3. **Pod IP** - From Kubernetes downward API (`POD_IP` env var)
4. **Hostname resolution** - Standard Kubernetes hostname resolution
5. **Network interface detection** - First non-loopback IPv4 address
6. **Localhost fallback** - Only for local development

## Multi-Cluster Configuration

### Option 1: LoadBalancer Service (Recommended)

Configure your streamer service with a LoadBalancer and set the external IP:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nexus-streamer
spec:
  type: LoadBalancer
  selector:
    app: nexus-streamer
  ports:
  - port: 8081
    targetPort: 8081
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-streamer
spec:
  template:
    spec:
      containers:
      - name: nexus-streamer
        env:
        - name: EXTERNAL_SERVICE_ADDRESS
          value: "203.0.113.50"  # LoadBalancer external IP
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
```

### Option 2: NodePort Service

Use NodePort service with the node's external IP:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: nexus-streamer
spec:
  type: NodePort
  selector:
    app: nexus-streamer
  ports:
  - port: 8081
    targetPort: 8081
    nodePort: 30081
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-streamer
spec:
  template:
    spec:
      containers:
      - name: nexus-streamer
        env:
        - name: EXTERNAL_SERVICE_ADDRESS
          value: "worker-node-external-ip"  # External IP of the node
        - name: HTTP_PORT
          value: "30081"  # NodePort
```

### Option 3: Ingress with External DNS

Use Ingress controller with external DNS:

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: nexus-streamer
spec:
  rules:
  - host: streamer.cluster-a.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: nexus-streamer
            port:
              number: 8081
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-streamer
spec:
  template:
    spec:
      containers:
      - name: nexus-streamer
        env:
        - name: EXTERNAL_SERVICE_ADDRESS
          value: "streamer.cluster-a.example.com"
        - name: HTTP_PORT
          value: "443"  # HTTPS port
```

## Environment Variables

### Core Configuration

- **`SERVICE_ADDRESS`**: Override all automatic detection (use for testing)
- **`EXTERNAL_SERVICE_ADDRESS`**: Address accessible from other clusters
- **`POD_IP`**: Set by Kubernetes downward API
- **`HOSTNAME`**: Set by Kubernetes
- **`HTTP_PORT`**: Service port (default: 8081 for streamer)

### Kubernetes-specific

- **`SERVICE_NAME`**: Name of the Kubernetes service
- **`NAMESPACE`**: Kubernetes namespace
- **`CLUSTER_ID`**: Logical cluster identifier for service discovery

## Example Multi-Cluster Setup

### Cluster A (Streamer)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-streamer
  namespace: telemetry
spec:
  template:
    spec:
      containers:
      - name: nexus-streamer
        image: nexus-streamer:latest
        env:
        - name: CLUSTER_ID
          value: "cluster-a"
        - name: STREAMER_ID
          value: "streamer-cluster-a-1"
        - name: EXTERNAL_SERVICE_ADDRESS
          value: "203.0.113.50"  # LoadBalancer IP
        - name: HTTP_PORT
          value: "8081"
        - name: ETCD_ENDPOINTS
          value: "etcd-cluster.shared.svc.cluster.local:2379"
        - name: POD_IP
          valueFrom:
            fieldRef:
              fieldPath: status.podIP
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        - name: SERVICE_NAME
          value: "nexus-streamer"
```

### Cluster B (Collector)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-collector
  namespace: telemetry
spec:
  template:
    spec:
      containers:
      - name: nexus-collector
        image: nexus-collector:latest
        env:
        - name: CLUSTER_ID
          value: "cluster-b"
        - name: COLLECTOR_ID
          value: "collector-cluster-b-1"
        - name: ETCD_ENDPOINTS
          value: "etcd-cluster.shared.svc.cluster.local:2379"
```

## Service Discovery

Services register themselves in etcd with the following key structure:

```
/services/nexus-streamer/streamer-cluster-a-1
```

The registration includes:

```json
{
  "id": "streamer-cluster-a-1",
  "type": "nexus-streamer",
  "address": "203.0.113.50",
  "port": 8081,
  "metadata": {
    "cluster_id": "cluster-a",
    "mode": "api-only",
    "version": "2.0.0",
    "endpoint": "203.0.113.50:8081"
  },
  "health": "healthy",
  "version": "2.0.0",
  "registered_at": "2024-01-15T10:30:00Z"
}
```

## Troubleshooting

### Check Service Registration

```bash
# Check what address the service registered with
kubectl logs deployment/nexus-streamer | grep "Service will register with address"

# Check service discovery in etcd
etcdctl get /services/nexus-streamer/ --prefix
```

### Common Issues

1. **Service registers with Pod IP**: Set `EXTERNAL_SERVICE_ADDRESS`
2. **Service registers with localhost**: Check network interfaces and DNS resolution
3. **Cross-cluster communication fails**: Verify LoadBalancer/NodePort/Ingress configuration
4. **Service not discoverable**: Check etcd connectivity and key structure

### Testing Connectivity

```bash
# Test from collector to streamer
curl http://203.0.113.50:8081/api/v1/health

# Test service discovery
curl http://203.0.113.50:8081/api/v1/status
```

## Security Considerations

- Use TLS for cross-cluster communication
- Implement proper network policies
- Secure etcd access with authentication
- Use service mesh for advanced traffic management

## Performance Optimization

- Place etcd cluster in a central, well-connected location
- Use dedicated network links for cross-cluster communication
- Monitor latency and adjust timeouts accordingly
- Consider regional clustering for geographically distributed deployments
