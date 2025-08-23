# Quick Start Guide - Telemetry Pipeline E2E Tests

This guide provides quick instructions to run the comprehensive end-to-end tests for the telemetry-pipeline project with 80%+ code coverage.

## 🚀 Quick Start (TL;DR)

```bash
# Navigate to the integration test directory
cd test/integration

# Run all tests with coverage (automated setup and cleanup)
./run_tests.sh

# Or using Make
make test-all
```

That's it! The script will:
- ✅ Check prerequisites (Docker, Go, etc.)
- ✅ Start etcd in Docker
- ✅ Build all services (streamer, collector, gateway)
- ✅ Run comprehensive E2E tests
- ✅ Verify 80%+ code coverage
- ✅ Generate HTML coverage reports
- ✅ Clean up automatically

## 📋 Prerequisites

- **Go 1.23+**
- **Docker & Docker Compose**
- **Make** (optional, for Makefile commands)

## 🧪 Test Coverage

The tests achieve **80%+ code coverage** by testing:

### Core Components
- ✅ **nexus-streamer** - CSV upload and processing
- ✅ **nexus-collector** - Data collection from message queue
- ✅ **nexus-gateway** - REST API, GraphQL, WebSocket endpoints
- ✅ **etcd integration** - Data persistence and retrieval
- ✅ **Service discovery** - Service registration and health checks
- ✅ **Message queue** - etcd-based message passing
- ✅ **Configuration management** - Dynamic config updates
- ✅ **Scaling coordination** - Auto-scaling logic

### API Endpoints Tested
- `GET /health` - Service health checks
- `POST /api/v1/csv/upload` - CSV file upload
- `GET /api/v1/gpus` - List all GPUs
- `GET /api/v1/gpus/{id}/telemetry` - GPU telemetry data
- `GET /api/v1/hosts` - List all hosts
- `GET /api/v1/clusters` - List clusters
- `GET /api/v1/clusters/{id}` - Cluster details
- `POST /graphql` - GraphQL queries
- `GET /ws` - WebSocket endpoint
- `GET /swagger/` - API documentation

### Test Scenarios
- ✅ **Happy path** - Normal operation flow
- ✅ **Error handling** - Invalid requests, missing resources
- ✅ **Concurrent requests** - Load testing with multiple clients
- ✅ **Data persistence** - etcd storage verification
- ✅ **Service integration** - End-to-end data flow
- ✅ **Performance** - Response time and throughput validation

## 🛠️ Available Commands

### Using the Test Runner Script

```bash
# Check prerequisites
./run_tests.sh check

# Set up test environment only
./run_tests.sh setup

# Run integration tests only
./run_tests.sh integration

# Run E2E tests only
./run_tests.sh e2e

# Run coverage verification
./run_tests.sh coverage

# Clean up everything
./run_tests.sh clean

# Show help
./run_tests.sh help
```

### Using Makefile

```bash
# Run all tests with coverage
make test-all

# Run E2E tests only
make test-e2e

# Run coverage tests only
make test-coverage

# Set up test environment
make setup-test-env

# Clean up test environment
make cleanup-test-env

# Build services
make build-services

# Check dependencies
make check-deps
```

### Using Go Test Directly

```bash
# Run all integration tests
go test -v -tags=integration -timeout=10m ./test/integration

# Run specific test suite
go test -v -tags=integration -run TestE2ETestSuite ./test/integration

# Run with coverage
go test -v -tags=integration -coverprofile=coverage.out -coverpkg=./... ./test/integration
```

## 📊 Coverage Reports

After running tests, you'll get:

- `integration_coverage.html` - Interactive HTML coverage report
- `integration_coverage.out` - Raw coverage data
- Console output showing coverage percentages

Open the HTML file in your browser to see detailed coverage information.

## 🐛 Troubleshooting

### Common Issues

1. **Docker not running**
   ```bash
   # Start Docker Desktop or Docker daemon
   sudo systemctl start docker  # Linux
   ```

2. **Port conflicts**
   ```bash
   # Check what's using the ports
   lsof -i :12379 -i :8080 -i :8081
   # Kill conflicting processes
   ```

3. **etcd startup timeout**
   ```bash
   # Check Docker logs
   docker-compose -f docker-compose.test.yml logs etcd
   ```

4. **Permission denied**
   ```bash
   # Make script executable
   chmod +x run_tests.sh
   ```

### Debug Mode

```bash
# Get detailed debug information
make debug

# Check test environment status
make status

# Manual cleanup if needed
make cleanup-test-env
docker system prune -f
```

## 🎯 What Gets Tested

### Data Flow Test
1. **Upload CSV** → Streamer receives and processes CSV file
2. **Message Queue** → Data flows through etcd-based queue
3. **Collection** → Collector processes messages and stores in etcd
4. **API Access** → Gateway serves data via REST/GraphQL APIs
5. **Verification** → All data is accessible and correct

### Service Integration Test
- Services start up and register with etcd
- Health checks pass for all services
- Services communicate through message queue
- Configuration updates propagate correctly
- Scaling decisions are made based on metrics

### API Comprehensive Test
- All REST endpoints return correct data
- GraphQL queries work properly
- WebSocket connections can be established
- Error handling works for invalid requests
- Swagger documentation is accessible

## 📈 Performance Validation

The tests also validate:
- **Response times** < 100ms for simple queries
- **Throughput** > 1000 records/second for CSV processing
- **Concurrent handling** of 10+ simultaneous requests
- **Memory usage** stays within reasonable bounds
- **No memory leaks** during extended operation

## 🔧 Customization

### Environment Variables

You can customize test behavior:

```bash
# Change coverage threshold
export COVERAGE_THRESHOLD=85

# Change test timeout
export TEST_TIMEOUT=15m

# Enable verbose output
export VERBOSE_OUTPUT=true

# Custom etcd endpoints
export ETCD_ENDPOINTS=localhost:12379,localhost:12380
```

### Test Configuration

Edit `test_config.yaml` to customize:
- Service ports and settings
- Test data parameters
- Coverage thresholds
- Performance criteria
- Docker configuration

## 🚀 CI/CD Integration

### GitHub Actions Example

```yaml
name: E2E Tests
on: [push, pull_request]
jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v3
        with:
          go-version: '1.23'
      - name: Run E2E Tests
        run: |
          cd test/integration
          ./run_tests.sh all
      - name: Upload Coverage
        uses: codecov/codecov-action@v3
        with:
          file: ./integration_coverage.out
```

## 📝 Test Results

After successful completion, you'll see:

```
✅ All prerequisites available
✅ Test environment ready
✅ Services built successfully
✅ Integration tests completed
✅ Coverage: 82.5% (meets 80% threshold)
✅ Test environment cleaned up

Generated files:
  - integration_coverage.html (Coverage report)
  - integration_coverage.out (Coverage data)
```

## 🎉 Success Criteria

Tests pass when:
- ✅ All services start successfully
- ✅ CSV upload and processing works
- ✅ All API endpoints return expected data
- ✅ Data persists correctly in etcd
- ✅ Code coverage ≥ 80%
- ✅ No memory leaks or performance issues
- ✅ Error handling works properly
- ✅ Concurrent requests handled correctly

## 🆘 Getting Help

If you encounter issues:

1. Check the troubleshooting section above
2. Run `./run_tests.sh help` for command options
3. Use `make debug` for detailed environment info
4. Check Docker logs: `docker-compose -f docker-compose.test.yml logs`
5. Ensure all prerequisites are installed and running

The tests are designed to be comprehensive, reliable, and easy to run. They provide confidence that the telemetry pipeline works correctly end-to-end with high code coverage.
