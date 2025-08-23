# Process-Based E2E Integration Testing

This document describes the process-based end-to-end (E2E) integration testing approach for the telemetry pipeline. This approach runs the streamer, collector, and API gateway as processes within the test suite, providing better control and faster execution compared to external binary-based testing.

## Overview

The process-based E2E testing approach:

1. **Runs services as processes**: Instead of starting external binaries, services run as goroutines within the test process
2. **Uses actual service implementations**: Calls the real service logic from the main packages
3. **Provides better control**: Can easily start/stop services and inspect their state
4. **Faster execution**: No need to build and start external processes
5. **Better debugging**: All logs and errors are captured within the test context

## Architecture

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Test Suite    │    │      etcd       │    │   Test Data     │
│                 │    │   (Docker)      │    │     (CSV)       │
└─────────────────┘    └─────────────────┘    └─────────────────┘
         │                       │                       │
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│    Streamer     │    │    Collector    │    │    Gateway      │
│   (Process)     │    │   (Process)     │    │   (Process)     │
│                 │    │                 │    │                 │
│ HTTP :8091      │    │ etcd Consumer   │    │ HTTP :8090      │
│ CSV Upload      │────▶ Message Proc   │────▶ REST API       │
│ etcd Producer   │    │ Nexus Storage   │    │ GraphQL         │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## Components

### 1. ProcessE2ETestSuite
The main test suite that orchestrates the entire E2E testing process:
- Sets up etcd infrastructure using Docker Compose
- Creates test data (CSV files)
- Starts services as processes
- Runs comprehensive tests
- Cleans up resources

### 2. ServiceWrapper
Provides utilities for running services with proper configuration parsing and lifecycle management:
- `RunStreamer()`: Runs the streamer service
- `RunCollector()`: Runs the collector service  
- `RunGateway()`: Runs the gateway service

### 3. Test Services
Simplified implementations of the main services optimized for testing:
- `TestStreamer`: HTTP server with CSV upload and etcd publishing
- `TestCollector`: etcd consumer with Nexus integration
- `TestGateway`: REST API server with etcd queries

## Test Scenarios

### 1. Health Endpoints
Tests that all services start correctly and respond to health checks:
```go
func (suite *ProcessE2ETestSuite) TestProcessHealthEndpoints()
```

### 2. CSV Upload and Processing
Tests the complete E2E flow:
1. Upload CSV file to streamer
2. Streamer processes and publishes to etcd
3. Collector consumes and stores in Nexus
4. Gateway serves data via REST API

```go
func (suite *ProcessE2ETestSuite) TestProcessCSVUploadAndProcessing()
```

### 3. API Endpoints
Tests all Gateway REST API endpoints:
- `/api/v1/gpus` - List all GPUs
- `/api/v1/hosts` - List all hosts
- `/api/v1/clusters` - List clusters
- `/api/v1/gpus/{id}/telemetry` - Get GPU telemetry

```go
func (suite *ProcessE2ETestSuite) TestProcessGatewayAPIEndpoints()
```

### 4. Data Persistence
Tests that data flows through the entire pipeline and is properly stored in etcd:
```go
func (suite *ProcessE2ETestSuite) TestProcessDataPersistence()
```

## Configuration

### Service Ports
- **Streamer**: 8091 (HTTP server for CSV upload)
- **Gateway**: 8090 (REST API server)
- **etcd**: 12379 (Docker container)

### Test Data
- **Cluster ID**: `process-test-cluster`
- **Test Hosts**: `test-host-1`, `test-host-2`
- **Test GPUs**: Various NVIDIA H100 GPUs with different UUIDs
- **CSV Records**: 10 sample telemetry records

## Running Tests

### Prerequisites
1. **Docker**: Required for etcd container
2. **Docker Compose**: Required for infrastructure setup
3. **Go 1.19+**: Required for running tests

### Basic Usage

