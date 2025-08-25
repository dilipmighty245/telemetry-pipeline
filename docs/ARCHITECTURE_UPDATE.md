# Telemetry Pipeline Architecture Update

## Overview

This document describes the updated architecture of the Elastic GPU Telemetry Pipeline, where CSV upload functionality has been moved from the Gateway to the Streamer component for better separation of concerns and production scalability.

## Architecture Changes

### Previous Architecture
```
CSV Upload → Gateway → Message Queue → Collector → Storage
```

### New Architecture
```
CSV Upload → Streamer → Message Queue → Collector → Storage
Gateway → Query APIs only
```

## Component Responsibilities

### 1. Nexus Streamer (`nexus-streamer`)
**Primary Role**: Data ingestion and streaming to message queue

**Responsibilities**:
- **CSV Upload Endpoint**: Accepts CSV file uploads via HTTP POST `/api/v1/csv/upload`
- **CSV Processing**: Validates, parses, and processes CSV files
- **Data Streaming**: Streams telemetry data to etcd message queue in batches
- **File Management**: Handles temporary file storage and cleanup
- **Health Monitoring**: Provides health and status endpoints

**HTTP Endpoints**:
- `POST /api/v1/csv/upload` - Upload and process CSV files
- `GET /health` - Health check
- `GET /api/v1/status` - Service status and metrics

**Configuration**:
- HTTP Port: 8081 (configurable)
- Max Upload Size: 100MB (configurable)
- Batch Size: 100 records (configurable)
- Stream Interval: 1 second (configurable)

### 2. Nexus Gateway (`nexus-gateway`)
**Primary Role**: REST API for querying telemetry data

**Responsibilities**:
- **Query APIs**: Provides REST endpoints for querying telemetry data
- **GraphQL Support**: GraphQL endpoint for flexible queries
- **WebSocket Support**: Real-time data streaming
- **API Documentation**: Auto-generated OpenAPI/Swagger documentation

**HTTP Endpoints**:
- `GET /api/v1/gpus` - List all GPUs
- `GET /api/v1/gpus/{id}/telemetry` - Query telemetry by GPU
- `GET /api/v1/hosts` - List all hosts
- `GET /api/v1/telemetry` - Query telemetry with filters
- `POST /graphql` - GraphQL endpoint
- `GET /ws` - WebSocket endpoint

**Configuration**:
- HTTP Port: 8080 (configurable)
- GraphQL: Enabled
- WebSocket: Enabled
- CORS: Enabled

### 3. Nexus Collector (`nexus-collector`)
**Primary Role**: Data processing and storage

**Responsibilities**:
- **Message Queue Consumer**: Consumes telemetry data from etcd message queue
- **Data Processing**: Processes and validates telemetry records
- **Storage**: Persists data to Nexus/etcd backend
- **Host/GPU Registration**: Automatically registers hosts and GPUs

**Configuration**:
- Batch Size: 100 records (configurable)
- Poll Interval: 1 second (configurable)
- Buffer Size: 10,000 records (configurable)
- Workers: 8 goroutines (configurable)

## Message Queue Architecture

### etcd-based Message Queue
- **Queue Key Pattern**: `/telemetry/queue/{cluster-id}/`
- **Message Key Pattern**: `/telemetry/queue/{cluster-id}/{timestamp}_{hostname}_{gpu_id}_{sequence}`
- **Message Format**: JSON-serialized TelemetryRecord
- **Delivery**: At-least-once delivery with automatic cleanup
- **Ordering**: Messages processed in timestamp order

### Message Flow
1. **Streamer** publishes telemetry records to etcd queue
2. **Collector** polls etcd queue for new messages
3. **Collector** processes messages and deletes them from queue
4. **Collector** stores processed data in Nexus backend

## Production Deployment

### Kubernetes Architecture

#### Streamer Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: telemetry-pipeline-nexus-streamer
spec:
  type: ClusterIP
  ports:
    - port: 8081
      targetPort: 8081
      name: http
  selector:
    app.kubernetes.io/component: nexus-streamer
```

#### Gateway Service
```yaml
apiVersion: v1
kind: Service
metadata:
  name: telemetry-pipeline-nexus-gateway
spec:
  type: ClusterIP
  ports:
    - port: 8080
      targetPort: 8080
      name: http
  selector:
    app.kubernetes.io/component: nexus-gateway
