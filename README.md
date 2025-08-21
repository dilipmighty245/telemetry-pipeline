# Elastic GPU Telemetry Pipeline

A scalable, elastic telemetry pipeline for AI clusters with custom message queue implementation. This system streams GPU telemetry data from CSV files through a custom message queue to collectors that persist data to PostgreSQL, with a REST API for querying telemetry data.

## 🏗️ System Architecture

The system consists of four main components:

1. **Telemetry Streamer**: Reads CSV telemetry data and streams it to the message queue
2. **Message Queue**: Custom in-memory message queue library for reliable message delivery  
3. **Telemetry Collector**: Consumes messages from the queue and persists to PostgreSQL
4. **API Gateway**: REST API for querying telemetry data with auto-generated OpenAPI spec

## 🚀 Quick Start

### Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Kubernetes cluster (for production deployment)
- Helm 3.0+ (for Kubernetes deployment)

### Local Development

```bash
# Clone and setup
make deps

# Build all services
make build

# Start with Docker Compose
docker-compose up -d

# Test API
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/gpus
curl http://localhost:8080/api/v1/stats
```

### Manual Build and Run

```bash
# Build all services
make build

# Start PostgreSQL
make db-setup

# Run services (in separate terminals)
make run-streamer    # Terminal 1
make run-collector   # Terminal 2
make run-api-gateway # Terminal 3
```

## 📊 API Endpoints

- `GET /health` - Service health status
- `GET /api/v1/gpus` - List all GPUs with telemetry data
- `GET /api/v1/gpus/{id}/telemetry` - Get telemetry data for specific GPU
- `GET /api/v1/stats` - Overall telemetry statistics

### Query Parameters

For `/api/v1/gpus/{id}/telemetry`:
- `start_time` (optional): Start time in RFC3339 format
- `end_time` (optional): End time in RFC3339 format  
- `limit` (optional): Max records to return (default: 100, max: 1000)
- `offset` (optional): Number of records to skip (default: 0)

## 🧪 Testing

### Unit and Integration Tests

```bash
make test                # Unit tests
make test-coverage       # Coverage report
make test-integration    # Integration tests
make benchmark          # Performance tests
```

### 🏠 Local Testing

#### Option 1: Quick Start with Docker Compose (Recommended)

This is the easiest way to test both streamer and collector services together:

```bash
# Navigate to the project directory
cd telemetry-pipeline

# Start all services with Docker Compose
docker-compose up -d

# Check logs
docker-compose logs streamer
docker-compose logs collector

# Test the pipeline
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/gpus
curl http://localhost:8080/api/v1/stats

# Stop services
docker-compose down
```

#### Option 2: Manual Build and Run (Step by Step)

If you want to build and test each component individually:

```bash
# 1. Install dependencies and build
make deps
make build

# 2. Setup local database
make db-setup

# 3. Run each service in separate terminals:

# Terminal 1: Start Streamer
make run-streamer

# Terminal 2: Start Collector  
make run-collector

# Terminal 3: Start API Gateway (optional, for testing)
make run-api-gateway
```

#### Option 3: Direct Binary Execution

After building, you can run the binaries directly with custom parameters:

```bash
# Build the services
make build

# Run streamer with custom parameters
./bin/streamer \
  -csv=dcgm_metrics_20250718_134233.csv \
  -loop=true \
  -batch-size=50 \
  -stream-interval=2s \
  -log-level=debug

# Run collector with custom parameters (in another terminal)
./bin/collector \
  -batch-size=100 \
  -poll-interval=1s \
  -log-level=debug \
  -db-host=localhost \
  -db-password=postgres
```

#### Configuration Options

**Streamer Configuration:**
- `-csv`: Path to CSV file (default: `dcgm_metrics_20250718_134233.csv`)
- `-batch-size`: Records per batch (default: 100)
- `-stream-interval`: Interval between batches (default: 1s)
- `-loop`: Enable continuous streaming (default: true)
- `-log-level`: Log level (debug, info, warn, error)

**Collector Configuration:**
- `-batch-size`: Messages per batch (default: 100)
- `-poll-interval`: Polling interval (default: 1s)
- `-db-host`: Database host (default: localhost)
- `-db-password`: Database password (default: postgres)
- `-log-level`: Log level (debug, info, warn, error)

#### Monitoring and Debugging

