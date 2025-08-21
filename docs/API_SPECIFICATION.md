# API Specification: GPU Telemetry REST API

## Table of Contents
1. [Overview](#overview)
2. [Authentication](#authentication)
3. [Common Response Formats](#common-response-formats)
4. [Error Handling](#error-handling)
5. [API Endpoints](#api-endpoints)
6. [Data Models](#data-models)
7. [OpenAPI Specification](#openapi-specification)

## Overview

The GPU Telemetry REST API provides access to GPU telemetry data collected from AI cluster nodes. The API follows RESTful principles and returns JSON responses.

**Base URL**: `https://api.gpu-telemetry.example.com/api/v1`

**API Version**: v1

**Content Type**: `application/json`

## Authentication

### Current Implementation
- No authentication required for initial implementation
- All endpoints are publicly accessible

### Future Enhancement
- API Key authentication via `X-API-Key` header
- JWT token-based authentication
- Role-based access control (RBAC)

## Common Response Formats

### Success Response
```json
{
  "status": "success",
  "data": {
    // Response data
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### Paginated Response
```json
{
  "status": "success",
  "data": {
    // Array of items
  },
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total_pages": 10,
    "total_items": 500,
    "has_next": true,
    "has_previous": false
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

## Error Handling

### Error Response Format
```json
{
  "status": "error",
  "error": {
    "code": "INVALID_GPU_ID",
    "message": "The specified GPU ID does not exist",
    "details": {
      "gpu_id": "invalid-gpu-123"
    }
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### HTTP Status Codes
- `200 OK`: Request successful
- `400 Bad Request`: Invalid request parameters
- `404 Not Found`: Resource not found
- `422 Unprocessable Entity`: Invalid data format
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error
- `503 Service Unavailable`: Service temporarily unavailable

### Error Codes
- `INVALID_GPU_ID`: GPU ID not found
- `INVALID_TIME_RANGE`: Invalid start_time or end_time
- `INVALID_PAGINATION`: Invalid page or page_size parameters
- `INVALID_METRIC_NAME`: Unsupported metric name
- `TIME_RANGE_TOO_LARGE`: Requested time range exceeds limits
- `RATE_LIMIT_EXCEEDED`: Too many requests
- `INTERNAL_ERROR`: Server-side error

## API Endpoints

### 1. Health Check

#### GET /health
Returns the health status of the API service.

**Parameters**: None

**Response**:
```json
{
  "status": "success",
  "data": {
    "service": "gpu-telemetry-api",
    "status": "healthy",
    "version": "1.0.0",
    "uptime": "2h 30m 15s",
    "dependencies": {
      "database": "healthy",
      "message_queue": "healthy",
      "cache": "healthy"
    }
  }
}
```

### 2. List All GPUs

#### GET /api/v1/gpus
Returns a paginated list of all GPUs for which telemetry data is available.

**Parameters**:
- `page` (query, optional): Page number (default: 1)
- `page_size` (query, optional): Items per page (default: 50, max: 1000)
- `hostname` (query, optional): Filter by hostname
- `model_name` (query, optional): Filter by GPU model name

**Example Request**:
```
GET /api/v1/gpus?page=1&page_size=10&hostname=mtv5-dgx1-hgpu-031
```

**Response**:
```json
{
  "status": "success",
  "data": {
    "gpus": [
      {
        "id": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        "device": "nvidia0",
        "model_name": "NVIDIA H100 80GB HBM3",
        "hostname": "mtv5-dgx1-hgpu-031",
        "uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
        "driver_version": "535.129.03",
        "first_seen": "2025-07-18T20:00:00Z",
        "last_seen": "2025-07-18T20:42:34Z",
        "status": "active",
        "metrics_available": [
          "DCGM_FI_DEV_GPU_UTIL",
          "DCGM_FI_DEV_MEM_COPY_UTIL",
          "DCGM_FI_DEV_GPU_TEMP"
        ]
      }
    ]
  },
  "pagination": {
    "page": 1,
    "page_size": 10,
    "total_pages": 7,
    "total_items": 64,
    "has_next": true,
    "has_previous": false
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### 3. Get GPU Details

#### GET /api/v1/gpus/{gpu_id}
Returns detailed information about a specific GPU.

**Parameters**:
- `gpu_id` (path, required): GPU UUID or ID

**Example Request**:
```
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50
```

**Response**:
```json
{
  "status": "success",
  "data": {
    "gpu": {
      "id": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "device": "nvidia0",
      "model_name": "NVIDIA H100 80GB HBM3",
      "hostname": "mtv5-dgx1-hgpu-031",
      "uuid": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
      "driver_version": "535.129.03",
      "first_seen": "2025-07-18T20:00:00Z",
      "last_seen": "2025-07-18T20:42:34Z",
      "status": "active",
      "metrics_available": [
        "DCGM_FI_DEV_GPU_UTIL",
        "DCGM_FI_DEV_MEM_COPY_UTIL",
        "DCGM_FI_DEV_GPU_TEMP"
      ],
      "statistics": {
        "total_datapoints": 15420,
        "avg_utilization": 85.2,
        "max_temperature": 78.5,
        "uptime_percentage": 99.8
      }
    }
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### 4. Query Telemetry by GPU

#### GET /api/v1/gpus/{gpu_id}/telemetry
Returns telemetry data for a specific GPU, ordered by timestamp.

**Parameters**:
- `gpu_id` (path, required): GPU UUID or ID
- `start_time` (query, optional): Start time in ISO 8601 format (inclusive)
- `end_time` (query, optional): End time in ISO 8601 format (inclusive)
- `metric_name` (query, optional): Filter by specific metric name
- `page` (query, optional): Page number (default: 1)
- `page_size` (query, optional): Items per page (default: 100, max: 1000)
- `order` (query, optional): Sort order - 'asc' or 'desc' (default: 'asc')

**Example Requests**:
```
# All telemetry for a GPU
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry

# Telemetry within time range
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-07-18T20:00:00Z&end_time=2025-07-18T21:00:00Z

# Specific metric within time range
GET /api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-07-18T20:00:00Z&end_time=2025-07-18T21:00:00Z&metric_name=DCGM_FI_DEV_GPU_UTIL
```

**Response**:
```json
{
  "status": "success",
  "data": {
    "gpu_id": "GPU-5fd4f087-86f3-7a43-b711-4771313afc50",
    "telemetry": [
      {
        "timestamp": "2025-07-18T20:42:34Z",
        "metric_name": "DCGM_FI_DEV_GPU_UTIL",
        "value": 85.0,
        "device": "nvidia0",
        "hostname": "mtv5-dgx1-hgpu-031",
        "labels": {
          "DCGM_FI_DRIVER_VERSION": "535.129.03",
          "instance": "mtv5-dgx1-hgpu-031:9400",
          "job": "dgx_dcgm_exporter"
        }
      },
      {
        "timestamp": "2025-07-18T20:42:35Z",
        "metric_name": "DCGM_FI_DEV_GPU_UTIL",
        "value": 87.5,
        "device": "nvidia0",
        "hostname": "mtv5-dgx1-hgpu-031",
        "labels": {
          "DCGM_FI_DRIVER_VERSION": "535.129.03",
          "instance": "mtv5-dgx1-hgpu-031:9400",
          "job": "dgx_dcgm_exporter"
        }
      }
    ]
  },
  "pagination": {
    "page": 1,
    "page_size": 100,
    "total_pages": 15,
    "total_items": 1420,
    "has_next": true,
    "has_previous": false
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789",
    "query_time_ms": 45
  }
}
```

### 5. Get Available Metrics

#### GET /api/v1/metrics
Returns a list of all available telemetry metrics.

**Parameters**:
- `gpu_id` (query, optional): Filter metrics available for specific GPU

**Response**:
```json
{
  "status": "success",
  "data": {
    "metrics": [
      {
        "name": "DCGM_FI_DEV_GPU_UTIL",
        "description": "GPU utilization percentage",
        "unit": "percent",
        "type": "gauge",
        "min_value": 0,
        "max_value": 100
      },
      {
        "name": "DCGM_FI_DEV_MEM_COPY_UTIL",
        "description": "Memory copy utilization percentage",
        "unit": "percent",
        "type": "gauge",
        "min_value": 0,
        "max_value": 100
      },
      {
        "name": "DCGM_FI_DEV_GPU_TEMP",
        "description": "GPU temperature",
        "unit": "celsius",
        "type": "gauge",
        "min_value": 0,
        "max_value": 100
      }
    ]
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### 6. System Metrics

#### GET /api/v1/system/metrics
Returns system-level metrics and statistics.

**Response**:
```json
{
  "status": "success",
  "data": {
    "system_metrics": {
      "total_gpus": 64,
      "active_gpus": 62,
      "inactive_gpus": 2,
      "total_hosts": 8,
      "total_datapoints": 1250000,
      "data_retention_days": 30,
      "api_version": "1.0.0",
      "last_updated": "2025-07-18T20:42:34Z"
    },
    "performance_metrics": {
      "avg_response_time_ms": 45,
      "requests_per_second": 150,
      "cache_hit_ratio": 0.85,
      "error_rate": 0.001
    }
  },
  "meta": {
    "timestamp": "2025-07-18T20:42:34Z",
    "request_id": "req_123456789"
  }
}
```

### 7. OpenAPI Specification

#### GET /api/v1/swagger.json
Returns the OpenAPI specification in JSON format.

#### GET /api/v1/swagger.yaml
Returns the OpenAPI specification in YAML format.

#### GET /api/v1/docs
Serves the Swagger UI documentation interface.

## Data Models

### GPU Model
```json
{
  "id": "string",
  "device": "string",
  "model_name": "string", 
  "hostname": "string",
  "uuid": "string",
  "driver_version": "string",
  "first_seen": "string (ISO 8601)",
  "last_seen": "string (ISO 8601)",
  "status": "string (enum: active, inactive, error)",
  "metrics_available": ["string"]
}
```

### Telemetry Data Point Model
```json
{
  "timestamp": "string (ISO 8601)",
  "metric_name": "string",
  "value": "number",
  "device": "string",
  "hostname": "string",
  "labels": {
    "key": "value"
  }
}
```

### Metric Definition Model
```json
{
  "name": "string",
  "description": "string",
  "unit": "string",
  "type": "string (enum: gauge, counter, histogram)",
  "min_value": "number (optional)",
  "max_value": "number (optional)"
}
```

### Pagination Model
```json
{
  "page": "integer",
  "page_size": "integer", 
  "total_pages": "integer",
  "total_items": "integer",
  "has_next": "boolean",
  "has_previous": "boolean"
}
```

### Error Model
```json
{
  "code": "string",
  "message": "string",
  "details": {
    "key": "value"
  }
}
```

## OpenAPI Specification

The complete OpenAPI 3.0 specification will be auto-generated from Go struct tags and Swagger annotations. Key features include:

### Generation Strategy:
- Use `swaggo/swag` for Go annotation-based generation
- Struct tags for request/response models
- Route comments for endpoint documentation
- Validation rules embedded in struct tags

### Example Go Annotations:
```go
// ListGPUs godoc
// @Summary List all GPUs
// @Description Get a paginated list of all GPUs with telemetry data
// @Tags gpus
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page" default(50)
// @Param hostname query string false "Filter by hostname"
// @Success 200 {object} GPUListResponse
// @Failure 400 {object} ErrorResponse
// @Router /api/v1/gpus [get]
func (h *Handler) ListGPUs(c *gin.Context) {
    // Implementation
}
```

### Makefile Commands:
```makefile
# Generate OpenAPI specification
generate-swagger:
	swag init -g cmd/api/main.go -o ./docs

# Serve documentation
serve-docs:
	swagger serve docs/swagger.json
```

### Documentation Features:
- Interactive Swagger UI
- Request/response examples
- Authentication documentation
- Error code reference
- Rate limiting information
- Versioning strategy

This API specification provides a comprehensive, RESTful interface for accessing GPU telemetry data with proper error handling, pagination, filtering, and documentation generation capabilities.
