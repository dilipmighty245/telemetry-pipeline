# API Specification

## Overview

The Telemetry Pipeline API provides REST endpoints for querying GPU telemetry data from AI clusters. The API follows RESTful principles and includes auto-generated OpenAPI documentation.

## Base URL

- **Development**: `http://localhost:8080`
- **Production**: `https://your-domain.com`

## Required API Endpoints (Per Project Specification)

### 1. List All GPUs

**Endpoint**: `GET /api/v1/gpus`

**Description**: Return a list of all GPUs for which telemetry data is available.

**Response**:
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

### 2. Query Telemetry by GPU

**Endpoint**: `GET /api/v1/gpus/{id}/telemetry`

**Description**: Return all telemetry entries for a specific GPU, ordered by time.

**Parameters**:
- `id` (path): GPU ID or UUID
- `start_time` (query, optional): Start time filter (RFC3339 format)
- `end_time` (query, optional): End time filter (RFC3339 format)
- `limit` (query, optional): Maximum number of results (default: 100)

**Examples**:
```bash
GET /api/v1/gpus/GPU-12345/telemetry
GET /api/v1/gpus/GPU-12345/telemetry?start_time=2024-01-01T00:00:00Z&end_time=2024-01-02T00:00:00Z
```

**Response**:
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
      "memory_used_mb": 45000,
      "memory_free_mb": 35000,
      "temperature": 65.0,
      "power_draw": 350.5,
      "sm_clock_mhz": 1410,
      "memory_clock_mhz": 1215
    }
  ],
  "count": 1
}
```

## Additional Endpoints

### Health Check
**Endpoint**: `GET /health`

**Response**:
```json
{
  "status": "healthy",
  "cluster_id": "default-cluster",
  "timestamp": "2024-01-15T10:30:00Z"
}
```

### List All Hosts
**Endpoint**: `GET /api/v1/hosts`

**Response**:
```json
{
  "success": true,
  "data": ["gpu-node-01", "gpu-node-02"],
  "count": 2
}
```

### Query General Telemetry
**Endpoint**: `GET /api/v1/telemetry`

**Parameters**:
- `host_id` (query): Filter by hostname
- `gpu_id` (query): Filter by GPU ID
- `start_time` (query): Start time filter
- `end_time` (query): End time filter
- `limit` (query): Maximum results

## Auto-Generated OpenAPI Documentation

The OpenAPI specification is automatically generated and available at:
- **Swagger UI**: `/swagger/`
- **OpenAPI JSON**: `/swagger/spec.json`

Generate updated specification:
```bash
make generate-openapi
```

## Error Responses

All endpoints return consistent error responses:

```json
{
  "success": false,
  "error": "Error description",
  "code": "ERROR_CODE"
}
```

**HTTP Status Codes**:
- `200`: Success
- `400`: Bad Request
- `404`: Not Found
- `500`: Internal Server Error

## Rate Limiting

API endpoints are rate-limited to prevent abuse:
- **Default**: 100 requests per minute per IP
- **Burst**: Up to 10 requests per second

## Authentication

Currently, the API does not require authentication for development. Production deployments should implement:
- API key authentication
- JWT token validation
- Rate limiting per user/application