```bash
# View logs from Docker Compose
docker-compose logs -f streamer
docker-compose logs -f collector

# Check message queue stats (services report metrics every 30 seconds)

# Test API endpoints
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/stats

# Access database via Adminer (web interface)
open http://localhost:8081

# Or connect directly to PostgreSQL
docker exec -it telemetry-postgres psql -U postgres -d telemetry
```

#### Troubleshooting Local Testing

**Common Issues:**

1. **CSV file not found:**
   - Ensure `dcgm_metrics_20250718_134233.csv` exists in the project root
   - Use absolute path: `-csv=/full/path/to/your/file.csv`

2. **Database connection issues:**
   - Check if PostgreSQL is running: `docker ps | grep postgres`
   - Verify database credentials in docker-compose.yml

3. **Port conflicts:**
   - PostgreSQL: 5432
   - Redis: 6379  
   - API Gateway: 8080
   - Adminer: 8081

**Debug Mode:**
```bash
# Run with debug logging
./bin/streamer -log-level=debug
./bin/collector -log-level=debug
```

#### Quick Validation

To quickly validate everything is working:

```bash
# Start everything
docker-compose up -d

# Wait a few seconds, then check
curl http://localhost:8080/api/v1/stats

# Should return JSON with telemetry statistics
# Example response:
# {
#   "total_records": 150,
#   "unique_gpus": 2,
#   "unique_hosts": 2
# }
```

## 🐳 Docker & Kubernetes Deployment

### Quick Start - Same Cluster Deployment

```bash
# Build and deploy everything in one cluster
make docker-build       # Build images
make docker-push        # Push to registry
make helm-install-same-cluster  # Deploy all components

# Check deployment status
make deploy-status

# Scale services
kubectl scale deployment telemetry-pipeline-streamer --replicas=3 -n telemetry-system
kubectl scale deployment telemetry-pipeline-collector --replicas=2 -n telemetry-system
```

### Cross-Cluster Deployment

#### Option 1: Using Makefile Targets

```bash
# Set environment variables
export EDGE_CONTEXT="edge-cluster-context"
export CENTRAL_CONTEXT="central-cluster-context"
export REDIS_HOST="redis.shared-infra.company.com"
export REDIS_PASSWORD="your-redis-password"
export DB_PASSWORD="your-db-password"

# Deploy to edge cluster (streamers only)
make helm-install-cross-cluster-edge

# Deploy to central cluster (collectors and API)
make helm-install-cross-cluster-central

# Or deploy all at once
make helm-install-cross-cluster-all

# Check status across clusters
make deploy-status
```

#### Option 2: Using Deployment Script

```bash
# Deploy all components across clusters
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-cluster \
  --central-context central-cluster \
  --redis-host redis.company.com \
  --redis-password "your-redis-password" \
  --db-password "your-db-password" \
  deploy-all

# Deploy only edge components
./scripts/deploy-cross-cluster.sh \
  --edge-context edge-cluster \
  --redis-host redis.company.com \
  --redis-password "your-redis-password" \
  deploy-edge

# Deploy only central components
./scripts/deploy-cross-cluster.sh \
  --central-context central-cluster \
  --redis-host redis.company.com \
  --redis-password "your-redis-password" \
  --db-password "your-db-password" \
  deploy-central
```

### Local Testing with Kind

```bash
# Setup Kind clusters for testing
make kind-setup

# Load images into Kind clusters
make kind-load-images

# Check cluster status
make kind-status

# Deploy using cross-cluster configuration
export EDGE_CONTEXT="kind-telemetry-edge"
export CENTRAL_CONTEXT="kind-telemetry-central"
export REDIS_HOST="localhost"
export REDIS_PASSWORD="test-password"
export DB_PASSWORD="test-db-password"

make helm-install-cross-cluster-all

# Cleanup
make kind-cleanup
```

## 🚀 Deployment Modes

The telemetry pipeline supports two main deployment patterns:

### 1. Same Cluster Deployment (Monolithic)

All components (streamers, collectors, API gateway, database, Redis) run in a single Kubernetes cluster.

**Pros:**
- Simple setup and management
- Lower network latency
- Easier debugging and monitoring
- No cross-cluster networking complexity

**Cons:**
- Single point of failure
- Resource contention
- Limited scalability
- No geographic distribution

**Use Cases:**
- Development and testing
- Small to medium scale deployments
- Single data center deployments
- Proof of concepts

### 2. Cross-Cluster Deployment (Distributed)

Components are distributed across multiple clusters:
- **Edge Clusters**: Run streamers close to data sources
- **Central Cluster**: Runs collectors, API gateway, and database
- **Shared Infrastructure**: External Redis for message queuing

