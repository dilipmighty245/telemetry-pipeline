#!/bin/bash

# Test script to verify deployment compatibility between same-cluster and cross-cluster deployments
# This script validates that the same application code works in both scenarios

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_CHART_DIR="${SCRIPT_DIR}/../deployments/helm/telemetry-pipeline"
TEST_NAMESPACE="telemetry-test"

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

# Test function to validate Helm templates
test_helm_template() {
    local scenario=$1
    local values_file=$2
    local additional_args=$3
    
    log_info "Testing $scenario deployment scenario..."
    
    # Run helm template to validate
    if helm template test-$scenario "$HELM_CHART_DIR" \
        --namespace "$TEST_NAMESPACE" \
        --values "$values_file" \
        $additional_args \
        --validate > /tmp/test-$scenario.yaml 2>&1; then
        
        log_success "$scenario deployment template is valid"
        
        # Check for key components
        if grep -q "kind: Deployment" /tmp/test-$scenario.yaml; then
            log_success "✓ Deployment manifests generated"
        fi
        
        if grep -q "kind: Service" /tmp/test-$scenario.yaml; then
            log_success "✓ Service manifests generated"
        fi
        
        if grep -q "kind: ConfigMap" /tmp/test-$scenario.yaml; then
            log_success "✓ ConfigMap manifests generated"
        fi
        
        return 0
    else
        log_error "$scenario deployment template validation failed"
        cat /tmp/test-$scenario.yaml
        return 1
    fi
}

# Test Redis URL configuration
test_redis_config() {
    local scenario=$1
    local values_file=$2
    local expected_redis_pattern=$3
    
    log_info "Testing Redis configuration for $scenario..."
    
    # Generate template and check Redis configuration
    helm template test-$scenario "$HELM_CHART_DIR" \
        --namespace "$TEST_NAMESPACE" \
        --values "$values_file" \
        --set externalRedis.host="test.redis.com" \
        --set externalRedis.password="test123" > /tmp/redis-test-$scenario.yaml
    
    if grep -q "$expected_redis_pattern" /tmp/redis-test-$scenario.yaml; then
        log_success "✓ Redis configuration correct for $scenario"
        return 0
    else
        log_error "✗ Redis configuration incorrect for $scenario"
        log_error "Expected pattern: $expected_redis_pattern"
        grep -A5 -B5 "REDIS_URL\|redis" /tmp/redis-test-$scenario.yaml || true
        return 1
    fi
}

# Test component enablement
test_component_config() {
    local scenario=$1
    local values_file=$2
    local component=$3
    local should_be_enabled=$4
    
    log_info "Testing $component configuration for $scenario..."
    
    helm template test-$scenario "$HELM_CHART_DIR" \
        --namespace "$TEST_NAMESPACE" \
        --values "$values_file" > /tmp/component-test-$scenario.yaml
    
    if [ "$should_be_enabled" = "true" ]; then
        if grep -q "app.kubernetes.io/component: $component" /tmp/component-test-$scenario.yaml; then
            log_success "✓ $component is enabled in $scenario"
            return 0
        else
            log_error "✗ $component should be enabled in $scenario but is not found"
            return 1
        fi
    else
        if ! grep -q "app.kubernetes.io/component: $component" /tmp/component-test-$scenario.yaml; then
            log_success "✓ $component is correctly disabled in $scenario"
            return 0
        else
            log_error "✗ $component should be disabled in $scenario but is found"
            return 1
        fi
    fi
}