```

### Scaling Considerations

#### Horizontal Scaling
- **Streamer**: Can be scaled horizontally for CSV upload throughput
- **Collector**: Can be scaled horizontally for processing throughput
- **Gateway**: Can be scaled horizontally for query throughput

#### Resource Requirements
- **Streamer**: CPU-intensive during CSV processing, memory for file buffering
- **Collector**: Memory-intensive for message buffering, CPU for processing
- **Gateway**: CPU-intensive for query processing, memory for caching

### High Availability

#### Component Redundancy
- Minimum 2 replicas per component
- Pod Disruption Budgets configured
- Health checks and readiness probes

#### Data Persistence
- etcd cluster with 3+ nodes
- Persistent volumes for etcd data
- Backup and recovery procedures

## API Changes

### Removed from Gateway
- `POST /api/v1/csv/upload` - **MOVED TO STREAMER**

### Added to Streamer
- `POST /api/v1/csv/upload` - CSV file upload and processing
- `GET /health` - Health check
- `GET /api/v1/status` - Service status

### Gateway API (Unchanged)
- All existing query endpoints remain the same
- GraphQL and WebSocket endpoints unchanged
- OpenAPI documentation updated to reflect changes

## Configuration Updates

### Helm Values (values-nexus.yaml)

#### Streamer Configuration
```yaml
nexusStreamer:
  enabled: true
  replicaCount: 2
  env:
    ENABLE_HTTP: "true"
    HTTP_PORT: "8081"
    UPLOAD_DIR: "/tmp/telemetry-uploads"
    MAX_UPLOAD_SIZE: "104857600"  # 100MB
    MAX_MEMORY: "33554432"        # 32MB
  service:
    enabled: true
    type: ClusterIP
    port: 8081
    targetPort: 8081
```

#### Gateway Configuration (Updated)
```yaml
nexusGateway:
  enabled: true
  replicaCount: 2
  env:
    PORT: "8080"
    ENABLE_GRAPHQL: "true"
    ENABLE_WEBSOCKET: "true"
    ENABLE_CORS: "true"
```

## Migration Guide

### For Existing Deployments

1. **Update Helm Charts**:
   ```bash
   helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline \
     -f ./deployments/helm/telemetry-pipeline/values-nexus.yaml
   ```

2. **Update Client Applications**:
   - Change CSV upload endpoint from Gateway to Streamer
   - Update endpoint URL from `gateway:8080/api/v1/csv/upload` to `streamer:8081/api/v1/csv/upload`

3. **Update Load Balancer/Ingress**:
   - Add route for Streamer service (port 8081)
   - Keep existing routes for Gateway service (port 8080)

### Testing the Migration

1. **Verify Streamer CSV Upload**:
   ```bash
   curl -X POST http://streamer:8081/api/v1/csv/upload \
     -F "file=@sample.csv"
   ```

2. **Verify Gateway Query APIs**:
   ```bash
   curl http://gateway:8080/api/v1/gpus
   ```

3. **Verify End-to-End Flow**:
   - Upload CSV via Streamer
   - Query data via Gateway
   - Verify data consistency

## Benefits of New Architecture

### Separation of Concerns
- **Streamer**: Focused on data ingestion and processing
- **Gateway**: Focused on data querying and API serving
- **Collector**: Focused on data persistence and storage

### Scalability
- Independent scaling of ingestion vs query workloads
- Better resource utilization
- Reduced bottlenecks

### Maintainability
- Clearer component boundaries
- Easier debugging and monitoring
- Simplified testing

### Production Readiness
- Better fault isolation
- Improved monitoring and observability
- Enhanced security boundaries

## Monitoring and Observability

### Metrics to Monitor
- **Streamer**: Upload rate, processing latency, error rate
- **Gateway**: Query rate, response time, cache hit rate
- **Collector**: Processing rate, queue depth, storage latency
- **Message Queue**: Queue depth, message age, throughput

### Health Checks
- All components provide `/health` endpoints
- Kubernetes readiness and liveness probes configured
- Service mesh health checks (if applicable)

### Logging
- Structured logging with correlation IDs
- Centralized log aggregation
- Error tracking and alerting

## Security Considerations

### Network Security
- Service-to-service communication within cluster
- Network policies for traffic isolation
- TLS encryption for external endpoints

### File Upload Security
- File size limits enforced
- File type validation
- Temporary file cleanup
- Resource limits to prevent DoS

### API Security
- Rate limiting on upload endpoints
- Authentication and authorization (if required)
- Input validation and sanitization

## Future Enhancements

### Potential Improvements
1. **Async Processing**: Queue CSV processing for large files
2. **Batch Upload**: Support multiple file uploads
3. **Streaming Upload**: Support streaming CSV data
4. **Data Validation**: Enhanced schema validation
5. **Compression**: Support compressed CSV files
6. **Encryption**: Support encrypted CSV files

### Monitoring Enhancements
1. **Distributed Tracing**: End-to-end request tracing
2. **Custom Metrics**: Business-specific metrics
3. **Alerting**: Proactive alerting on anomalies
4. **Dashboards**: Real-time operational dashboards
