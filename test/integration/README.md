# Telemetry Pipeline Integration Tests

This directory contains comprehensive end-to-end (E2E) integration tests for the telemetry pipeline project. These tests ensure that all components work together correctly and achieve the required code coverage of 80%+.

## Overview

The integration tests cover:

- **End-to-End Pipeline Testing**: Complete data flow from CSV upload through processing to API retrieval
- **Service Integration**: Testing nexus-streamer, nexus-collector, and nexus-gateway working together
- **etcd Integration**: Data persistence and retrieval from etcd
- **API Coverage**: All REST API endpoints, GraphQL, and WebSocket functionality
- **Error Handling**: Various error scenarios and edge cases
- **Performance**: Concurrent request handling and load testing
- **Code Coverage**: Ensuring 80%+ test coverage across the codebase

## Test Structure

### Files

- `e2e_test.go` - Main end-to-end test suite
- `coverage_test.go` - Code coverage verification tests
- `docker-compose.test.yml` - Docker setup for etcd test infrastructure
- `Makefile` - Test automation and utilities
- `README.md` - This documentation

### Test Components

1. **E2ETestSuite** - Main test suite that:
   - Starts etcd in Docker
   - Builds and starts all telemetry services
   - Runs comprehensive API tests
   - Verifies data persistence
   - Tests error handling and edge cases

2. **Coverage Tests** - Verify code coverage:
   - Overall project coverage (80%+ threshold)
   - Per-package coverage analysis
   - Component-specific coverage
   - Integration test coverage contribution

## Prerequisites

### Required Software

- **Go 1.23+** - For running tests
- **Docker & Docker Compose** - For etcd infrastructure
- **Make** - For test automation (optional but recommended)

### Verification

```bash
# Check if all prerequisites are available
make check-deps
```

## Running Tests

### Quick Start

```bash
# Run all integration tests with coverage
make test-all

# Run only E2E tests
make test-e2e

# Run only coverage tests
make test-coverage
```

### Manual Test Execution

```bash
# Set up test environment
make setup-test-env

# Run specific test suite
go test -v -tags=integration -timeout=10m ./test/integration -run TestE2ETestSuite

# Clean up
make cleanup-test-env
```

### Individual Test Cases

```bash
# Run specific test
make test-specific TEST=TestHealthEndpoints

# Run with race detection
make test-race

# Run performance tests
make test-performance
```

## Test Scenarios

### 1. Health Check Tests
- Verifies all services are running and healthy
- Tests service status endpoints
- Validates etcd connectivity

### 2. CSV Upload and Processing
- Uploads test CSV file to streamer
- Verifies data processing pipeline
- Checks data persistence in etcd
- Validates collector processing

### 3. API Endpoint Testing
- **GET /api/v1/gpus** - List all GPUs
- **GET /api/v1/gpus/{id}/telemetry** - GPU telemetry data
- **GET /api/v1/hosts** - List all hosts
- **GET /api/v1/clusters** - List clusters
- **GET /api/v1/clusters/{id}** - Specific cluster info
- **POST /graphql** - GraphQL queries
- **GET /ws** - WebSocket endpoint
- **GET /swagger/** - API documentation

### 4. Data Persistence Tests
- Verifies cluster data in etcd
- Checks host registration
- Validates GPU registration
- Confirms telemetry data storage

### 5. Error Handling Tests
- Non-existent resource requests
- Invalid GraphQL queries
- Malformed requests
- Service unavailability scenarios

### 6. Performance Tests
- Concurrent request handling
- Load testing with multiple requests
- Response time validation
- Resource usage monitoring

## Coverage Requirements

The tests enforce the following coverage thresholds:

- **Overall Project**: 80% minimum
- **Critical Packages**: 70% minimum
  - `pkg/config`
  - `pkg/messagequeue`
  - `internal/nexus`
- **Standard Packages**: 50% minimum
- **Integration Tests**: 30% contribution minimum

### Coverage Reports

Coverage reports are generated in multiple formats:

```bash
# Generate HTML coverage report
make test-all
# Opens: integration_coverage.html

# View coverage by component
go test -tags=integration ./test/integration -run TestCoverageByComponent
```

## Test Data

### Sample CSV Format

The tests use realistic telemetry data:

```csv
timestamp,gpu_id,uuid,device,modelName,Hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:00:00Z,0,GPU-12345-67890-ABCDE,nvidia0,NVIDIA H100 80GB HBM3,test-host-1,85.5,60.2,48000,32000,72.0,350.5,1410,1215
```

### Test Environment

- **etcd**: Single node cluster for data storage
- **Ports**: 
  - etcd: 12379, 12380
  - streamer: 8081
  - gateway: 8080
- **Cluster ID**: `test-cluster`

## Troubleshooting

### Common Issues

1. **Docker not available**
   ```bash
   # Install Docker and Docker Compose
   # Verify with: docker version && docker-compose version
   ```

2. **Port conflicts**
   ```bash
   # Check if ports are in use
   lsof -i :12379 -i :8080 -i :8081
   # Stop conflicting services
   ```

3. **etcd startup timeout**
   ```bash
   # Check Docker logs
   docker-compose -f docker-compose.test.yml logs etcd
   # Increase timeout in Makefile if needed
   ```

4. **Test failures**
   ```bash
   # Run with verbose output
   go test -v -tags=integration ./test/integration
   # Check service logs in test output
   ```

### Debug Mode

```bash
# Get detailed environment information
make debug

# Check test status
make status

# Manual cleanup if needed
make cleanup-test-env
```

### Service Logs

During tests, service logs are displayed in the console. Key things to look for:

- **Streamer**: CSV processing messages, upload confirmations
- **Collector**: Message consumption, data processing
- **Gateway**: API request handling, etcd queries
- **etcd**: Connection status, data operations

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests
on: [push, pull_request]
jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.23'
      - name: Run Integration Tests
        run: make test-ci
      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./integration_coverage.out
```

### Local Development

```bash
# Quick development cycle
make test-quick

# Full validation before commit
make test-all

# Continuous testing during development
watch -n 30 'make test-quick'
```

## Performance Benchmarks

The tests include performance validation:

- **API Response Time**: < 100ms for simple queries
- **CSV Processing**: > 1000 records/second
- **Concurrent Requests**: 10+ simultaneous requests
- **Memory Usage**: Reasonable resource consumption

## Contributing

When adding new tests:

1. Follow the existing test structure
2. Add appropriate coverage verification
3. Update this README if needed
4. Ensure tests are deterministic and reliable
5. Include both positive and negative test cases

### Test Naming Convention

- `TestE2ETestSuite` - Main end-to-end test suite
- `TestCoverage*` - Coverage-related tests
- `Test*Endpoints` - API endpoint tests
- `Test*Integration` - Component integration tests

## Monitoring and Metrics

The tests validate that the system properly:

- Registers services in etcd
- Maintains service health status
- Processes telemetry data accurately
- Handles concurrent operations
- Provides comprehensive API coverage
- Maintains data consistency

## Security Considerations

The integration tests run in an isolated environment:

- Uses test-specific etcd instance
- Temporary data directories
- No external network dependencies
- Clean environment setup/teardown
