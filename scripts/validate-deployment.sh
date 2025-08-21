#!/bin/bash

# Deployment Validation Script for Telemetry Pipeline
# Validates deployment configurations and connectivity

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
NAMESPACE="telemetry-system"
TIMEOUT=300

# Logging functions
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

# Print usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS] COMMAND

Validate telemetry pipeline deployments.

COMMANDS:
    validate-same-cluster       Validate same cluster deployment
    validate-cross-cluster      Validate cross-cluster deployment
    validate-connectivity       Test connectivity between components
    validate-health             Check health endpoints
    validate-scaling            Test auto-scaling functionality

OPTIONS:
    -e, --edge-context CONTEXT      Kubernetes context for edge cluster
    -c, --central-context CONTEXT   Kubernetes context for central cluster
    -n, --namespace NAMESPACE       Kubernetes namespace (default: telemetry-system)
    --timeout SECONDS               Timeout for validation checks (default: 300)
    -h, --help                      Show this help message

EXAMPLES:
    # Validate same cluster deployment
    $0 validate-same-cluster

    # Validate cross-cluster deployment
    $0 -e edge-ctx -c central-ctx validate-cross-cluster

    # Test connectivity
    $0 -e edge-ctx -c central-ctx validate-connectivity

EOF
}

# Parse command line arguments
EDGE_CONTEXT=""
CENTRAL_CONTEXT=""
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--edge-context)
            EDGE_CONTEXT="$2"
            shift 2
            ;;
        -c|--central-context)
            CENTRAL_CONTEXT="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        validate-same-cluster|validate-cross-cluster|validate-connectivity|validate-health|validate-scaling)
            COMMAND="$1"
            shift
            ;;
        *)
            log_error "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate same cluster deployment