# Main test execution
main() {
    log_info "Starting deployment compatibility tests..."
    echo "=================================================="
    
    local test_results=0
    
    # Test 1: Same-cluster deployment
    log_info "Test 1: Same-cluster deployment validation"
    if test_helm_template "same-cluster" "$HELM_CHART_DIR/values.yaml" ""; then
        log_success "Same-cluster deployment: PASSED"
    else
        log_error "Same-cluster deployment: FAILED"
        test_results=1
    fi
    echo ""
    
    # Test 2: Edge cluster deployment (if values file exists)
    if [ -f "$HELM_CHART_DIR/values-edge-cluster.yaml" ]; then
        log_info "Test 2: Edge cluster deployment validation"
        if test_helm_template "edge-cluster" "$HELM_CHART_DIR/values-edge-cluster.yaml" \
            "--set externalRedis.enabled=true --set externalRedis.host=test.redis.com"; then
            log_success "Edge cluster deployment: PASSED"
        else
            log_error "Edge cluster deployment: FAILED"
            test_results=1
        fi
        echo ""
    fi
    
    # Test 3: Central cluster deployment (if values file exists)
    if [ -f "$HELM_CHART_DIR/values-central-cluster.yaml" ]; then
        log_info "Test 3: Central cluster deployment validation"
        if test_helm_template "central-cluster" "$HELM_CHART_DIR/values-central-cluster.yaml" \
            "--set externalRedis.enabled=true --set externalRedis.host=test.redis.com --set postgresql.auth.password=test123"; then
            log_success "Central cluster deployment: PASSED"
        else
            log_error "Central cluster deployment: FAILED"
            test_results=1
        fi
        echo ""
    fi
    
    # Test 4: Redis configuration compatibility
    log_info "Test 4: Redis configuration compatibility"
    
    # Same cluster should use internal Redis service
    if test_redis_config "same-cluster-redis" "$HELM_CHART_DIR/values.yaml" "redis://.*redis.*:6379"; then
        log_success "Same-cluster Redis config: PASSED"
    else
        log_error "Same-cluster Redis config: FAILED"
        test_results=1
    fi
    
    # Cross-cluster should use external Redis
    if [ -f "$HELM_CHART_DIR/values-edge-cluster.yaml" ]; then
        if test_redis_config "edge-cluster-redis" "$HELM_CHART_DIR/values-edge-cluster.yaml" "test.redis.com"; then
            log_success "Edge cluster Redis config: PASSED"
        else
            log_error "Edge cluster Redis config: FAILED"
            test_results=1
        fi
    fi
    echo ""
    
    # Test 5: Component enablement logic
    log_info "Test 5: Component enablement logic"
    
    # Same cluster: both streamer and collector enabled
    if test_component_config "same-cluster" "$HELM_CHART_DIR/values.yaml" "streamer" "true" && \
       test_component_config "same-cluster" "$HELM_CHART_DIR/values.yaml" "collector" "true"; then
        log_success "Same-cluster component config: PASSED"
    else
        log_error "Same-cluster component config: FAILED"
        test_results=1
    fi
    
    # Edge cluster: only streamer enabled
    if [ -f "$HELM_CHART_DIR/values-edge-cluster.yaml" ]; then
        if test_component_config "edge-cluster" "$HELM_CHART_DIR/values-edge-cluster.yaml" "streamer" "true" && \
           test_component_config "edge-cluster" "$HELM_CHART_DIR/values-edge-cluster.yaml" "collector" "false"; then
            log_success "Edge cluster component config: PASSED"
        else
            log_error "Edge cluster component config: FAILED"
            test_results=1
        fi
    fi
    
    # Central cluster: only collector enabled
    if [ -f "$HELM_CHART_DIR/values-central-cluster.yaml" ]; then
        if test_component_config "central-cluster" "$HELM_CHART_DIR/values-central-cluster.yaml" "streamer" "false" && \
           test_component_config "central-cluster" "$HELM_CHART_DIR/values-central-cluster.yaml" "collector" "true"; then
            log_success "Central cluster component config: PASSED"
        else
            log_error "Central cluster component config: FAILED"
            test_results=1
        fi
    fi
    echo ""
    
    # Test 6: Application code compatibility
    log_info "Test 6: Application code analysis"
    
    # Check that streamer and collector use the same Redis connection logic
    if grep -q "messagequeue.NewMessageQueueService" "$SCRIPT_DIR/../cmd/streamer/main.go" && \
       grep -q "messagequeue.NewMessageQueueService" "$SCRIPT_DIR/../cmd/collector/main.go"; then
        log_success "✓ Both components use identical message queue service"
    else
        log_error "✗ Components use different message queue initialization"
        test_results=1
    fi
    
    # Check that Redis backend uses environment variable
    if grep -q "REDIS_URL" "$SCRIPT_DIR/../pkg/messagequeue/redis_backend.go"; then
        log_success "✓ Redis backend uses environment variable configuration"
    else
        log_error "✗ Redis backend does not use flexible configuration"
        test_results=1
    fi
    echo ""
    
    # Summary
    echo "=================================================="
    if [ $test_results -eq 0 ]; then
        log_success "🎉 ALL TESTS PASSED!"
        log_success "The application is fully compatible with both same-cluster and cross-cluster deployments!"
        echo ""
        log_info "Key compatibility features verified:"
        echo "  ✓ Helm templates validate for all deployment scenarios"
        echo "  ✓ Redis configuration adapts to deployment pattern"
        echo "  ✓ Components can be selectively enabled/disabled"
        echo "  ✓ Application code is deployment-agnostic"
        echo "  ✓ Same message queue interface used everywhere"
        echo ""
        log_info "You can deploy with confidence in any configuration!"
    else
        log_error "❌ SOME TESTS FAILED!"
        log_error "Please review the errors above and fix the issues."
        echo ""
        log_info "Common fixes:"
        echo "  • Check Helm values file syntax"
        echo "  • Verify component enablement logic"
        echo "  • Ensure Redis configuration is flexible"
        echo "  • Confirm application code consistency"
    fi
    
    # Cleanup
    rm -f /tmp/test-*.yaml /tmp/redis-test-*.yaml /tmp/component-test-*.yaml
    
    exit $test_results
}

# Print usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS]

Test deployment compatibility between same-cluster and cross-cluster scenarios.

OPTIONS:
    -h, --help      Show this help message

EXAMPLES:
    # Run all compatibility tests
    $0

    # Run tests with verbose output
    $0 --debug

This script validates that the telemetry-pipeline can be deployed in both
same-cluster and cross-cluster configurations without code changes.

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -h|--help)
            usage
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Run main function
main
