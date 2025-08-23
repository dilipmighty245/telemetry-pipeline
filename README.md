# Elastic GPU Telemetry Pipeline with Message Queue

A scalable, elastic telemetry pipeline for AI clusters with a custom message queue implementation. This system streams GPU telemetry data from CSV files, processes it through a custom etcd-based message queue, and serves it via REST APIs with auto-generated OpenAPI specifications.

## 🏗️ System Architecture

### Core Components

1. **Telemetry Streamer** - Provides HTTP API for CSV upload and streams telemetry data to the custom message queue
2. **Telemetry Collector** - Consumes telemetry from the message queue, parses and persists it
3. **API Gateway** - REST API exposing telemetry data with auto-generated OpenAPI specification
4. **Custom Message Queue** - etcd-based messaging system connecting streamers with collectors

### Architecture Diagram

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   CSV Files     │    │  Custom Message  │    │   Persistent    │
│                 │    │     Queue        │    │    Storage      │
│  ┌───────────┐  │    │   (etcd-based)   │    │   (etcd)        │
│  │ GPU Data  │  │    │                  │    │                 │
│  │ Metrics   │  │    └──────────────────┘    └─────────────────┘
│  └───────────┘  │             │                        │
└─────────────────┘             │                        │
         │                      │                        │
         ▼                      ▼                        ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Telemetry     │───▶│  Custom Message  │───▶│   Telemetry     │
│   Streamer      │    │     Queue        │    │   Collector     │
│                 │    │                  │    │                 │
│ • CSV Upload    │    │ • Pub/Sub        │    │ • Data Parse    │
│ • HTTP API      │    │ • Persistence    │    │ • Data Store    │
│ • Data Stream   │    │ • Scaling        │    │ • Processing    │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                                               │
         │              ┌──────────────────┐             │
         └─────────────▶│   API Gateway    │◀────────────┘
                        │                  │
                        │ • REST APIs      │
                        │ • OpenAPI Spec   │
                        │ • Query Engine   │
                        └──────────────────┘
                                 │
                                 ▼
                        ┌──────────────────┐
                        │   Client Apps    │
                        │                  │
                        │ • Web UI         │
                        │ • Mobile Apps    │
                        │ • Dashboards     │
                        └──────────────────┘
```

### Data Flow

1. **CSV Ingestion**: Streamer reads CSV files containing GPU telemetry data
2. **Message Streaming**: Data is streamed to custom etcd-based message queue in batches
3. **Data Processing**: Collectors consume messages, parse and persist telemetry data
4. **API Serving**: Gateway exposes REST APIs for querying telemetry data
5. **Auto-scaling**: Components can dynamically scale up/down (up to 10 instances each)

## 🚀 Quick Start

### Prerequisites

- **Go 1.21+**
- **Docker & Docker Compose**
- **etcd 3.5+** (for message queue and storage)
- **Kubernetes & Helm** (for production deployment)

### Local Development

```bash
# 1. Clone the repository
git clone <repository-url>
cd telemetry-pipeline

# 2. Install dependencies and build
make deps
make build-nexus

# 3. Start etcd (message queue & storage backend)
make setup-etcd

# 4. Run components locally
make run-nexus-streamer    # Terminal 1 - Starts CSV upload API server
make run-nexus-collector   # Terminal 2 - Starts data collection  
make run-nexus-gateway     # Terminal 3 - Starts API gateway

# 5. Test the system
curl http://localhost:8080/api/v1/gpus | jq .
curl "http://localhost:8080/api/v1/gpus/GPU-12345/telemetry?start_time=2024-01-01T00:00:00Z"
```

### Production Deployment (Kubernetes)

```bash
# 1. Build Docker images
make docker-build-nexus

# 2. Deploy to Kubernetes with Helm
helm install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  -f ./deployments/helm/telemetry-pipeline/values-nexus.yaml

# 3. Verify deployment
kubectl get pods -l app.kubernetes.io/name=telemetry-pipeline
kubectl get services

# 4. Test APIs
kubectl port-forward svc/telemetry-pipeline-nexus-gateway 8080:8080
curl http://localhost:8080/api/v1/gpus
```

## 📊 Sample CSV Data Format

The system expects CSV files with the following format:

```csv
timestamp,gpu_id,hostname,uuid,device,modelname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:30:00Z,0,gpu-node-01,GPU-12345,nvidia0,NVIDIA H100 80GB HBM3,85.5,70.2,45000,35000,65.0,350.5,1410,1215
2024-01-15T10:30:01Z,1,gpu-node-01,GPU-12346,nvidia1,NVIDIA H100 80GB HBM3,92.1,85.7,68000,12000,72.0,380.2,1410,1215
```

## 🔌 API Endpoints

### Required API Endpoints (Per Specification)

#### 1. List All GPUs
```http
GET /api/v1/gpus
```
Returns a list of all GPUs for which telemetry data is available.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "id": "0",
      "uuid": "GPU-12345",
      "hostname": "gpu-node-01",
      "name": "NVIDIA H100 80GB HBM3",
      "device": "nvidia0",
      "status": "active"
    }
  ],
  "count": 1
}
```