**Pros:**
- High availability and fault tolerance
- Better resource utilization
- Geographic distribution
- Independent scaling per cluster
- Data locality (edge processing)

**Cons:**
- Complex setup and management
- Network latency considerations
- Cross-cluster networking requirements
- More monitoring complexity

**Use Cases:**
- Production deployments
- Multi-region setups
- Edge computing scenarios
- High-scale deployments

## 📚 Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory. See the [Documentation Index](docs/README.md) for a complete overview.

### Architecture & Design
- **[High Level Design](docs/HIGH_LEVEL_DESIGN.md)** - System architecture and design decisions
- **[API Specification](docs/API_SPECIFICATION.md)** - Detailed REST API documentation
- **[Message Queue Design](docs/MESSAGE_QUEUE_DESIGN.md)** - Custom message queue implementation details

### Deployment Guides
- **[Deployment Modes](docs/DEPLOYMENT-MODES.md)** - Comprehensive guide for same-cluster and cross-cluster deployments
- **[Cross-Cluster Deployment](docs/CROSS_CLUSTER_DEPLOYMENT.md)** - Detailed cross-cluster deployment guide
- **[Cross-Cluster Setup](docs/CROSS_CLUSTER_SETUP.md)** - Step-by-step cross-cluster setup instructions
- **[Kind Setup Guide](docs/KIND_SETUP.md)** - Local development with Kind clusters
- **[Deployment Strategy](docs/DEPLOYMENT_STRATEGY.md)** - Production deployment guidelines
- **[Deployment Comparison](docs/DEPLOYMENT_COMPARISON.md)** - Comparison of different deployment approaches
- **[Deployment Summary](docs/DEPLOYMENT-SUMMARY.md)** - Quick reference for deployment commands

## 🔧 Configuration

### Deployment Configuration Files

The system provides different configuration files for various deployment scenarios:

- **`values.yaml`**: Default configuration (suitable for development)
- **`values-same-cluster.yaml`**: Single cluster deployment with all components
- **`values-edge-cluster.yaml`**: Edge cluster configuration (streamers only)
- **`values-central-cluster.yaml`**: Central cluster configuration (collectors, API, database)

### Environment Variables

#### Database Configuration
- `DB_HOST`: PostgreSQL host (default: localhost)
- `DB_PORT`: PostgreSQL port (default: 5432)
- `DB_USER`: Database user (default: postgres)
- `DB_PASSWORD`: Database password (default: postgres)
- `DB_NAME`: Database name (default: telemetry)
- `DB_SSL_MODE`: SSL mode for database connection (default: disable)

#### CSV Data Source Configuration

The telemetry pipeline supports multiple ways to provide CSV data to streamers in Kubernetes:

**1. Embedded CSV (Default)**
```yaml
streamer:
  config:
    csvSource:
      type: "embedded"
    csvFile: "dcgm_metrics_20250718_134233.csv"
```

**2. CSV from ConfigMap**
```yaml
streamer:
  config:
    csvSource:
      type: "configmap"
      configMapName: "telemetry-csv-data"
      configMapKey: "telemetry.csv"
```

**3. CSV from Persistent Volume**
```yaml
streamer:
  config:
    csvSource:
      type: "pvc"
      pvcName: "telemetry-csv-pvc"
      mountPath: "/data"
      fileName: "telemetry_data.csv"
```

**4. CSV from URL**
```yaml
streamer:
  config:
    csvSource:
      type: "url"
      url: "https://example.com/telemetry_data.csv"
      refreshInterval: "1h"
```

#### Usage Examples

**Deploy with ConfigMap CSV:**
```bash
# Create ConfigMap with your CSV data
kubectl create configmap telemetry-csv-data \
  --from-file=telemetry.csv=your_data.csv \
  -n telemetry-system

# Deploy using ConfigMap values
helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-csv-configmap.yaml
```

**Deploy with PVC CSV:**
```bash
# Create PVC for CSV storage
kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: telemetry-csv-pvc
  namespace: telemetry-system
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 1Gi
EOF

# Deploy using PVC values
helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-csv-pvc.yaml
```

**Deploy with URL CSV:**
```bash
# Deploy with URL-based CSV (downloaded at startup)
helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-csv-url.yaml \
  --set streamer.config.csvSource.url="https://your-domain.com/data.csv"
```