```bash
# Run all process-based E2E tests
make -f Makefile.process test-process

# Run with verbose output
make -f Makefile.process test-process-verbose

# Run specific test
make -f Makefile.process test-specific TEST=TestProcessHealthEndpoints

# Set up test environment
make -f Makefile.process setup-test-env

# Clean up test data
make -f Makefile.process clean-test-data
```

### Direct Go Commands

```bash
# Run all process E2E tests
go test -tags=integration -run TestProcessE2ETestSuite ./test/integration/

# Run with verbose output
go test -tags=integration -v -run TestProcessE2ETestSuite ./test/integration/

# Run specific test method
go test -tags=integration -run TestProcessE2ETestSuite/TestProcessHealthEndpoints ./test/integration/
```

## Test Data Structure

### CSV Format
```csv
timestamp,gpu_id,uuid,device,modelName,Hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:00:00Z,0,GPU-12345-67890-ABCDE,nvidia0,NVIDIA H100 80GB HBM3,test-host-1,85.5,60.2,48000,32000,72.0,350.5,1410,1215
```

### Expected Data Flow
1. **CSV Upload** → Streamer HTTP endpoint
2. **CSV Processing** → Parse and validate records
3. **Message Publishing** → Store in etcd queue
4. **Message Consumption** → Collector reads from etcd
5. **Data Storage** → Store in Nexus/etcd
6. **API Queries** → Gateway serves data

## Debugging

### Logs
All service logs are captured and displayed during test execution. Use verbose mode to see detailed logs:
```bash
go test -tags=integration -v -run TestProcessE2ETestSuite ./test/integration/
```

### etcd Inspection
You can inspect etcd data during or after tests:
```bash
# Start etcd manually
make -f Makefile.process start-etcd

# Connect to etcd
docker exec -it telemetry-etcd-test etcdctl get --prefix "/telemetry"

# Stop etcd
make -f Makefile.process stop-etcd
```

### Service Endpoints
During test execution, you can manually test service endpoints:
- Streamer health: `http://localhost:8091/health`
- Gateway health: `http://localhost:8090/health`
- Gateway GPUs: `http://localhost:8090/api/v1/gpus`

## Advantages

### 1. Speed
- No need to build external binaries
- Faster startup and shutdown
- Parallel test execution

### 2. Control
- Direct access to service state
- Easy configuration changes
- Better error handling

### 3. Debugging
- All logs in one place
- Easy to set breakpoints
- Better stack traces

### 4. Reliability
- No external process management
- Consistent test environment
- Better resource cleanup

## Limitations

### 1. Test Isolation
- Services run in the same process
- Shared memory space
- Potential for interference

### 2. Real-world Differences
- Not testing actual binary deployment
- Different resource constraints
- Network behavior may differ

### 3. Complexity
- More complex test setup
- Requires service refactoring
- Harder to maintain

## Best Practices

### 1. Test Organization
- Use descriptive test names
- Group related tests together
- Clean up resources properly

### 2. Data Management
- Use unique test data for each test
- Clean up test data after tests
- Use temporary directories

### 3. Service Configuration
- Use different ports for each service
- Use unique cluster/service IDs
- Configure appropriate timeouts

### 4. Error Handling
- Check all service health endpoints
- Validate data flow at each step
- Provide meaningful error messages

## Future Enhancements

1. **Parallel Test Execution**: Run multiple test suites in parallel
2. **Performance Testing**: Add load testing capabilities
3. **Chaos Testing**: Introduce failures and test resilience
4. **Monitoring Integration**: Add metrics collection during tests
5. **CI/CD Integration**: Optimize for continuous integration pipelines

## Troubleshooting

### Common Issues

1. **Port Conflicts**: Ensure ports 8090, 8091, and 12379 are available
2. **Docker Issues**: Ensure Docker daemon is running
3. **etcd Connection**: Check etcd container health
4. **Test Timeouts**: Increase timeout values for slow systems
5. **Resource Cleanup**: Run cleanup commands between test runs

### Getting Help

1. Check test logs for detailed error messages
2. Inspect etcd data for debugging data flow issues
3. Use verbose test output for detailed execution traces
4. Check service health endpoints manually
5. Review Docker container logs for infrastructure issues
