# Kubernetes Deployment Strategy & Helm Charts

## Table of Contents
1. [Deployment Overview](#deployment-overview)
2. [Namespace Organization](#namespace-organization)
3. [Helm Chart Structure](#helm-chart-structure)
4. [Component Deployments](#component-deployments)
5. [Configuration Management](#configuration-management)
6. [Storage Strategy](#storage-strategy)
7. [Networking and Services](#networking-and-services)
8. [Auto-scaling Configuration](#auto-scaling-configuration)
9. [Monitoring and Observability](#monitoring-and-observability)
10. [Security Considerations](#security-considerations)
11. [Environment Management](#environment-management)
12. [Operational Procedures](#operational-procedures)

## Deployment Overview

The GPU Telemetry Pipeline is deployed as a cloud-native application on Kubernetes using Helm charts. The deployment strategy emphasizes:

- **Microservices Architecture**: Each component deployed independently
- **Scalability**: Horizontal pod autoscaling for dynamic workloads
- **Reliability**: Multi-replica deployments with health checks
- **Observability**: Comprehensive monitoring and logging
- **Security**: Network policies and RBAC implementation

### Deployment Environments
- **Development**: Single-node cluster, minimal resources
- **Staging**: Multi-node cluster, production-like configuration
- **Production**: Multi-AZ cluster, high availability setup

## Namespace Organization

```yaml
# Namespace structure
apiVersion: v1
kind: Namespace
metadata:
  name: gpu-telemetry-system
  labels:
    app.kubernetes.io/name: gpu-telemetry
    app.kubernetes.io/version: "1.0.0"
    environment: production
---
apiVersion: v1
kind: Namespace
metadata:
  name: gpu-telemetry-monitoring
  labels:
    app.kubernetes.io/name: gpu-telemetry-monitoring
    app.kubernetes.io/version: "1.0.0"
---
apiVersion: v1
kind: Namespace
metadata:
  name: gpu-telemetry-data
  labels:
    app.kubernetes.io/name: gpu-telemetry-data
    app.kubernetes.io/version: "1.0.0"
```

### Namespace Responsibilities
- **gpu-telemetry-system**: Core application components
- **gpu-telemetry-monitoring**: Prometheus, Grafana, alerting
- **gpu-telemetry-data**: Databases, caches, storage services

## Helm Chart Structure

```
charts/
├── gpu-telemetry-pipeline/           # Main umbrella chart
│   ├── Chart.yaml
│   ├── values.yaml
│   ├── values-dev.yaml
│   ├── values-staging.yaml
│   ├── values-prod.yaml
│   ├── templates/
│   │   ├── NOTES.txt
│   │   ├── _helpers.tpl
│   │   ├── configmap.yaml
│   │   ├── secret.yaml
│   │   └── networkpolicy.yaml
│   └── charts/                       # Sub-charts
│       ├── message-queue/
│       ├── telemetry-streamer/
│       ├── telemetry-collector/
│       ├── api-gateway/
│       ├── timescaledb/
│       └── monitoring/
├── message-queue/                    # Message Queue sub-chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── statefulset.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       ├── pvc.yaml
│       └── servicemonitor.yaml
├── telemetry-streamer/              # Streamer sub-chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       ├── hpa.yaml
│       └── servicemonitor.yaml
├── telemetry-collector/             # Collector sub-chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── configmap.yaml
│       ├── hpa.yaml
│       └── servicemonitor.yaml
├── api-gateway/                     # API Gateway sub-chart
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── ingress.yaml
│       ├── configmap.yaml
│       ├── hpa.yaml
│       └── servicemonitor.yaml
└── monitoring/                      # Monitoring sub-chart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── prometheus/
        ├── grafana/
        └── alertmanager/
```

## Component Deployments

### 1. Message Queue StatefulSet

```yaml
# charts/message-queue/templates/statefulset.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{ include "message-queue.fullname" . }}
  labels:
    {{- include "message-queue.labels" . | nindent 4 }}
spec:
  serviceName: {{ include "message-queue.fullname" . }}-headless
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "message-queue.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
      labels:
        {{- include "message-queue.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "message-queue.serviceAccountName" . }}
      containers:
      - name: message-queue
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        ports:
        - name: broker
          containerPort: 9092
          protocol: TCP
        - name: metrics
          containerPort: 8080
          protocol: TCP
        env:
        - name: BROKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: CLUSTER_BOOTSTRAP_SERVERS
          value: {{ include "message-queue.bootstrapServers" . }}
        volumeMounts:
        - name: data
          mountPath: /data
        - name: config
          mountPath: /etc/config
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          {{- toYaml .Values.resources | nindent 12 }}
      volumes:
      - name: config
        configMap:
          name: {{ include "message-queue.fullname" . }}
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: {{ .Values.persistence.storageClass }}
      resources:
        requests:
          storage: {{ .Values.persistence.size }}
```

### 2. Telemetry Streamer Deployment

```yaml
# charts/telemetry-streamer/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "telemetry-streamer.fullname" . }}
  labels:
    {{- include "telemetry-streamer.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "telemetry-streamer.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
      labels:
        {{- include "telemetry-streamer.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "telemetry-streamer.serviceAccountName" . }}
      containers:
      - name: streamer
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        ports:
        - name: metrics
          containerPort: 8080
          protocol: TCP
        env:
        - name: MESSAGE_QUEUE_SERVERS
          value: {{ .Values.messageQueue.servers }}
        - name: CSV_FILE_PATH
          value: {{ .Values.csvData.filePath }}
        - name: STREAMING_INTERVAL
          value: {{ .Values.streaming.interval }}
        - name: BATCH_SIZE
          value: {{ .Values.streaming.batchSize | quote }}
        volumeMounts:
        - name: csv-data
          mountPath: /data/csv
          readOnly: true
        - name: config
          mountPath: /etc/config
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          {{- toYaml .Values.resources | nindent 12 }}
      volumes:
      - name: csv-data
        configMap:
          name: {{ .Values.csvData.configMapName }}
      - name: config
        configMap:
          name: {{ include "telemetry-streamer.fullname" . }}
```

### 3. Telemetry Collector Deployment

```yaml
# charts/telemetry-collector/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "telemetry-collector.fullname" . }}
  labels:
    {{- include "telemetry-collector.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "telemetry-collector.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
      labels:
        {{- include "telemetry-collector.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "telemetry-collector.serviceAccountName" . }}
      containers:
      - name: collector
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        ports:
        - name: metrics
          containerPort: 8080
          protocol: TCP
        env:
        - name: MESSAGE_QUEUE_SERVERS
          value: {{ .Values.messageQueue.servers }}
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: {{ .Values.database.secretName }}
              key: url
        - name: CONSUMER_GROUP_ID
          value: {{ .Values.consumer.groupId }}
        - name: BATCH_SIZE
          value: {{ .Values.consumer.batchSize | quote }}
        volumeMounts:
        - name: config
          mountPath: /etc/config
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          {{- toYaml .Values.resources | nindent 12 }}
      volumes:
      - name: config
        configMap:
          name: {{ include "telemetry-collector.fullname" . }}
```

### 4. API Gateway Deployment

```yaml
# charts/api-gateway/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "api-gateway.fullname" . }}
  labels:
    {{- include "api-gateway.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      {{- include "api-gateway.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      annotations:
        checksum/config: {{ include (print $.Template.BasePath "/configmap.yaml") . | sha256sum }}
      labels:
        {{- include "api-gateway.selectorLabels" . | nindent 8 }}
    spec:
      serviceAccountName: {{ include "api-gateway.serviceAccountName" . }}
      containers:
      - name: api-gateway
        image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
        imagePullPolicy: {{ .Values.image.pullPolicy }}
        ports:
        - name: http
          containerPort: 8080
          protocol: TCP
        - name: metrics
          containerPort: 9090
          protocol: TCP
        env:
        - name: DATABASE_URL
          valueFrom:
            secretKeyRef:
              name: {{ .Values.database.secretName }}
              key: url
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: {{ .Values.redis.secretName }}
              key: url
        - name: SERVER_PORT
          value: "8080"
        - name: METRICS_PORT
          value: "9090"
        volumeMounts:
        - name: config
          mountPath: /etc/config
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          {{- toYaml .Values.resources | nindent 12 }}
      volumes:
      - name: config
        configMap:
          name: {{ include "api-gateway.fullname" . }}
```

## Configuration Management

### Main Values File
```yaml
# charts/gpu-telemetry-pipeline/values.yaml
global:
  imageRegistry: "docker.io"
  imagePullSecrets: []
  storageClass: "standard"
  
# Message Queue Configuration
message-queue:
  enabled: true
  replicaCount: 3
  image:
    repository: gpu-telemetry/message-queue
    tag: "1.0.0"
    pullPolicy: IfNotPresent
  
  persistence:
    enabled: true
    storageClass: "fast-ssd"
    size: "20Gi"
  
  resources:
    requests:
      memory: "1Gi"
      cpu: "500m"
    limits:
      memory: "2Gi"
      cpu: "1000m"
  
  config:
    segmentSize: "1GB"
    retentionTime: "7d"
    replicationFactor: 3

# Telemetry Streamer Configuration
telemetry-streamer:
  enabled: true
  replicaCount: 2
  image:
    repository: gpu-telemetry/streamer
    tag: "1.0.0"
    pullPolicy: IfNotPresent
  
  autoscaling:
    enabled: true
    minReplicas: 1
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
  
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "512Mi"
      cpu: "500m"
  
  csvData:
    configMapName: "telemetry-csv-data"
    filePath: "/data/csv/dcgm_metrics.csv"
  
  streaming:
    interval: "1s"
    batchSize: 100

# Telemetry Collector Configuration
telemetry-collector:
  enabled: true
  replicaCount: 3
  image:
    repository: gpu-telemetry/collector
    tag: "1.0.0"
    pullPolicy: IfNotPresent
  
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
  
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "1Gi"
      cpu: "1000m"
  
  consumer:
    groupId: "telemetry-collectors"
    batchSize: 1000

# API Gateway Configuration
api-gateway:
  enabled: true
  replicaCount: 2
  image:
    repository: gpu-telemetry/api-gateway
    tag: "1.0.0"
    pullPolicy: IfNotPresent
  
  autoscaling:
    enabled: true
    minReplicas: 2
    maxReplicas: 10
    targetCPUUtilizationPercentage: 70
    targetMemoryUtilizationPercentage: 80
  
  resources:
    requests:
      memory: "512Mi"
      cpu: "500m"
    limits:
      memory: "1Gi"
      cpu: "1000m"
  
  ingress:
    enabled: true
    className: "nginx"
    annotations:
      nginx.ingress.kubernetes.io/rewrite-target: /
      cert-manager.io/cluster-issuer: "letsencrypt-prod"
    hosts:
      - host: api.gpu-telemetry.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: api-gateway-tls
        hosts:
          - api.gpu-telemetry.example.com

# Database Configuration
timescaledb:
  enabled: true
  image:
    repository: timescale/timescaledb-ha
    tag: "pg14-latest"
  
  persistence:
    enabled: true
    storageClass: "fast-ssd"
    size: "100Gi"
  
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
    limits:
      memory: "4Gi"
      cpu: "2000m"

# Monitoring Configuration
monitoring:
  enabled: true
  prometheus:
    enabled: true
    retention: "30d"
    storageSize: "50Gi"
  
  grafana:
    enabled: true
    adminPassword: "admin123"
    dashboards:
      enabled: true
```

### Environment-specific Values

#### Development Environment
```yaml
# charts/gpu-telemetry-pipeline/values-dev.yaml
global:
  storageClass: "standard"

message-queue:
  replicaCount: 1
  persistence:
    size: "5Gi"
  resources:
    requests:
      memory: "256Mi"
      cpu: "250m"
    limits:
      memory: "512Mi"
      cpu: "500m"

telemetry-streamer:
  replicaCount: 1
  autoscaling:
    enabled: false

telemetry-collector:
  replicaCount: 1
  autoscaling:
    enabled: false

api-gateway:
  replicaCount: 1
  autoscaling:
    enabled: false
  ingress:
    enabled: false

timescaledb:
  persistence:
    size: "20Gi"
  resources:
    requests:
      memory: "512Mi"
      cpu: "250m"
    limits:
      memory: "1Gi"
      cpu: "500m"
```

#### Production Environment
```yaml
# charts/gpu-telemetry-pipeline/values-prod.yaml
global:
  storageClass: "fast-ssd"

message-queue:
  replicaCount: 5
  persistence:
    size: "50Gi"
  resources:
    requests:
      memory: "2Gi"
      cpu: "1000m"
    limits:
      memory: "4Gi"
      cpu: "2000m"

telemetry-streamer:
  autoscaling:
    minReplicas: 3
    maxReplicas: 15

telemetry-collector:
  autoscaling:
    minReplicas: 5
    maxReplicas: 15

api-gateway:
  autoscaling:
    minReplicas: 3
    maxReplicas: 15

timescaledb:
  persistence:
    size: "500Gi"
  resources:
    requests:
      memory: "8Gi"
      cpu: "4000m"
    limits:
      memory: "16Gi"
      cpu: "8000m"
```

## Storage Strategy

### Storage Classes
```yaml
# Fast SSD storage for high-performance workloads
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fast-ssd
provisioner: kubernetes.io/gce-pd
parameters:
  type: pd-ssd
  replication-type: regional-pd
allowVolumeExpansion: true
reclaimPolicy: Retain
---
# Standard storage for less critical data
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: standard
provisioner: kubernetes.io/gce-pd
parameters:
  type: pd-standard
allowVolumeExpansion: true
reclaimPolicy: Delete
```

### Persistent Volume Claims
```yaml
# Message Queue data storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: message-queue-data-0
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 20Gi
---
# Database storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: timescaledb-data
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: fast-ssd
  resources:
    requests:
      storage: 100Gi
```

## Networking and Services

### Service Definitions
```yaml
# Message Queue Headless Service
apiVersion: v1
kind: Service
metadata:
  name: message-queue-headless
spec:
  clusterIP: None
  selector:
    app.kubernetes.io/name: message-queue
  ports:
  - name: broker
    port: 9092
    targetPort: 9092
---
# API Gateway Service
apiVersion: v1
kind: Service
metadata:
  name: api-gateway
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: api-gateway
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: metrics
    port: 9090
    targetPort: 9090
```

### Network Policies
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gpu-telemetry-network-policy
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/part-of: gpu-telemetry
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app.kubernetes.io/part-of: gpu-telemetry
    - namespaceSelector:
        matchLabels:
          name: gpu-telemetry-monitoring
  egress:
  - to:
    - podSelector:
        matchLabels:
          app.kubernetes.io/part-of: gpu-telemetry
  - to: []
    ports:
    - protocol: TCP
      port: 53
    - protocol: UDP
      port: 53
```

## Auto-scaling Configuration

### Horizontal Pod Autoscaler
```yaml
# Telemetry Streamer HPA
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: telemetry-streamer-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: telemetry-streamer
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
  behavior:
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 25
        periodSeconds: 60
```

### Custom Metrics Scaling
```yaml
# Custom metrics for message queue lag
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: telemetry-collector-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: telemetry-collector
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Pods
    pods:
      metric:
        name: message_queue_consumer_lag
      target:
        type: AverageValue
        averageValue: "1000"
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

## Monitoring and Observability

### ServiceMonitor for Prometheus
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: gpu-telemetry-metrics
  labels:
    app.kubernetes.io/name: gpu-telemetry
spec:
  selector:
    matchLabels:
      app.kubernetes.io/part-of: gpu-telemetry
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
```

### PrometheusRule for Alerting
```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: gpu-telemetry-alerts
spec:
  groups:
  - name: gpu-telemetry
    rules:
    - alert: MessageQueueHighLag
      expr: message_queue_consumer_lag > 10000
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "Message queue consumer lag is high"
        description: "Consumer lag is {{ $value }} messages"
    
    - alert: APIGatewayHighLatency
      expr: histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m])) > 0.5
      for: 5m
      labels:
        severity: warning
      annotations:
        summary: "API Gateway high latency"
        description: "99th percentile latency is {{ $value }}s"
```

## Security Considerations

### RBAC Configuration
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gpu-telemetry
  namespace: gpu-telemetry-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gpu-telemetry
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: gpu-telemetry
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: gpu-telemetry
subjects:
- kind: ServiceAccount
  name: gpu-telemetry
  namespace: gpu-telemetry-system
```

### Pod Security Policy
```yaml
apiVersion: policy/v1beta1
kind: PodSecurityPolicy
metadata:
  name: gpu-telemetry-psp
spec:
  privileged: false
  allowPrivilegeEscalation: false
  requiredDropCapabilities:
    - ALL
  volumes:
    - 'configMap'
    - 'emptyDir'
    - 'projected'
    - 'secret'
    - 'downwardAPI'
    - 'persistentVolumeClaim'
  runAsUser:
    rule: 'MustRunAsNonRoot'
  seLinux:
    rule: 'RunAsAny'
  fsGroup:
    rule: 'RunAsAny'
```

## Environment Management

### Deployment Commands
```bash
# Development deployment
helm upgrade --install gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --namespace gpu-telemetry-system \
  --create-namespace \
  --values ./charts/gpu-telemetry-pipeline/values-dev.yaml

# Staging deployment
helm upgrade --install gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --namespace gpu-telemetry-system \
  --create-namespace \
  --values ./charts/gpu-telemetry-pipeline/values-staging.yaml

# Production deployment
helm upgrade --install gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --namespace gpu-telemetry-system \
  --create-namespace \
  --values ./charts/gpu-telemetry-pipeline/values-prod.yaml
```

### Environment Validation
```bash
# Validate Helm templates
helm template gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --values ./charts/gpu-telemetry-pipeline/values-prod.yaml \
  --validate

# Dry run deployment
helm upgrade --install gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --namespace gpu-telemetry-system \
  --values ./charts/gpu-telemetry-pipeline/values-prod.yaml \
  --dry-run

# Check deployment status
kubectl get pods -n gpu-telemetry-system
kubectl get services -n gpu-telemetry-system
kubectl get pvc -n gpu-telemetry-system
```

## Operational Procedures

### Backup and Recovery
```yaml
# Database backup CronJob
apiVersion: batch/v1
kind: CronJob
metadata:
  name: database-backup
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:14
            command:
            - /bin/bash
            - -c
            - |
              pg_dump $DATABASE_URL | gzip > /backup/backup-$(date +%Y%m%d).sql.gz
              # Upload to cloud storage
              gsutil cp /backup/backup-$(date +%Y%m%d).sql.gz gs://gpu-telemetry-backups/
            env:
            - name: DATABASE_URL
              valueFrom:
                secretKeyRef:
                  name: database-secret
                  key: url
            volumeMounts:
            - name: backup-storage
              mountPath: /backup
          volumes:
          - name: backup-storage
            emptyDir: {}
          restartPolicy: OnFailure
```

### Rolling Updates
```bash
# Update image version
helm upgrade gpu-telemetry ./charts/gpu-telemetry-pipeline \
  --namespace gpu-telemetry-system \
  --set api-gateway.image.tag=1.1.0 \
  --reuse-values

# Monitor rollout
kubectl rollout status deployment/api-gateway -n gpu-telemetry-system

# Rollback if needed
helm rollback gpu-telemetry 1 -n gpu-telemetry-system
```

### Scaling Operations
```bash
# Manual scaling
kubectl scale deployment telemetry-streamer --replicas=5 -n gpu-telemetry-system

# Update HPA settings
kubectl patch hpa telemetry-streamer-hpa -n gpu-telemetry-system -p '{"spec":{"maxReplicas":15}}'

# Check autoscaling status
kubectl get hpa -n gpu-telemetry-system
```

This comprehensive deployment strategy provides a robust, scalable, and maintainable approach to deploying the GPU Telemetry Pipeline on Kubernetes using Helm charts, with proper consideration for security, monitoring, and operational requirements.