#### Redis Configuration
- `REDIS_URL`: Redis connection URL (auto-generated based on deployment mode)
- `REDIS_PASSWORD`: Redis password (required for external Redis)
- `REDIS_TLS_ENABLED`: Enable TLS for Redis connection (default: false)
- `REDIS_CA_CERT`: CA certificate for Redis TLS verification

#### Service Configuration
- `STREAMER_BATCH_SIZE`: Records per batch (default: 100)
- `STREAMER_STREAM_INTERVAL`: Streaming interval (default: 1s)
- `STREAMER_LOOP_MODE`: Enable loop mode for continuous streaming (default: true)
- `COLLECTOR_BATCH_SIZE`: Messages per batch (default: 100)
- `COLLECTOR_POLL_INTERVAL`: Polling interval (default: 1s)
- `COLLECTOR_BUFFER_SIZE`: Buffer size for message processing (default: 1000)
- `API_GATEWAY_PORT`: API Gateway HTTP port (default: 8080)

#### Deployment Mode Variables (Makefile)
- `EDGE_CONTEXT`: Kubernetes context for edge cluster
- `CENTRAL_CONTEXT`: Kubernetes context for central cluster
- `SHARED_CONTEXT`: Kubernetes context for shared infrastructure
- `REDIS_HOST`: External Redis host for cross-cluster deployment
- `REDIS_PASSWORD`: Redis authentication password
- `DB_PASSWORD`: Database password
- `NAMESPACE`: Kubernetes namespace (default: telemetry-system)

## 🔄 Scaling

The system supports horizontal scaling of all components:

### Same Cluster Scaling

```bash
# Scale streamers
kubectl scale deployment telemetry-pipeline-streamer --replicas=5 -n telemetry-system

# Scale collectors  
kubectl scale deployment telemetry-pipeline-collector --replicas=3 -n telemetry-system

# Enable auto-scaling for same cluster
helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system \
  --values ./deployments/helm/telemetry-pipeline/values-same-cluster.yaml \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10
```

### Cross-Cluster Scaling

```bash
# Scale edge streamers
kubectl scale deployment telemetry-edge-streamer --replicas=5 \
  --context="$EDGE_CONTEXT" -n telemetry-system

# Scale central collectors
kubectl scale deployment telemetry-central-collector --replicas=8 \
  --context="$CENTRAL_CONTEXT" -n telemetry-system

# Enable auto-scaling for edge cluster
helm upgrade telemetry-edge ./deployments/helm/telemetry-pipeline \
  --kube-context "$EDGE_CONTEXT" \
  --namespace telemetry-system \
  --values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
  --set autoscaling.enabled=true \
  --set autoscaling.maxReplicas=10

# Enable auto-scaling for central cluster
helm upgrade telemetry-central ./deployments/helm/telemetry-pipeline \
  --kube-context "$CENTRAL_CONTEXT" \
  --namespace telemetry-system \
  --values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
  --set autoscaling.enabled=true \
  --set autoscaling.maxReplicas=15
```

## 🛠️ Development

### Project Structure

```
telemetry-pipeline/
├── cmd/                    # Main applications
├── internal/              # Private application code
├── pkg/                   # Public library code
├── protos/               # Protocol buffer definitions
├── deployments/          # Docker & Kubernetes configs
├── test/                 # Test files
└── docs/                 # Documentation
```

### Make Targets

```bash
# Build and test
make build              # Build all services
make test               # Run tests
make docker-build       # Build Docker images
make generate-swagger   # Generate OpenAPI spec
make clean             # Clean build artifacts

# CSV Data Management
make csv-help                                    # Show CSV management help
make csv-validate CSV_FILE=data.csv             # Validate CSV file
make csv-create-configmap CSV_FILE=data.csv     # Create ConfigMap from CSV
make csv-create-pvc SIZE=5Gi                    # Create PVC for CSV storage
make csv-upload-pvc CSV_FILE=data.csv           # Upload CSV to PVC
make csv-list                                   # List CSV data sources
make csv-deploy-configmap CSV_FILE=data.csv     # Deploy with ConfigMap CSV
make csv-deploy-pvc CSV_FILE=data.csv SIZE=5Gi  # Deploy with PVC CSV
make csv-deploy-url CSV_URL=https://example.com/data.csv  # Deploy with URL CSV

# Same cluster deployment
make helm-install-same-cluster     # Deploy all components to one cluster
make helm-uninstall               # Remove same cluster deployment

# Cross-cluster deployment
make helm-install-cross-cluster-edge     # Deploy streamers to edge cluster
make helm-install-cross-cluster-central  # Deploy collectors/API to central
make helm-install-cross-cluster-all      # Deploy all cross-cluster components
make helm-uninstall-cross-cluster        # Remove cross-cluster deployments

# Kind cluster management
make kind-setup         # Setup Kind clusters for testing
make kind-cleanup       # Clean up Kind clusters
make kind-status        # Show Kind clusters status
make kind-load-images   # Load images into Kind clusters

# Deployment management
make deploy-status      # Check deployment status
make deployment-info    # Show deployment configuration
make validate-cross-cluster-env  # Validate cross-cluster environment

# Deployment validation
make validate-same-cluster    # Validate same cluster deployment
make validate-cross-cluster   # Validate cross-cluster deployment
make validate-connectivity    # Test connectivity between components
make validate-health          # Check health endpoints
make validate-scaling         # Test auto-scaling functionality

# Generate manifests
make helm-template      # Generate same-cluster manifests
make helm-template-edge # Generate edge cluster manifests
make helm-template-central # Generate central cluster manifests
```

