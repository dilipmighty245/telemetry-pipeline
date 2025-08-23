# Makefile for telemetry-pipeline integration tests

.PHONY: help test-e2e test-coverage test-all setup-test-env cleanup-test-env build-services

# Default target
help:
	@echo "Available targets:"
	@echo "  setup-test-env    - Set up test environment (start etcd)"
	@echo "  cleanup-test-env  - Clean up test environment (stop etcd)"
	@echo "  build-services    - Build all telemetry services"
	@echo "  test-e2e          - Run end-to-end tests"
	@echo "  test-coverage     - Run coverage tests"
	@echo "  test-all          - Run all tests with coverage"
	@echo "  test-quick        - Run quick integration tests"
	@echo "  help              - Show this help message"

# Project root directory
PROJECT_ROOT := $(shell cd ../.. && pwd)

# Test configuration
ETCD_VERSION := v3.5.10
COVERAGE_THRESHOLD := 80
TEST_TIMEOUT := 10m

# Setup test environment
setup-test-env:
	@echo "Setting up test environment..."
	@docker-compose -f docker-compose.test.yml down -v || true
	@docker-compose -f docker-compose.test.yml up -d etcd
	@echo "Waiting for etcd to be ready..."
	@timeout 60s bash -c 'until docker-compose -f docker-compose.test.yml exec -T etcd etcdctl endpoint health; do sleep 2; done'
	@echo "Test environment ready!"

# Clean up test environment
cleanup-test-env:
	@echo "Cleaning up test environment..."
	@docker-compose -f docker-compose.test.yml down -v
	@docker system prune -f --volumes || true
	@echo "Test environment cleaned up!"

# Build all services
build-services:
	@echo "Building telemetry services..."
	@cd $(PROJECT_ROOT) && go build -o bin/nexus-streamer ./cmd/nexus-streamer
	@cd $(PROJECT_ROOT) && go build -o bin/nexus-collector ./cmd/nexus-collector
	@cd $(PROJECT_ROOT) && go build -o bin/nexus-gateway ./cmd/nexus-gateway
	@echo "Services built successfully!"

# Run end-to-end tests
test-e2e: setup-test-env
	@echo "Running end-to-end tests..."
	@cd $(PROJECT_ROOT) && go test -v -tags=integration -timeout=$(TEST_TIMEOUT) ./test/integration -run TestE2ETestSuite
	@$(MAKE) cleanup-test-env

# Run coverage tests
test-coverage:
	@echo "Running coverage tests..."
	@cd $(PROJECT_ROOT) && go test -v -tags=integration -timeout=$(TEST_TIMEOUT) ./test/integration -run TestCoverage

# Run all tests with coverage
test-all: setup-test-env
	@echo "Running all integration tests with coverage..."
	@cd $(PROJECT_ROOT) && go test -v -tags=integration -coverprofile=integration_coverage.out -coverpkg=./... -timeout=$(TEST_TIMEOUT) ./test/integration
	@cd $(PROJECT_ROOT) && go tool cover -html=integration_coverage.out -o integration_coverage.html
	@echo "Coverage report generated: $(PROJECT_ROOT)/integration_coverage.html"
	@$(MAKE) cleanup-test-env

# Run quick integration tests (without full e2e setup)
test-quick:
	@echo "Running quick integration tests..."
	@cd $(PROJECT_ROOT) && go test -v -short -tags=integration ./test/integration

# Run tests with race detection
test-race: setup-test-env
	@echo "Running tests with race detection..."
	@cd $(PROJECT_ROOT) && go test -v -race -tags=integration -timeout=$(TEST_TIMEOUT) ./test/integration
	@$(MAKE) cleanup-test-env

# Run performance tests
test-performance: setup-test-env
	@echo "Running performance tests..."
	@cd $(PROJECT_ROOT) && go test -v -tags=integration -bench=. -benchmem -timeout=$(TEST_TIMEOUT) ./test/integration
	@$(MAKE) cleanup-test-env

# Check test dependencies
check-deps:
	@echo "Checking test dependencies..."
	@command -v docker >/dev/null 2>&1 || { echo "Docker is required but not installed. Aborting." >&2; exit 1; }
	@command -v docker-compose >/dev/null 2>&1 || { echo "Docker Compose is required but not installed. Aborting." >&2; exit 1; }
	@echo "All dependencies are available!"

# Lint integration tests
lint:
	@echo "Linting integration tests..."
	@cd $(PROJECT_ROOT) && golangci-lint run ./test/integration/...

# Format integration test code
fmt:
	@echo "Formatting integration test code..."
	@cd $(PROJECT_ROOT) && go fmt ./test/integration/...

# Run tests in CI environment
test-ci: check-deps
	@echo "Running tests in CI environment..."
	@$(MAKE) test-all
	@echo "CI tests completed successfully!"

# Generate test data
generate-test-data:
	@echo "Generating test data..."
	@mkdir -p test-data
	@cat > test-data/sample_telemetry.csv << 'EOF'
timestamp,gpu_id,uuid,device,modelName,Hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2024-01-15T10:00:00Z,0,GPU-12345-67890-ABCDE,nvidia0,NVIDIA H100 80GB HBM3,test-host-1,85.5,60.2,48000,32000,72.0,350.5,1410,1215
2024-01-15T10:00:01Z,1,GPU-12345-67890-ABCDF,nvidia1,NVIDIA H100 80GB HBM3,test-host-1,92.1,75.8,60800,19200,75.5,380.2,1410,1215
2024-01-15T10:00:02Z,0,GPU-FEDCB-09876-54321,nvidia0,NVIDIA H100 80GB HBM3,test-host-2,78.3,55.1,44080,35920,70.2,340.8,1410,1215
EOF
	@echo "Test data generated in test-data/"

# Clean test artifacts
clean:
	@echo "Cleaning test artifacts..."
	@rm -rf test-data/
	@rm -f $(PROJECT_ROOT)/*.out $(PROJECT_ROOT)/*.html
	@rm -rf $(PROJECT_ROOT)/bin/
	@echo "Test artifacts cleaned!"

# Show test status
status:
	@echo "Test environment status:"
	@docker-compose -f docker-compose.test.yml ps || echo "No containers running"
	@echo ""
	@echo "Available test files:"
	@ls -la *.go 2>/dev/null || echo "No test files found"

# Debug test environment
debug: setup-test-env
	@echo "Debug information:"
	@echo "Project root: $(PROJECT_ROOT)"
	@echo "Docker containers:"
	@docker-compose -f docker-compose.test.yml ps
	@echo "etcd status:"
	@docker-compose -f docker-compose.test.yml exec -T etcd etcdctl endpoint status --write-out=table || true
	@echo "etcd health:"
	@docker-compose -f docker-compose.test.yml exec -T etcd etcdctl endpoint health || true

# Run specific test
test-specific:
	@echo "Running specific test: $(TEST)"
	@cd $(PROJECT_ROOT) && go test -v -tags=integration -run $(TEST) ./test/integration

# Example usage:
# make test-specific TEST=TestHealthEndpoints