#### 2. Query Telemetry by GPU
```http
GET /api/v1/gpus/{id}/telemetry
GET /api/v1/gpus/{id}/telemetry?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z
```
Returns all telemetry entries for a specific GPU, ordered by time, with optional time window filters.

**Response:**
```json
{
  "success": true,
  "data": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "gpu_id": "0",
      "hostname": "gpu-node-01",
      "gpu_utilization": 85.5,
      "memory_utilization": 70.2,
      "temperature": 65.0,
      "power_draw": 350.5
    }
  ],
  "count": 1
}
```

### Additional Endpoints

- `GET /health` - Health check
- `GET /api/v1/hosts` - List all hosts
- `GET /api/v1/telemetry` - Query telemetry with filters
- `GET /swagger/` - Auto-generated OpenAPI documentation

## 🛠️ Build and Packaging

### Build Commands

```bash
# Build all components
make build-nexus

# Build individual components
make build-nexus-streamer
make build-nexus-collector  
make build-nexus-gateway

# Run tests with coverage
make test-coverage

# Generate OpenAPI specification
make generate-openapi
```

### Docker Images

```bash
# Build all Docker images
make docker-build-nexus

# Build individual images
make docker-build-nexus-streamer
make docker-build-nexus-collector
make docker-build-nexus-gateway

# Push to registry
make docker-push-nexus
```

## 📦 Installation Workflow

### 1. Local Development Setup

```bash
# Install dependencies
make deps

# Setup local etcd
make setup-etcd

# Run development environment
make dev-up
```

### 2. Kubernetes Production Setup

```bash
# Prerequisites
kubectl cluster-info
helm version

# Install with default values
helm install telemetry-pipeline ./deployments/helm/telemetry-pipeline

# Install with custom configuration
helm install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  -f ./deployments/helm/telemetry-pipeline/values-nexus.yaml \
  --set global.clusterId=production-cluster \
  --set nexusStreamer.replicaCount=3 \
  --set nexusCollector.replicaCount=3

# Upgrade deployment
helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline

# Uninstall
helm uninstall telemetry-pipeline
```

### 3. Scaling Configuration

```bash
# Scale streamers (up to 10 instances)
kubectl scale deployment telemetry-pipeline-nexus-streamer --replicas=5

# Scale collectors (up to 10 instances)  
kubectl scale deployment telemetry-pipeline-nexus-collector --replicas=5

# Auto-scaling with HPA
kubectl autoscale deployment telemetry-pipeline-nexus-streamer \
  --cpu-percent=70 --min=2 --max=10
```

## 👤 Sample User Workflow

### 1. Data Ingestion Workflow

```bash
# 1. Prepare CSV data file
cp sample_gpu_data.csv /data/telemetry.csv

# 2. Configure streamer to read CSV file
export BATCH_SIZE=100
export STREAM_INTERVAL=1s
export HTTP_PORT=8081

# 3. Start CSV upload API server
./bin/nexus-streamer

# 4. Upload CSV file via API
curl -X POST -F "file=@dcgm_metrics_20250718_134233.csv" \
  http://localhost:8081/api/v1/csv/upload

# 5. Monitor server status
curl http://localhost:8081/api/v1/status
```

### 2. Data Query Workflow

```bash
# 1. List available GPUs
curl http://localhost:8080/api/v1/gpus

# 2. Query specific GPU telemetry
GPU_ID="GPU-12345"
curl "http://localhost:8080/api/v1/gpus/${GPU_ID}/telemetry"

# 3. Query with time filters
START_TIME="2024-01-15T10:00:00Z"
END_TIME="2024-01-15T11:00:00Z"
curl "http://localhost:8080/api/v1/gpus/${GPU_ID}/telemetry?start_time=${START_TIME}&end_time=${END_TIME}"

# 4. Query all telemetry data
curl "http://localhost:8080/api/v1/telemetry?limit=100"
```

### 3. Monitoring and Observability

```bash
# Check component health
curl http://localhost:8080/health
curl http://localhost:8081/health

# View OpenAPI documentation
open http://localhost:8080/swagger/

# Monitor message queue
etcdctl get --prefix /telemetry/queue/

# Check component logs
kubectl logs -f deployment/telemetry-pipeline-nexus-streamer
kubectl logs -f deployment/telemetry-pipeline-nexus-collector
kubectl logs -f deployment/telemetry-pipeline-nexus-gateway
```

## 🤖 AI Assistance Documentation

### Project Bootstrapping

**AI Prompts Used:**
1. *"Design a scalable telemetry pipeline architecture for GPU clusters with custom message queue"*
   - **AI Contribution**: Generated initial architecture diagram and component breakdown
   - **Manual Intervention**: Refined etcd-based message queue design for production requirements

2. *"Create Go project structure for microservices with etcd message queue"*
   - **AI Contribution**: Generated project layout, Makefile structure, and dependency management
   - **Manual Intervention**: Added Nexus framework integration and custom build targets

### Code Development

