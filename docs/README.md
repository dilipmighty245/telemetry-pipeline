# Documentation

This directory contains comprehensive documentation for the Elastic GPU Telemetry Pipeline.

## Core Documentation

### [Architecture](ARCHITECTURE.md)
Detailed system architecture, component design, and data flow diagrams.

### [API Specification](API_SPECIFICATION.md)
Complete REST API documentation with required endpoints and examples.

### [etcd Message Queue](ETCD_MESSAGE_QUEUE.md)
Custom message queue implementation using etcd for pub/sub messaging.

### [Nexus Integration](NEXUS_INTEGRATION_GUIDE.md)
Integration guide for Nexus framework components and features.

### [Scaling and Kubernetes](SCALING_AND_KUBERNETES.md)
Kubernetes deployment, scaling strategies, and production considerations.

### [Debugging Guide](DEBUGGING.md)
Troubleshooting guide for common issues and debugging techniques.

## Quick Reference

### API Endpoints (Required)
- `GET /api/v1/gpus` - List all GPUs
- `GET /api/v1/gpus/{id}/telemetry` - Query telemetry by GPU with time filters

### Component Ports
- **Streamer**: 8081 (CSV upload and streaming)
- **Gateway**: 8080 (REST API queries)
- **Collector**: N/A (internal message processing)

### Key Configuration
- **Max Instances**: 10 streamers, 10 collectors (per requirements)
- **Message Queue**: etcd-based custom implementation
- **Data Format**: CSV with GPU telemetry metrics

## Auto-Generated Documentation

### OpenAPI Specification
The REST API specification is auto-generated and available at:
- **Swagger UI**: `http://localhost:8080/swagger/`
- **OpenAPI JSON**: `http://localhost:8080/swagger/spec.json`
- **Generated Files**: See `./generated/` directory for auto-generated documentation files

Generate updated docs:
```bash
make generate-openapi
```

## Development Workflow

1. **Architecture Review**: Start with [ARCHITECTURE.md](ARCHITECTURE.md)
2. **API Design**: Review [API_SPECIFICATION.md](API_SPECIFICATION.md)
3. **Deployment**: Follow [SCALING_AND_KUBERNETES.md](SCALING_AND_KUBERNETES.md)
4. **Troubleshooting**: Use [DEBUGGING.md](DEBUGGING.md) for issues

## Contributing to Documentation

When updating documentation:
1. Keep it aligned with project requirements
2. Update auto-generated specs with `make generate-openapi`
3. Verify examples and code snippets work
4. Update this README if adding new documents