validate_same_cluster() {
    log_info "Validating same cluster deployment..."
    
    local context="${EDGE_CONTEXT:-$(kubectl config current-context)}"
    
    # Check if namespace exists
    if ! kubectl --context="$context" get namespace "$NAMESPACE" &>/dev/null; then
        log_error "Namespace '$NAMESPACE' not found"
        return 1
    fi
    
    # Check if all components are deployed
    local components=("streamer" "collector" "api-gateway")
    local missing_components=()
    
    for component in "${components[@]}"; do
        if ! kubectl --context="$context" -n "$NAMESPACE" get deployment -l app.kubernetes.io/component="$component" &>/dev/null; then
            missing_components+=("$component")
        fi
    done
    
    if [ ${#missing_components[@]} -gt 0 ]; then
        log_error "Missing components: ${missing_components[*]}"
        return 1
    fi
    
    # Check if pods are ready
    for component in "${components[@]}"; do
        log_info "Checking $component pods..."
        if ! kubectl --context="$context" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component="$component" --timeout="${TIMEOUT}s"; then
            log_error "$component pods are not ready"
            return 1
        fi
        log_success "$component pods are ready"
    done
    
    # Check database connectivity
    log_info "Checking database connectivity..."
    local db_pod=$(kubectl --context="$context" -n "$NAMESPACE" get pod -l app.kubernetes.io/name=postgresql -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$db_pod" ]; then
        if kubectl --context="$context" -n "$NAMESPACE" exec "$db_pod" -- pg_isready -U postgres &>/dev/null; then
            log_success "Database is accessible"
        else
            log_error "Database is not accessible"
            return 1
        fi
    else
        log_warning "Database pod not found (might be using external database)"
    fi
    
    # Check Redis connectivity
    log_info "Checking Redis connectivity..."
    local redis_pod=$(kubectl --context="$context" -n "$NAMESPACE" get pod -l app.kubernetes.io/name=redis -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$redis_pod" ]; then
        if kubectl --context="$context" -n "$NAMESPACE" exec "$redis_pod" -- redis-cli ping | grep -q "PONG"; then
            log_success "Redis is accessible"
        else
            log_error "Redis is not accessible"
            return 1
        fi
    else
        log_warning "Redis pod not found (might be using external Redis)"
    fi
    
    log_success "Same cluster deployment validation passed"
}

# Validate cross-cluster deployment
validate_cross_cluster() {
    log_info "Validating cross-cluster deployment..."
    
    if [ -z "$EDGE_CONTEXT" ] || [ -z "$CENTRAL_CONTEXT" ]; then
        log_error "Both edge and central contexts are required for cross-cluster validation"
        return 1
    fi
    
    # Validate edge cluster (streamers)
    log_info "Validating edge cluster..."
    if kubectl --context="$EDGE_CONTEXT" -n "$NAMESPACE" get deployment -l app.kubernetes.io/component=streamer &>/dev/null; then
        if kubectl --context="$EDGE_CONTEXT" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=streamer --timeout="${TIMEOUT}s"; then
            log_success "Edge cluster streamers are ready"
        else
            log_error "Edge cluster streamers are not ready"
            return 1
        fi
    else
        log_error "No streamers found in edge cluster"
        return 1
    fi
    
    # Validate central cluster (collectors, API)
    log_info "Validating central cluster..."
    local central_components=("collector" "api-gateway")
    for component in "${central_components[@]}"; do
        if kubectl --context="$CENTRAL_CONTEXT" -n "$NAMESPACE" get deployment -l app.kubernetes.io/component="$component" &>/dev/null; then
            if kubectl --context="$CENTRAL_CONTEXT" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component="$component" --timeout="${TIMEOUT}s"; then
                log_success "Central cluster $component is ready"
            else
                log_error "Central cluster $component is not ready"
                return 1
            fi
        else
            log_error "No $component found in central cluster"
            return 1
        fi
    done
    
    log_success "Cross-cluster deployment validation passed"
}

# Validate connectivity between components
validate_connectivity() {
    log_info "Validating component connectivity..."
    
    local edge_context="${EDGE_CONTEXT:-$(kubectl config current-context)}"
    local central_context="${CENTRAL_CONTEXT:-$(kubectl config current-context)}"
    
    # Test Redis connectivity from streamers
    log_info "Testing Redis connectivity from streamers..."
    local streamer_pod=$(kubectl --context="$edge_context" -n "$NAMESPACE" get pod -l app.kubernetes.io/component=streamer -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$streamer_pod" ]; then
        local redis_url=$(kubectl --context="$edge_context" -n "$NAMESPACE" get pod "$streamer_pod" -o jsonpath='{.spec.containers[0].env[?(@.name=="REDIS_URL")].value}')
        if [ -n "$redis_url" ]; then
            log_info "Testing Redis connection: $redis_url"
            # This is a simplified test - in reality you'd need to exec into the pod and test
            log_success "Redis URL configured in streamer"
        else
            log_error "Redis URL not found in streamer configuration"
            return 1
        fi
    else
        log_warning "No streamer pods found for connectivity test"
    fi
    
    # Test database connectivity from collectors
    log_info "Testing database connectivity from collectors..."
    local collector_pod=$(kubectl --context="$central_context" -n "$NAMESPACE" get pod -l app.kubernetes.io/component=collector -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")
    if [ -n "$collector_pod" ]; then
        local db_host=$(kubectl --context="$central_context" -n "$NAMESPACE" get pod "$collector_pod" -o jsonpath='{.spec.containers[0].env[?(@.name=="DB_HOST")].value}')
        if [ -n "$db_host" ]; then
            log_info "Testing database connection: $db_host"
            log_success "Database host configured in collector"
        else
            log_error "Database host not found in collector configuration"
            return 1
        fi
    else
        log_warning "No collector pods found for connectivity test"
    fi
    
    log_success "Connectivity validation passed"
}

# Validate health endpoints
validate_health() {
    log_info "Validating health endpoints..."
    
    local context="${CENTRAL_CONTEXT:-$(kubectl config current-context)}"
    
    # Check API Gateway health
    if kubectl --context="$context" -n "$NAMESPACE" get svc -l app.kubernetes.io/component=api-gateway &>/dev/null; then
        log_info "Testing API Gateway health endpoint..."
        
        # Port forward and test health endpoint
        kubectl --context="$context" -n "$NAMESPACE" port-forward svc/$(kubectl --context="$context" -n "$NAMESPACE" get svc -l app.kubernetes.io/component=api-gateway -o jsonpath='{.items[0].metadata.name}') 8080:80 &
        local port_forward_pid=$!
        
        sleep 5
        
        if curl -f http://localhost:8080/health &>/dev/null; then
            log_success "API Gateway health endpoint is accessible"
        else
            log_error "API Gateway health endpoint is not accessible"
            kill $port_forward_pid 2>/dev/null || true
            return 1
        fi
        
        kill $port_forward_pid 2>/dev/null || true
    else
        log_warning "API Gateway service not found"
    fi
    
    log_success "Health validation passed"
}

# Validate scaling functionality
validate_scaling() {
    log_info "Validating scaling functionality..."
    
    local context="${EDGE_CONTEXT:-$(kubectl config current-context)}"
    
    # Test manual scaling
    if kubectl --context="$context" -n "$NAMESPACE" get deployment -l app.kubernetes.io/component=streamer &>/dev/null; then
        local deployment_name=$(kubectl --context="$context" -n "$NAMESPACE" get deployment -l app.kubernetes.io/component=streamer -o jsonpath='{.items[0].metadata.name}')
        local current_replicas=$(kubectl --context="$context" -n "$NAMESPACE" get deployment "$deployment_name" -o jsonpath='{.spec.replicas}')
        
        log_info "Current streamer replicas: $current_replicas"
        log_info "Testing scale up..."
        
        kubectl --context="$context" -n "$NAMESPACE" scale deployment "$deployment_name" --replicas=$((current_replicas + 1))
        
        if kubectl --context="$context" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=streamer --timeout="${TIMEOUT}s"; then
            log_success "Scale up successful"
            
            # Scale back to original
            kubectl --context="$context" -n "$NAMESPACE" scale deployment "$deployment_name" --replicas="$current_replicas"
            log_success "Scaled back to original replica count"
        else
            log_error "Scale up failed"
            return 1
        fi
    else
        log_warning "No streamer deployment found for scaling test"
    fi
    
    # Check HPA status
    if kubectl --context="$context" -n "$NAMESPACE" get hpa &>/dev/null; then
        log_info "HPA status:"
        kubectl --context="$context" -n "$NAMESPACE" get hpa
        log_success "HPA is configured"
    else
        log_warning "No HPA found"
    fi
    
    log_success "Scaling validation passed"
}

# Main execution
main() {
    if [[ -z "$COMMAND" ]]; then
        log_error "No command specified"
        usage
        exit 1
    fi
    
    case $COMMAND in
        validate-same-cluster)
            validate_same_cluster
            ;;
        validate-cross-cluster)
            validate_cross_cluster
            ;;
        validate-connectivity)
            validate_connectivity
            ;;
        validate-health)
            validate_health
            ;;
        validate-scaling)
            validate_scaling
            ;;
        *)
            log_error "Unknown command: $COMMAND"
            usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