## 🤖 AI-Assisted Development

This project was developed with AI assistance:

- **Project Structure** (90% AI): Complete Go module setup, directory structure
- **Core Implementation** (75% AI): Message queue, CSV reader, API handlers
- **Testing** (70% AI): Unit tests, integration tests, benchmarks
- **Infrastructure** (85% AI): Docker, Kubernetes, Helm charts
- **Documentation** (80% AI): README, API docs, deployment guides

See [docs/](docs/) for detailed development workflow and AI assistance documentation.

## 🌟 Deployment Examples

### Example 1: Development Setup (Same Cluster)

```bash
# Quick development setup
make deps
make docker-build-local
make helm-install-same-cluster

# Test the deployment
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/gpus
```

### Example 2: Production Cross-Cluster Setup

```bash
# Step 1: Set up shared Redis infrastructure
export SHARED_CONTEXT="shared-infra-cluster"
export REDIS_PASSWORD="$(openssl rand -base64 32)"

./scripts/deploy-cross-cluster.sh \
  --shared-context "$SHARED_CONTEXT" \
  --redis-password "$REDIS_PASSWORD" \
  deploy-shared

# Step 2: Deploy edge streamers
export EDGE_CONTEXT="edge-us-west-1"
export REDIS_HOST="redis.shared-infra.company.com"

./scripts/deploy-cross-cluster.sh \
  --edge-context "$EDGE_CONTEXT" \
  --redis-host "$REDIS_HOST" \
  --redis-password "$REDIS_PASSWORD" \
  deploy-edge

# Step 3: Deploy central processing
export CENTRAL_CONTEXT="central-us-east-1"
export DB_PASSWORD="$(openssl rand -base64 32)"

./scripts/deploy-cross-cluster.sh \
  --central-context "$CENTRAL_CONTEXT" \
  --redis-host "$REDIS_HOST" \
  --redis-password "$REDIS_PASSWORD" \
  --db-password "$DB_PASSWORD" \
  deploy-central

# Step 4: Verify deployment
make deploy-status \
  EDGE_CONTEXT="$EDGE_CONTEXT" \
  CENTRAL_CONTEXT="$CENTRAL_CONTEXT" \
  SHARED_CONTEXT="$SHARED_CONTEXT"
```

### Example 3: Hybrid Deployment (Same Cluster + External Redis)

```bash
# Deploy with external Redis but same cluster for other components
helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --namespace telemetry-system --create-namespace \
  --values ./deployments/helm/telemetry-pipeline/values-same-cluster.yaml \
  --set externalRedis.enabled=true \
  --set externalRedis.host="redis.company.com" \
  --set externalRedis.password="your-redis-password" \
  --set redis.enabled=false
```

## 📈 Features

✅ **Custom Message Queue** - In-memory implementation with topic management  
✅ **Dynamic Scaling** - Horizontal scaling for streamers and collectors  
✅ **REST API** - Auto-generated OpenAPI/Swagger documentation  
✅ **Kubernetes Ready** - Complete Helm charts and deployment configs  
✅ **Cross-Cluster Support** - Distributed deployment across multiple clusters  
✅ **Flexible Configuration** - Same-cluster and cross-cluster deployment modes  
✅ **Production Ready** - Health checks, metrics, graceful shutdowns  
✅ **Comprehensive Testing** - Unit, integration, and performance tests  

## 📄 License

MIT License - see LICENSE file for details.

## 🔍 Troubleshooting

### Common Issues and Solutions

#### 1. Same Cluster Deployment Issues

