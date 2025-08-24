#!/bin/bash

# Comprehensive Test Runner for Telemetry Pipeline
# This script runs all tests with full coverage analysis

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
COVERAGE_THRESHOLD=80
PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_DIR="$PROJECT_ROOT/coverage-reports"
TIMEOUT_UNIT="5m"
TIMEOUT_INTEGRATION="15m"
TIMEOUT_E2E="10m"

# Functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}"
}

log_step() {
    echo -e "\n${BLUE}🔄 $1${NC}"
}

check_prerequisites() {
    log_step "Checking prerequisites..."
    
    # Check Go
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed"
        exit 1
    fi
    log_info "Go version: $(go version)"
    
    # Check Docker (optional for integration tests)
    if command -v docker &> /dev/null; then
        log_info "Docker is available for integration tests"
        DOCKER_AVAILABLE=true
    else
        log_warning "Docker not available, integration tests will be skipped"
        DOCKER_AVAILABLE=false
    fi
    
    # Check etcd (optional for E2E tests)
    if command -v etcd &> /dev/null; then
        log_info "etcd is available for E2E tests"
        ETCD_AVAILABLE=true
    else
        log_warning "etcd not available, E2E tests will be skipped"
        ETCD_AVAILABLE=false
    fi
    
    # Check bc for coverage calculation
    if ! command -v bc &> /dev/null; then
        log_warning "bc not available, using basic coverage calculation"
        BC_AVAILABLE=false
    else
        BC_AVAILABLE=true
    fi
}

setup_environment() {
    log_step "Setting up test environment..."
    
    cd "$PROJECT_ROOT"
    
    # Clean previous artifacts
    rm -f *.out *.html benchmark-results.txt coverage-summary.txt
    rm -rf "$COVERAGE_DIR"
    mkdir -p "$COVERAGE_DIR"
    
    # Clean test cache
    go clean -testcache
    
    log_success "Environment setup complete"
}

run_unit_tests() {
    log_step "Running unit tests with race detection..."
    
    if go test -v -race -coverprofile=unit-coverage.out -coverpkg=./... -timeout="$TIMEOUT_UNIT" ./...; then
        log_success "Unit tests passed"
        return 0
    else
        log_error "Unit tests failed"
        return 1
    fi
}

run_integration_tests() {
    if [ "$DOCKER_AVAILABLE" = false ]; then
        log_warning "Skipping integration tests (Docker not available)"
        return 0
    fi
    
    log_step "Running integration tests..."
    
    if go test -v -tags=integration -coverprofile=integration-coverage.out -coverpkg=./... -timeout="$TIMEOUT_INTEGRATION" ./test/integration/...; then
        log_success "Integration tests passed"
        return 0
    else
        log_error "Integration tests failed"
        return 1
    fi
}

run_e2e_tests() {
    if [ "$ETCD_AVAILABLE" = false ]; then
        log_warning "Skipping E2E tests (etcd not available)"
        return 0
    fi
    
    log_step "Running E2E tests..."
    
    if go test -v -coverprofile=e2e-coverage.out -coverpkg=./... -timeout="$TIMEOUT_E2E" ./test/e2e/...; then
        log_success "E2E tests passed"
        return 0
    else
        log_error "E2E tests failed"
        return 1
    fi
}

run_benchmark_tests() {
    log_step "Running benchmark tests..."
    
    if go test -bench=. -benchmem -run=^$ ./... > benchmark-results.txt 2>&1; then
        log_success "Benchmark tests completed"
        log_info "Results saved to benchmark-results.txt"
    else
        log_warning "Some benchmarks may not exist yet"
    fi
}

merge_coverage_profiles() {
    log_step "Merging coverage profiles..."
    
    echo "mode: set" > coverage.out
    
    for file in unit-coverage.out integration-coverage.out e2e-coverage.out; do
        if [ -f "$file" ]; then
            tail -n +2 "$file" >> coverage.out
            log_info "Merged $file"
        fi
    done
    
    log_success "Coverage profiles merged"
}

generate_coverage_reports() {
    log_step "Generating coverage reports..."
    
    # Main coverage report
    if [ -f "coverage.out" ]; then
        go tool cover -html=coverage.out -o coverage.html
        go tool cover -func=coverage.out > coverage-summary.txt
        log_success "Main coverage report: coverage.html"
    fi
    
    # Individual reports
    for file in unit-coverage.out integration-coverage.out e2e-coverage.out; do
        if [ -f "$file" ]; then
            base_name=$(basename "$file" .out)
            go tool cover -html="$file" -o "$COVERAGE_DIR/${base_name}.html"
            log_info "Generated $COVERAGE_DIR/${base_name}.html"
        fi
    done
    
    log_success "All coverage reports generated"
}

