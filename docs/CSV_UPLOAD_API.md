# CSV Upload API Documentation

## Overview

The Nexus Telemetry Pipeline now includes production-grade CSV file upload functionality, allowing users to upload, validate, process, and manage CSV files containing telemetry data through a comprehensive REST API.

## Features

### 🔒 Security & Validation
- **File Size Limits**: Maximum 100MB per file
- **File Type Validation**: Only `.csv` files accepted
- **Content Validation**: Validates required headers (`timestamp`, `gpu_id`, `hostname`)
- **Filename Sanitization**: Removes dangerous characters and path traversal attempts
- **MD5 Hash Verification**: Generates and stores file integrity hashes

### 📊 File Management
- **Secure Storage**: Files stored in dedicated upload directory with unique IDs
- **Metadata Tracking**: Comprehensive file information stored in etcd
- **Status Tracking**: Real-time processing status (uploaded → processing → processed/error)
- **Batch Processing**: Efficient processing in configurable batches

### 🔍 Monitoring & Logging
- **Comprehensive Logging**: All operations logged with structured logging
- **Processing Metrics**: Track upload time, processing duration, record counts
- **Error Handling**: Detailed error messages and status codes
- **Audit Trail**: Complete history of file operations

## API Endpoints

### 1. Upload CSV File
```http
POST /api/v1/csv/upload
Content-Type: multipart/form-data
```

**Parameters:**
- `file` (required): CSV file to upload
- `description` (optional): File description
- `tags` (optional): Comma-separated tags

**Example:**
```bash
curl -X POST \
  -F "file=@telemetry_data.csv" \
  -F "description=GPU telemetry data from cluster A" \
  -F "tags=production,cluster-a,gpu" \
  http://localhost:8080/api/v1/csv/upload
```

**Response:**
```json
{
  "success": true,
  "file_id": "a1b2c3d4e5f6g7h8",
  "filename": "a1b2c3d4e5f6g7h8_telemetry_data.csv",
  "size": 1048576,
  "md5_hash": "d41d8cd98f00b204e9800998ecf8427e",
  "record_count": 1000,
  "headers": ["timestamp", "gpu_id", "hostname", "gpu_utilization", "..."],
  "uploaded_at": "2025-01-18T10:00:00Z",
  "status": "uploaded",
  "metadata": {
    "original_filename": "telemetry_data.csv",
    "content_type": "text/csv",
    "upload_ip": "192.168.1.100",
    "user_agent": "curl/7.68.0"
  }
}
```

### 2. List Uploaded Files
```http
GET /api/v1/csv/files?status=uploaded&limit=50
```

**Query Parameters:**
- `status` (optional): Filter by status (`uploaded`, `processing`, `processed`, `error`)
- `limit` (optional): Maximum number of results (default: 50, max: 1000)

**Example:**
```bash
curl http://localhost:8080/api/v1/csv/files?status=processed&limit=10
```

### 3. Get File Information
```http
GET /api/v1/csv/files/{file_id}
```

**Example:**
```bash
curl http://localhost:8080/api/v1/csv/files/a1b2c3d4e5f6g7h8
```

### 4. Process CSV File
```http
POST /api/v1/csv/files/{file_id}/process
```

Starts asynchronous processing of the uploaded CSV file, streaming data into the telemetry pipeline.

**Example:**
```bash
curl -X POST http://localhost:8080/api/v1/csv/files/a1b2c3d4e5f6g7h8/process
```

### 5. Check Processing Status
```http
GET /api/v1/csv/files/{file_id}/status
```

**Example:**
```bash
curl http://localhost:8080/api/v1/csv/files/a1b2c3d4e5f6g7h8/status
```

**Response:**
```json
{
  "success": true,
  "file_id": "a1b2c3d4e5f6g7h8",
  "status": "processed",
  "uploaded_at": "2025-01-18T10:00:00Z",
  "processed_at": "2025-01-18T10:02:30Z"
}
```

### 6. Download CSV File
```http
GET /api/v1/csv/files/{file_id}/download
```

**Example:**
```bash
curl -O http://localhost:8080/api/v1/csv/files/a1b2c3d4e5f6g7h8/download
```

### 7. Delete CSV File
```http
DELETE /api/v1/csv/files/{file_id}
```

**Example:**
```bash
curl -X DELETE http://localhost:8080/api/v1/csv/files/a1b2c3d4e5f6g7h8
```

## CSV File Format Requirements

### Required Headers
The CSV file must contain these headers (case-insensitive):
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

1. **Upload**: File is uploaded and validated
2. **Storage**: File is securely stored with unique ID
3. **Validation**: CSV structure and required headers are validated
4. **Metadata**: File information is stored in etcd
5. **Processing**: When triggered, file is processed in batches
6. **Streaming**: Records are streamed to the telemetry pipeline
7. **Completion**: Status is updated to `processed` or `error`

## Configuration

### Environment Variables
- `UPLOAD_DIR`: Directory for uploaded files (default: `./uploads`)
- `MAX_UPLOAD_SIZE`: Maximum file size in bytes (default: 100MB)
- `MAX_MEMORY`: Maximum memory for multipart parsing (default: 32MB)

