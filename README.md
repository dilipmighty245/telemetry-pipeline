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

```bash
make test                # Unit tests
make test-coverage       # Coverage report
make test-integration    # Integration tests
make benchmark          # Performance tests
```

## 🐳 Docker & Kubernetes Deployment

```bash
# Docker
make docker-build       # Build images
make docker-push        # Push to registry

# Kubernetes with Helm
make helm-install       # Deploy to cluster
make helm-uninstall     # Remove deployment

# Scale services
kubectl scale deployment telemetry-pipeline-streamer --replicas=3
kubectl scale deployment telemetry-pipeline-collector --replicas=2
```

## 📚 Documentation

Comprehensive documentation is available in the `docs/` directory:

- **[High Level Design](docs/HIGH_LEVEL_DESIGN.md)** - System architecture and design decisions
- **[API Specification](docs/API_SPECIFICATION.md)** - Detailed REST API documentation
- **[Message Queue Design](docs/MESSAGE_QUEUE_DESIGN.md)** - Custom message queue implementation details
- **[Deployment Strategy](docs/DEPLOYMENT_STRATEGY.md)** - Production deployment guidelines

## 🔧 Configuration

### Environment Variables

#### Database
- `DB_HOST`: PostgreSQL host (default: localhost)
- `DB_PORT`: PostgreSQL port (default: 5432)
- `DB_USER`: Database user (default: postgres)
- `DB_PASSWORD`: Database password (default: postgres)
- `DB_NAME`: Database name (default: telemetry)

#### Services
- `STREAMER_BATCH_SIZE`: Records per batch (default: 100)
- `COLLECTOR_BATCH_SIZE`: Messages per batch (default: 100)
- `PORT`: API Gateway HTTP port (default: 8080)

## 🔄 Scaling

The system supports horizontal scaling of all components:

```bash
# Scale streamers
kubectl scale deployment telemetry-pipeline-streamer --replicas=5

# Scale collectors  
kubectl scale deployment telemetry-pipeline-collector --replicas=3

# Enable auto-scaling
helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=10
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
make build              # Build all services
make test               # Run tests
make docker-build       # Build Docker images
make helm-install       # Deploy to Kubernetes
make generate-swagger   # Generate OpenAPI spec
make clean             # Clean build artifacts
```

## 🤖 AI-Assisted Development

This project was developed with extensive AI assistance:

- **Project Structure** (90% AI): Complete Go module setup, directory structure
- **Core Implementation** (85% AI): Message queue, CSV reader, API handlers
- **Testing** (80% AI): Unit tests, integration tests, benchmarks
- **Infrastructure** (95% AI): Docker, Kubernetes, Helm charts
- **Documentation** (90% AI): README, API docs, deployment guides

See [docs/](docs/) for detailed development workflow and AI assistance documentation.

## 📈 Features

✅ **Custom Message Queue** - In-memory implementation with topic management  
✅ **Dynamic Scaling** - Horizontal scaling for streamers and collectors  
✅ **REST API** - Auto-generated OpenAPI/Swagger documentation  
✅ **Kubernetes Ready** - Complete Helm charts and deployment configs  
✅ **Production Ready** - Health checks, metrics, graceful shutdowns  
✅ **Comprehensive Testing** - Unit, integration, and performance tests  

## 📄 License

MIT License - see LICENSE file for details.

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass: `make test`
5. Submit a pull request

---

**Built with extensive AI assistance for rapid development and production-ready deployment.**