analyze_coverage() {
    log_step "Analyzing coverage thresholds..."
    
    if [ ! -f "coverage.out" ]; then
        log_error "No coverage data found"
        return 1
    fi
    
    # Extract coverage percentage
    if [ "$BC_AVAILABLE" = true ]; then
        COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
        THRESHOLD_MET=$(echo "$COVERAGE >= $COVERAGE_THRESHOLD" | bc -l)
    else
        # Fallback without bc
        COVERAGE=$(go tool cover -func=coverage.out | tail -1 | awk '{print $3}' | sed 's/%//')
        if (( $(echo "$COVERAGE >= $COVERAGE_THRESHOLD" | awk '{print ($1 >= $2)}') )); then
            THRESHOLD_MET=1
        else
            THRESHOLD_MET=0
        fi
    fi
    
    echo ""
    echo "📊 Coverage Analysis Results:"
    echo "================================"
    echo "Overall Coverage: ${COVERAGE}%"
    echo "Target Threshold: ${COVERAGE_THRESHOLD}%"
    echo ""
    
    if [ "$THRESHOLD_MET" = "1" ]; then
        log_success "Coverage threshold met (≥${COVERAGE_THRESHOLD}%)"
        COVERAGE_PASSED=true
    else
        log_error "Coverage below threshold (target: ≥${COVERAGE_THRESHOLD}%, actual: ${COVERAGE}%)"
        log_info "Consider adding more tests to improve coverage"
        COVERAGE_PASSED=false
    fi
    
    # Show top uncovered packages
    echo ""
    echo "📋 Coverage by Package:"
    echo "======================="
    go tool cover -func=coverage.out | grep -v "total:" | sort -k3 -n | head -10
    
    return 0
}

print_summary() {
    echo ""
    echo "🎯 Test Summary"
    echo "==============="
    echo "📊 Coverage Reports:"
    echo "  - Main Report: coverage.html"
    echo "  - Unit Tests: $COVERAGE_DIR/unit-coverage.html"
    [ -f "integration-coverage.out" ] && echo "  - Integration Tests: $COVERAGE_DIR/integration-coverage.html"
    [ -f "e2e-coverage.out" ] && echo "  - E2E Tests: $COVERAGE_DIR/e2e-coverage.html"
    echo "  - Benchmark Results: benchmark-results.txt"
    echo ""
    
    if [ -f "coverage.out" ]; then
        echo "📈 Final Coverage:"
        go tool cover -func=coverage.out | tail -1
    fi
    
    echo ""
    if [ "$COVERAGE_PASSED" = true ]; then
        log_success "All tests completed successfully with adequate coverage!"
    else
        log_warning "Tests completed but coverage is below threshold"
    fi
}

# Main execution
main() {
    echo "🚀 Telemetry Pipeline Comprehensive Test Suite"
    echo "=============================================="
    
    check_prerequisites
    setup_environment
    
    # Track test results
    UNIT_PASSED=false
    INTEGRATION_PASSED=false
    E2E_PASSED=false
    COVERAGE_PASSED=false
    
    # Run tests
    if run_unit_tests; then
        UNIT_PASSED=true
    fi
    
    if run_integration_tests; then
        INTEGRATION_PASSED=true
    fi
    
    if run_e2e_tests; then
        E2E_PASSED=true
    fi
    
    run_benchmark_tests
    merge_coverage_profiles
    generate_coverage_reports
    
    if analyze_coverage; then
        if [ "$COVERAGE_PASSED" = true ]; then
            COVERAGE_PASSED=true
        fi
    fi
    
    print_summary
    
    # Exit with appropriate code
    if [ "$UNIT_PASSED" = true ] && [ "$COVERAGE_PASSED" = true ]; then
        exit 0
    else
        exit 1
    fi
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0 [options]"
        echo ""
        echo "Options:"
        echo "  --help, -h          Show this help message"
        echo "  --unit-only         Run only unit tests"
        echo "  --integration-only  Run only integration tests"
        echo "  --e2e-only         Run only E2E tests"
        echo "  --no-coverage      Skip coverage analysis"
        echo ""
        echo "Environment Variables:"
        echo "  COVERAGE_THRESHOLD  Coverage threshold percentage (default: 80)"
        echo "  TIMEOUT_UNIT       Unit test timeout (default: 5m)"
        echo "  TIMEOUT_INTEGRATION Integration test timeout (default: 15m)"
        echo "  TIMEOUT_E2E        E2E test timeout (default: 10m)"
        exit 0
        ;;
    --unit-only)
        check_prerequisites
        setup_environment
        run_unit_tests
        exit $?
        ;;
    --integration-only)
        check_prerequisites
        setup_environment
        run_integration_tests
        exit $?
        ;;
    --e2e-only)
        check_prerequisites
        setup_environment
        run_e2e_tests
        exit $?
        ;;
    --no-coverage)
        # Run tests without coverage analysis
        check_prerequisites
        setup_environment
        run_unit_tests
        run_integration_tests
        run_e2e_tests
        run_benchmark_tests
        exit 0
        ;;
    "")
        # Default: run all tests
        main
        ;;
    *)
        echo "Unknown option: $1"
        echo "Use --help for usage information"
        exit 1
        ;;
esac
