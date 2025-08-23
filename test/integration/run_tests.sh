#!/bin/bash

# Telemetry Pipeline E2E Test Runner
# This script provides an easy way to run comprehensive integration tests

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COVERAGE_THRESHOLD=80
TEST_TIMEOUT="10m"

# Functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_prerequisites() {
    log_info "Checking prerequisites..."
    
    # Check Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        exit 1
    fi
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed or not in PATH"
        exit 1
    fi
    
    # Check Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        log_error "Docker Compose is not installed or not in PATH"
        exit 1
    fi
    
    # Check if Docker is running
    if ! docker info &> /dev/null; then
        log_error "Docker is not running"
        exit 1
    fi
    
    log_success "All prerequisites are available"
}

setup_environment() {
    log_info "Setting up test environment..."
    
    cd "$TEST_DIR"
    
    # Stop any existing containers
    docker-compose -f docker-compose.test.yml down -v &> /dev/null || true
    
    # Start etcd
    log_info "Starting etcd container..."
    docker-compose -f docker-compose.test.yml up -d etcd
    
    # Wait for etcd to be ready
    log_info "Waiting for etcd to be ready..."
    local timeout=60
    local count=0
    
    while [ $count -lt $timeout ]; do
        if docker-compose -f docker-compose.test.yml exec -T etcd etcdctl endpoint health &> /dev/null; then
            log_success "etcd is ready"
            return 0
        fi
        sleep 2
        count=$((count + 2))
    done
    
    log_error "Timeout waiting for etcd to be ready"
    cleanup_environment
    exit 1
}

cleanup_environment() {
    log_info "Cleaning up test environment..."
    cd "$TEST_DIR"
    docker-compose -f docker-compose.test.yml down -v &> /dev/null || true
    log_success "Test environment cleaned up"
}

build_services() {
    log_info "Building telemetry services..."
    cd "$PROJECT_ROOT"
    
    # Create bin directory
    mkdir -p bin
    
    # Build services
    go build -o bin/nexus-streamer ./cmd/nexus-streamer
    go build -o bin/nexus-collector ./cmd/nexus-collector
    go build -o bin/nexus-gateway ./cmd/nexus-gateway
    
    log_success "Services built successfully"
}

run_unit_tests() {
    log_info "Running unit tests..."
    cd "$PROJECT_ROOT"
    
    # Run unit tests with coverage
    go test -v -coverprofile=unit_coverage.out -coverpkg=./... ./... -short
    
    # Generate coverage report
    go tool cover -html=unit_coverage.out -o unit_coverage.html
    
    log_success "Unit tests completed"
}

run_integration_tests() {
    log_info "Running integration tests..."
    cd "$PROJECT_ROOT"
    
    # Run integration tests with coverage
    go test -v -tags=integration -coverprofile=integration_coverage.out -coverpkg=./... -timeout="$TEST_TIMEOUT" ./test/integration
    
    # Generate coverage report
    go tool cover -html=integration_coverage.out -o integration_coverage.html
    
    log_success "Integration tests completed"
}

run_e2e_tests() {
    log_info "Running E2E tests..."
    cd "$PROJECT_ROOT"
    
    # Run E2E test suite
    go test -v -tags=integration -timeout="$TEST_TIMEOUT" ./test/integration -run TestE2ETestSuite
    
    log_success "E2E tests completed"
}

run_coverage_tests() {
    log_info "Running coverage tests..."
    cd "$PROJECT_ROOT"
    
    # Run coverage verification tests
    go test -v -tags=integration -timeout="$TEST_TIMEOUT" ./test/integration -run TestCoverage
    
    log_success "Coverage tests completed"
}

check_coverage() {
    log_info "Checking coverage threshold..."
    cd "$PROJECT_ROOT"
    
    if [ -f "integration_coverage.out" ]; then
        # Calculate coverage percentage
        local coverage=$(go tool cover -func=integration_coverage.out | grep total | awk '{print $3}' | sed 's/%//')
        
        if (( $(echo "$coverage >= $COVERAGE_THRESHOLD" | bc -l) )); then
            log_success "Coverage: ${coverage}% (meets ${COVERAGE_THRESHOLD}% threshold)"
        else
            log_error "Coverage: ${coverage}% (below ${COVERAGE_THRESHOLD}% threshold)"
            return 1
        fi
    else
        log_warning "No coverage file found"
    fi
}

show_results() {
    log_info "Test Results Summary:"
    cd "$PROJECT_ROOT"
    
    echo "Generated files:"
    [ -f "unit_coverage.html" ] && echo "  - unit_coverage.html (Unit test coverage report)"
    [ -f "integration_coverage.html" ] && echo "  - integration_coverage.html (Integration test coverage report)"
    [ -f "integration_coverage.out" ] && echo "  - integration_coverage.out (Coverage data)"
    
    echo ""
    echo "To view coverage reports:"
    [ -f "unit_coverage.html" ] && echo "  open unit_coverage.html"
    [ -f "integration_coverage.html" ] && echo "  open integration_coverage.html"
}

# Main execution
main() {
    local command="${1:-all}"
    
    echo "Telemetry Pipeline E2E Test Runner"
    echo "=================================="
    echo ""
    
    case "$command" in
        "check")
            check_prerequisites
            ;;
        "setup")
            check_prerequisites
            setup_environment
            ;;
        "build")
            build_services
            ;;
        "unit")
            check_prerequisites
            run_unit_tests
            ;;
        "integration")
            check_prerequisites
            setup_environment
            trap cleanup_environment EXIT
            run_integration_tests
            check_coverage
            show_results
            ;;
        "e2e")
            check_prerequisites
            setup_environment
            trap cleanup_environment EXIT
            run_e2e_tests
            ;;
        "coverage")
            check_prerequisites
            run_coverage_tests
            ;;
        "all")
            check_prerequisites
            setup_environment
            trap cleanup_environment EXIT
            build_services
            run_integration_tests
            check_coverage
            show_results
            ;;
        "clean")
            cleanup_environment
            cd "$PROJECT_ROOT"
            rm -f *.out *.html
            rm -rf bin/
            log_success "Cleaned up all test artifacts"
            ;;
        "help"|"-h"|"--help")
            echo "Usage: $0 [command]"
            echo ""
            echo "Commands:"
            echo "  check       - Check prerequisites"
            echo "  setup       - Set up test environment"
            echo "  build       - Build services"
            echo "  unit        - Run unit tests only"
            echo "  integration - Run integration tests"
            echo "  e2e         - Run E2E tests only"
            echo "  coverage    - Run coverage tests"
            echo "  all         - Run all tests (default)"
            echo "  clean       - Clean up test artifacts"
            echo "  help        - Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                    # Run all tests"
            echo "  $0 integration        # Run integration tests only"
            echo "  $0 e2e               # Run E2E tests only"
            echo "  $0 clean             # Clean up"
            ;;
        *)
            log_error "Unknown command: $command"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"