### File Limits
- **Maximum file size**: 100MB
- **Maximum memory usage**: 32MB for multipart parsing
- **Allowed extensions**: `.csv` only
- **Batch size**: 100 records per batch during processing

## Error Handling

### Common Error Codes
- `400 Bad Request`: Invalid file format, missing required headers, or malformed request
- `413 Payload Too Large`: File exceeds maximum size limit
- `404 Not Found`: File ID not found
- `409 Conflict`: File is already being processed
- `500 Internal Server Error`: Server-side processing error

### Error Response Format
```json
{
  "success": false,
  "error": "Invalid CSV file: missing required headers: [timestamp, gpu_id]"
}
```

## Demo Script

A comprehensive demo script is available to test all functionality:

```bash
# Run full demonstration
./scripts/demo-csv-upload.sh demo

# Show API documentation
./scripts/demo-csv-upload.sh docs

# Clean up demo files
./scripts/demo-csv-upload.sh cleanup
```

## Integration with Telemetry Pipeline

The CSV upload functionality integrates seamlessly with the existing telemetry pipeline:

1. **Data Validation**: Same validation rules as the streamer
2. **Message Queue**: Processed records are sent to the same etcd message queue
3. **Format Compatibility**: Output format matches streamer output
4. **Monitoring**: Same logging and monitoring infrastructure

## Security Considerations

### File Security
- Files are stored outside the web root
- Unique file IDs prevent direct access
- Filename sanitization prevents path traversal
- File type validation prevents malicious uploads

### Access Control
- All operations are logged with IP and user agent
- File metadata includes upload source information
- Consider adding authentication for production use

### Data Privacy
- Files are stored locally (not in cloud storage)
- File deletion removes both file and metadata
- Processing is done in-memory with configurable batch sizes

## Performance Considerations

### Upload Performance
- Streaming upload processing (no full file buffering)
- Configurable memory limits for multipart parsing
- MD5 hash calculation during upload (single pass)

### Processing Performance
- Batch processing (default: 100 records per batch)
- Configurable delays between batches
- Asynchronous processing (non-blocking API)

### Storage Efficiency
- Unique file IDs prevent duplicate storage
- Metadata stored in etcd (distributed and persistent)
- Automatic cleanup options available

## Monitoring and Observability

### Logging
All operations are logged with structured logging:
```
INFO[2025-01-18T10:00:00Z] CSV file uploaded successfully: telemetry_data.csv (ID: a1b2c3d4e5f6g7h8, Size: 1048576 bytes, Records: 1000)
INFO[2025-01-18T10:02:30Z] CSV file processing completed: telemetry_data.csv (ID: a1b2c3d4e5f6g7h8, Records: 1000, Duration: 2m30s)
```

### Metrics
- Upload success/failure rates
- Processing duration
- File sizes and record counts
- Error rates by type

### Health Checks
The `/health` endpoint includes CSV upload system status.

## Production Deployment

### Recommended Configuration
```yaml
# Environment variables for production
UPLOAD_DIR: "/var/lib/telemetry-pipeline/uploads"
MAX_UPLOAD_SIZE: "104857600"  # 100MB
MAX_MEMORY: "33554432"        # 32MB
LOG_LEVEL: "info"
```

### Storage Requirements
- Ensure adequate disk space for uploaded files
- Consider file retention policies
- Implement backup strategies for critical data

### Monitoring
- Monitor disk usage in upload directory
- Set up alerts for processing failures
- Track upload and processing metrics

## API Documentation

Complete API documentation is available via Swagger UI:
- **Development**: http://localhost:8080/swagger/
- **Interactive API Explorer**: Test endpoints directly in the browser
- **OpenAPI Specification**: Complete schema definitions

## Troubleshooting

### Common Issues

1. **File Upload Fails**
   - Check file size (max 100MB)
   - Verify file extension is `.csv`
   - Ensure required headers are present

2. **Processing Stuck**
   - Check etcd connectivity
   - Verify message queue is functioning
   - Check disk space in upload directory

3. **File Not Found**
   - Verify file ID is correct
   - Check if file was deleted
   - Ensure etcd contains file metadata

### Debug Commands
```bash
# Check upload directory
ls -la ./uploads/

# Check etcd for file metadata
etcdctl get --prefix "/csv/files/"

# Check gateway logs
tail -f /var/log/nexus-gateway.log
```

## Future Enhancements

### Planned Features
- **Authentication**: JWT-based authentication
- **Rate Limiting**: Per-user upload limits
- **File Compression**: Support for gzipped CSV files
- **Batch Upload**: Multiple file upload support
- **Webhook Notifications**: Processing completion callbacks
- **File Versioning**: Support for file updates
- **Data Transformation**: Custom field mapping
- **Scheduled Processing**: Cron-based processing triggers

### API Versioning
The current API is version 1 (`/api/v1/`). Future versions will maintain backward compatibility while adding new features.
