#!/bin/bash

# Helper script to create Helm values files for Kind deployment
# Usage: ./scripts/create-kind-values.sh <streamer_instances> <collector_instances> <api_gw_instances>

set -euo pipefail

STREAMER_INSTANCES=${1:-2}
COLLECTOR_INSTANCES=${2:-2}
API_GW_INSTANCES=${3:-2}

echo "Creating Helm values files for Kind deployment..."
echo "  - Streamers: $STREAMER_INSTANCES"
echo "  - Collectors: $COLLECTOR_INSTANCES" 
echo "  - API Gateways: $API_GW_INSTANCES"

# Create central cluster values
cat > /tmp/kind-central-values.yaml << 'EOF'
# Central cluster configuration for Kind
streamer:
  enabled: false
collector:
  enabled: true
  replicaCount: COLLECTOR_INSTANCES_PLACEHOLDER
  resources:
    limits:
      cpu: 500m
      memory: 512Mi
    requests:
      cpu: 100m
      memory: 128Mi
apiGateway:
  enabled: true
  replicaCount: API_GW_INSTANCES_PLACEHOLDER
  service:
    type: ClusterIP
  ingress:
    enabled: false
  resources:
    limits:
      cpu: 500m
      memory: 256Mi
    requests:
      cpu: 100m
      memory: 64Mi
postgresql:
  enabled: true
  auth:
    postgresPassword: postgres
    username: telemetry
    password: telemetry
    database: telemetry
  primary:
    persistence:
      enabled: false
    resources:
      limits:
        cpu: 500m
        memory: 512Mi
      requests:
        cpu: 100m
        memory: 128Mi
redis:
  enabled: true
  architecture: standalone
  auth:
    enabled: false
  master:
    persistence:
      enabled: false
    resources:
      limits:
        cpu: 200m
        memory: 256Mi
      requests:
        cpu: 50m
        memory: 64Mi
externalRedis:
  enabled: false
monitoring:
  enabled: false
autoscaling:
  enabled: false
networkPolicy:
  enabled: false
EOF

# Replace placeholders
sed -i.bak "s/COLLECTOR_INSTANCES_PLACEHOLDER/$COLLECTOR_INSTANCES/g" /tmp/kind-central-values.yaml
sed -i.bak "s/API_GW_INSTANCES_PLACEHOLDER/$API_GW_INSTANCES/g" /tmp/kind-central-values.yaml
rm -f /tmp/kind-central-values.yaml.bak

# Create edge cluster values
cat > /tmp/kind-edge-values.yaml << 'EOF'
# Edge cluster configuration for Kind
streamer:
  enabled: true
  replicaCount: 1
  resources:
    limits:
      cpu: 200m
      memory: 256Mi
    requests:
      cpu: 50m
      memory: 64Mi
collector:
  enabled: false
apiGateway:
  enabled: false
postgresql:
  enabled: false
redis:
  enabled: false
externalRedis:
  enabled: true
  host: host.docker.internal
  port: 6379
  auth:
    enabled: false
monitoring:
  enabled: false
autoscaling:
  enabled: false
networkPolicy:
  enabled: false
EOF

echo "✅ Helm values files created:"
echo "  - Central cluster: /tmp/kind-central-values.yaml"
echo "  - Edge cluster: /tmp/kind-edge-values.yaml"
