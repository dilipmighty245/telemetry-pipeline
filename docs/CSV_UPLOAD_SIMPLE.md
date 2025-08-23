# Streamlined CSV Upload API

## Overview

The Nexus Telemetry Pipeline provides a simple, memory-efficient CSV upload API that processes telemetry data immediately without storing files on disk.

## Key Features

✅ **Memory Efficient**: No file storage - processes data directly from memory  
✅ **Single Endpoint**: One API call uploads and processes the CSV  
✅ **Real-time Processing**: Data is immediately streamed to the telemetry pipeline  
✅ **Validation**: Validates CSV format and required headers  
✅ **Processing Metrics**: Returns detailed processing statistics  

## API Endpoint

### Upload and Process CSV File

```http
POST /api/v1/csv/upload
Content-Type: multipart/form-data
```

**Parameters:**
- `file` (required): CSV file to upload and process

**Example Request:**
```bash
curl -X POST \
  -F "file=@telemetry_data.csv" \
  http://localhost:8080/api/v1/csv/upload
```

**Example Response:**
```json
{
  "success": true,
  "filename": "telemetry_data.csv",
  "size": 1048576,
  "total_records": 1000,
  "processed_records": 995,
  "skipped_records": 5,
  "processing_time": "2.5s",
  "records_per_second": 398.0,
  "status": "completed",
  "message": "CSV file processed and streamed successfully"
}
```

## CSV File Requirements

### Required Headers (case-insensitive)
- `timestamp`: ISO 8601 format (e.g., `2025-01-18T10:00:00Z`)
- `gpu_id`: GPU identifier (e.g., `0`, `1`, `2`)
- `hostname`: Host machine name

### Optional Headers
- `uuid`: GPU UUID
- `device`: Device name (e.g., `nvidia0`)
- `modelName`: GPU model name
- `gpu_utilization`: GPU utilization percentage
- `memory_utilization`: Memory utilization percentage
- `memory_used_mb`: Memory used in MB
- `memory_free_mb`: Memory free in MB
- `temperature`: GPU temperature in Celsius
- `power_draw`: Power consumption in watts
- `sm_clock_mhz`: SM clock frequency in MHz
- `memory_clock_mhz`: Memory clock frequency in MHz

### Example CSV Format
```csv
timestamp,gpu_id,hostname,uuid,device,modelName,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2025-01-18T10:00:00Z,0,gpu-node-01,GPU-12345678-1234-1234-1234-123456789abc,nvidia0,NVIDIA H100 80GB HBM3,85.5,72.3,58000,22000,65.2,350.8,1410,1215
2025-01-18T10:00:01Z,1,gpu-node-01,GPU-87654321-4321-4321-4321-cba987654321,nvidia1,NVIDIA H100 80GB HBM3,92.1,68.7,55000,25000,67.8,380.2,1420,1220
```

## Processing Workflow

1. **Upload**: CSV file is uploaded via multipart form
2. **Validation**: File size, extension, and CSV structure are validated
3. **Header Check**: Required headers are verified
4. **Stream Processing**: Data is processed in batches directly from memory
5. **Telemetry Streaming**: Records are streamed to the telemetry pipeline
6. **Response**: Processing statistics are returned immediately
7. **Cleanup**: No files are stored - memory is freed

## Configuration

### Limits
- **Maximum file size**: 100MB
- **Maximum memory usage**: 32MB for multipart parsing
- **Allowed extensions**: `.csv` only
- **Batch size**: 100 records per batch during processing

## Error Handling

### Common Errors
- `400 Bad Request`: Invalid file format, missing required headers
- `413 Payload Too Large`: File exceeds 100MB limit
- `500 Internal Server Error`: Processing error

### Error Response Format
```json
{
  "success": false,
  "error": "Invalid CSV file: missing required headers: [timestamp, gpu_id]"
}
```

## Demo Script

Test the API with the included demo script:

```bash
# Run demonstration
./scripts/demo-csv-upload.sh demo

# Show API documentation
./scripts/demo-csv-upload.sh docs

# Clean up demo files
./scripts/demo-csv-upload.sh cleanup
```

## Benefits of Streamlined Approach

### Memory Efficiency
- **No File Storage**: Files are not saved to disk after processing
- **Stream Processing**: Data flows directly from upload to telemetry pipeline
- **Immediate Cleanup**: Memory is freed as soon as processing completes

### Simplicity
- **Single Endpoint**: One API call handles upload and processing
- **No State Management**: No need to track file status or manage file lifecycle
- **Immediate Results**: Processing statistics returned in the same request

### Performance
- **Reduced I/O**: No disk writes for temporary file storage
- **Lower Latency**: No separate processing step required
- **Batch Processing**: Efficient processing in configurable batches

### Production Ready
- **Proper Validation**: Comprehensive input validation
- **Error Handling**: Detailed error messages and appropriate HTTP status codes
- **Logging**: Complete audit trail of processing operations
- **Monitoring**: Processing metrics and performance statistics

## Integration

The streamlined CSV upload integrates seamlessly with the existing telemetry pipeline:

- **Same Message Queue**: Uses the same etcd message queue as the streamer
- **Same Data Format**: Output format matches existing telemetry data structure
- **Same Validation**: Uses the same validation rules as other components
- **Same Monitoring**: Integrated with existing logging and monitoring infrastructure

## API Documentation

Complete API documentation is available via Swagger UI:
- **Interactive Documentation**: http://localhost:8080/swagger/
- **Test Endpoints**: Try the API directly in your browser
- **Schema Definitions**: Complete request/response schemas