```bash
# Check pod status
kubectl get pods -n telemetry-system

# Check pod logs
kubectl logs -f deployment/telemetry-pipeline-streamer -n telemetry-system
kubectl logs -f deployment/telemetry-pipeline-collector -n telemetry-system
kubectl logs -f deployment/telemetry-pipeline-api-gateway -n telemetry-system

# Check events for issues
kubectl get events -n telemetry-system --sort-by='.lastTimestamp'

# Validate deployment
make validate-same-cluster
```

#### 2. Cross-Cluster Deployment Issues

```bash
# Check deployment status across clusters
make deploy-status EDGE_CONTEXT=edge-ctx CENTRAL_CONTEXT=central-ctx

# Validate cross-cluster setup
make validate-cross-cluster EDGE_CONTEXT=edge-ctx CENTRAL_CONTEXT=central-ctx

# Test connectivity
make validate-connectivity EDGE_CONTEXT=edge-ctx CENTRAL_CONTEXT=central-ctx

# Check Redis connectivity from edge cluster
kubectl exec -it deployment/telemetry-edge-streamer -n telemetry-system \
  --context=edge-ctx -- sh -c 'echo "PING" | nc $REDIS_HOST $REDIS_PORT'

# Check database connectivity from central cluster
kubectl exec -it deployment/telemetry-central-collector -n telemetry-system \
  --context=central-ctx -- sh -c 'pg_isready -h $DB_HOST -p $DB_PORT'
```

#### 3. Network and Connectivity Issues

```bash
# Test DNS resolution
kubectl exec -it <pod-name> -n telemetry-system -- nslookup redis.company.com

# Test Redis TLS connection
kubectl exec -it <pod-name> -n telemetry-system -- \
  openssl s_client -connect redis.company.com:6379 -servername redis.company.com

# Check network policies
kubectl get networkpolicy -n telemetry-system
kubectl describe networkpolicy <policy-name> -n telemetry-system
```

#### 4. Performance and Scaling Issues

```bash
# Check resource usage
kubectl top pods -n telemetry-system

# Check HPA status
kubectl get hpa -n telemetry-system
kubectl describe hpa <hpa-name> -n telemetry-system

# Test scaling
make validate-scaling

# Check metrics
kubectl port-forward svc/prometheus 9090:9090
# Access http://localhost:9090 for metrics
```

#### 5. Configuration Issues

```bash
# Check configuration
kubectl get configmap telemetry-pipeline-config -n telemetry-system -o yaml

# Check secrets
kubectl get secrets -n telemetry-system

# Validate environment variables
kubectl exec -it <pod-name> -n telemetry-system -- env | grep -E "(REDIS|DB)_"

# Check deployment info
kubectl get configmap telemetry-pipeline-deployment-info -n telemetry-system -o yaml
```

### Monitoring and Observability

```bash
# Check deployment health
make validate-health

# Monitor logs in real-time
kubectl logs -f -l app.kubernetes.io/name=telemetry-pipeline -n telemetry-system --all-containers=true

# Check service endpoints
kubectl get svc -n telemetry-system
kubectl get ingress -n telemetry-system

# Test API endpoints
kubectl port-forward svc/telemetry-pipeline-api-gateway 8080:80 -n telemetry-system
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/gpus
```

### Recovery Procedures

#### Restart Components
```bash
# Restart streamers
kubectl rollout restart deployment/telemetry-pipeline-streamer -n telemetry-system

# Restart collectors
kubectl rollout restart deployment/telemetry-pipeline-collector -n telemetry-system

# Restart API gateway
kubectl rollout restart deployment/telemetry-pipeline-api-gateway -n telemetry-system
```

#### Reset Redis Queue
```bash
# Connect to Redis and flush queues
kubectl exec -it deployment/telemetry-pipeline-redis -n telemetry-system -- redis-cli FLUSHALL

# Or for external Redis
redis-cli -h redis.company.com -p 6379 -a your-password FLUSHALL
```

#### Database Recovery
```bash
# Check database status
kubectl exec -it deployment/telemetry-pipeline-postgresql -n telemetry-system -- pg_isready

# Access database console
kubectl exec -it deployment/telemetry-pipeline-postgresql -n telemetry-system -- psql -U telemetry -d telemetry

# Backup database
kubectl exec deployment/telemetry-pipeline-postgresql -n telemetry-system -- pg_dump -U telemetry telemetry > backup.sql
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass: `make test`
5. Validate your deployment: `make validate-same-cluster`
6. Submit a pull request

---