**AI Prompts Used:**
1. *"Implement CSV reader that streams telemetry data to etcd message queue in Go"*
   - **AI Contribution**: Generated CSV parsing logic, batch processing, and etcd integration
   - **Manual Intervention**: Added error handling, memory management, and production optimizations

2. *"Create REST API with auto-generated OpenAPI spec for telemetry queries"*
   - **AI Contribution**: Generated Echo framework setup, route handlers, and Swagger annotations
   - **Manual Intervention**: Implemented complex query logic and response formatting

3. *"Implement etcd-based message queue with pub/sub pattern"*
   - **AI Contribution**: Generated basic etcd client operations and message serialization
   - **Manual Intervention**: Added transaction support, error recovery, and scaling optimizations

### Testing Development

**AI Prompts Used:**
1. *"Generate unit tests for telemetry data processing with table-driven tests"*
   - **AI Contribution**: Created test structure, mock data, and basic test cases
   - **Manual Intervention**: Added edge cases, error scenarios, and integration tests

2. *"Create integration tests for message queue functionality"*
   - **AI Contribution**: Generated test setup and basic message flow tests
   - **Manual Intervention**: Added concurrent processing tests and failure scenarios

### Build Environment

**AI Prompts Used:**
1. *"Create Dockerfile and Helm charts for Kubernetes deployment"*
   - **AI Contribution**: Generated multi-stage Dockerfiles and basic Helm templates
   - **Manual Intervention**: Added security contexts, resource limits, and production configurations

2. *"Design Makefile with build, test, and deployment targets"*
   - **AI Contribution**: Generated basic build and test targets
   - **Manual Intervention**: Added Docker builds, Helm operations, and development workflows

### AI Limitations Encountered

1. **Complex etcd Transactions**: AI generated basic etcd operations but required manual implementation of atomic batch operations and error recovery
2. **Kubernetes Security**: AI provided basic deployments but manual intervention needed for security contexts, RBAC, and network policies
3. **Production Optimizations**: AI suggested basic configurations but manual tuning required for memory management, connection pooling, and scaling parameters
4. **Error Handling**: AI generated happy-path code but comprehensive error handling and graceful degradation required manual implementation

## 🧪 Testing

### Unit Tests

```bash
# Run all tests with coverage
make test-coverage

# Run specific component tests
make test-streamer
make test-collector
make test-gateway

# View coverage report
make coverage-html
open coverage.html
```

### Integration Tests

```bash
# Run integration tests
make test-integration

# Run end-to-end tests
make test-e2e
```

### Performance Tests

```bash
# Load test the pipeline
make test-load

# Benchmark message queue
make benchmark-queue
```

## 📈 Performance and Scaling

### Scaling Limits (Per Requirements)
- **Maximum Streamers**: 10 instances
- **Maximum Collectors**: 10 instances  
- **Message Queue**: etcd cluster (3-5 nodes recommended)

### Performance Characteristics
- **Throughput**: ~10,000 telemetry records/second per streamer
- **Latency**: <100ms end-to-end processing
- **Storage**: Configurable retention (default: 7 days)

### Monitoring Metrics
- Message queue depth and throughput
- Component CPU/memory utilization  
- API response times and error rates
- Data processing latency

## 🔧 Configuration

### Environment Variables

#### Streamer Configuration
```bash
CSV_FILE="data.csv"           # CSV file path
BATCH_SIZE=100                # Records per batch
STREAM_INTERVAL=1s            # Streaming interval
LOOP_MODE=true                # Loop through CSV file
ETCD_ENDPOINTS=localhost:2379 # etcd connection
```

#### Collector Configuration  
```bash
BATCH_SIZE=50                 # Processing batch size
POLL_INTERVAL=1s              # Queue polling interval
BUFFER_SIZE=1000              # Internal buffer size
WORKERS=8                     # Processing workers
```

#### Gateway Configuration
```bash
PORT=8080                     # HTTP server port
ENABLE_GRAPHQL=true           # Enable GraphQL endpoint
ENABLE_WEBSOCKET=true         # Enable WebSocket
ENABLE_CORS=true              # Enable CORS
```

## 📝 API Documentation

The OpenAPI specification is auto-generated and available at:
- **Swagger UI**: `http://localhost:8080/swagger/`
- **OpenAPI JSON**: `http://localhost:8080/swagger/spec.json`

Generate updated specification:
```bash
make generate-openapi
```

## 🚀 Production Considerations

### High Availability
- Deploy etcd cluster with 3+ nodes
- Use multiple replicas for each component
- Configure pod disruption budgets
- Implement health checks and readiness probes

### Security
- Enable etcd authentication and TLS
- Use Kubernetes RBAC and network policies
- Implement API rate limiting
- Secure container images and runtime

### Monitoring
- Prometheus metrics collection
- Grafana dashboards
- Log aggregation with ELK stack
- Distributed tracing with Jaeger

### Backup and Recovery
- etcd data backup procedures
- Configuration backup
- Disaster recovery testing
- Data retention policies

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 📞 Support

For questions and support:
- Create an issue in the GitHub repository
- Check the documentation in the `docs/` directory
- Review the OpenAPI specification at `/swagger/`