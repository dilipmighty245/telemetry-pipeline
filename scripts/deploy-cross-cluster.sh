#!/bin/bash

# Cross-Cluster Deployment Script for Telemetry Pipeline
# This script automates the deployment of streamers and collectors across different clusters

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HELM_CHART_DIR="${SCRIPT_DIR}/../deployments/helm/telemetry-pipeline"
NAMESPACE="telemetry-system"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

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

Cross-cluster deployment script for telemetry pipeline.

COMMANDS:
    deploy-edge         Deploy streamers to edge cluster
    deploy-central      Deploy collectors and API to central cluster
    deploy-shared       Deploy shared Redis infrastructure
    deploy-all          Deploy all components (requires multiple cluster contexts)
    status              Check deployment status
    cleanup             Clean up deployments

OPTIONS:
    -e, --edge-context CONTEXT      Kubernetes context for edge cluster
    -c, --central-context CONTEXT   Kubernetes context for central cluster
    -s, --shared-context CONTEXT    Kubernetes context for shared infrastructure
    -r, --redis-host HOST           External Redis host (required for cross-cluster)
    -p, --redis-password PASSWORD   Redis password
    -d, --db-password PASSWORD      Database password
    -n, --namespace NAMESPACE       Kubernetes namespace (default: telemetry-system)
    --dry-run                       Perform a dry run without actual deployment
    --debug                         Enable debug output
    -h, --help                      Show this help message

EXAMPLES:
    # Deploy to edge cluster
    $0 -e edge-k8s-context -r redis.company.com -p redis123 deploy-edge

    # Deploy to central cluster
    $0 -c central-k8s-context -r redis.company.com -p redis123 -d db123 deploy-central

    # Deploy all components
    $0 -e edge-ctx -c central-ctx -s shared-ctx -r redis.company.com -p redis123 -d db123 deploy-all

    # Check status
    $0 -e edge-ctx -c central-ctx status

EOF
}

# Parse command line arguments
EDGE_CONTEXT=""
CENTRAL_CONTEXT=""
SHARED_CONTEXT=""
REDIS_HOST=""
REDIS_PASSWORD=""
DB_PASSWORD=""
DRY_RUN=""
DEBUG=""
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
        -s|--shared-context)
            SHARED_CONTEXT="$2"
            shift 2
            ;;
        -r|--redis-host)
            REDIS_HOST="$2"
            shift 2
            ;;
        -p|--redis-password)
            REDIS_PASSWORD="$2"
            shift 2
            ;;
        -d|--db-password)
            DB_PASSWORD="$2"
            shift 2
            ;;
        -n|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN="--dry-run"
            shift
            ;;
        --debug)
            DEBUG="--debug"
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        deploy-edge|deploy-central|deploy-shared|deploy-all|status|cleanup)
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

# Validate required parameters
validate_params() {
    case $COMMAND in
        deploy-edge)
            if [[ -z "$EDGE_CONTEXT" ]] || [[ -z "$REDIS_HOST" ]] || [[ -z "$REDIS_PASSWORD" ]]; then
                log_error "Edge deployment requires: --edge-context, --redis-host, --redis-password"
                exit 1
            fi
            ;;
        deploy-central)
            if [[ -z "$CENTRAL_CONTEXT" ]] || [[ -z "$REDIS_HOST" ]] || [[ -z "$REDIS_PASSWORD" ]] || [[ -z "$DB_PASSWORD" ]]; then
                log_error "Central deployment requires: --central-context, --redis-host, --redis-password, --db-password"
                exit 1
            fi
            ;;
        deploy-shared)
            if [[ -z "$SHARED_CONTEXT" ]] || [[ -z "$REDIS_PASSWORD" ]]; then
                log_error "Shared infrastructure deployment requires: --shared-context, --redis-password"
                exit 1
            fi
            ;;
        deploy-all)
            if [[ -z "$EDGE_CONTEXT" ]] || [[ -z "$CENTRAL_CONTEXT" ]] || [[ -z "$REDIS_HOST" ]] || [[ -z "$REDIS_PASSWORD" ]] || [[ -z "$DB_PASSWORD" ]]; then
                log_error "Full deployment requires: --edge-context, --central-context, --redis-host, --redis-password, --db-password"
                exit 1
            fi
            ;;
        status|cleanup)
            if [[ -z "$EDGE_CONTEXT" ]] && [[ -z "$CENTRAL_CONTEXT" ]] && [[ -z "$SHARED_CONTEXT" ]]; then
                log_error "At least one cluster context is required"
                exit 1
            fi
            ;;
        "")
            log_error "Command is required"
            usage
            exit 1
            ;;
    esac
}

# Check if kubectl context exists and is accessible
check_context() {
    local context=$1
    local cluster_type=$2
    
    log_info "Checking $cluster_type cluster context: $context"
    
    if ! kubectl config get-contexts "$context" &>/dev/null; then
        log_error "Kubernetes context '$context' not found"
        return 1
    fi
    
    if ! kubectl --context="$context" cluster-info &>/dev/null; then
        log_error "Cannot access cluster with context '$context'"
        return 1
    fi
    
    log_success "$cluster_type cluster context '$context' is accessible"
    return 0
}

# Create namespace if it doesn't exist
create_namespace() {
    local context=$1
    local cluster_type=$2
    
    log_info "Creating namespace '$NAMESPACE' in $cluster_type cluster"
    
    if kubectl --context="$context" get namespace "$NAMESPACE" &>/dev/null; then
        log_info "Namespace '$NAMESPACE' already exists in $cluster_type cluster"
    else
        kubectl --context="$context" create namespace "$NAMESPACE"
        log_success "Created namespace '$NAMESPACE' in $cluster_type cluster"
    fi
}

# Deploy to edge cluster (streamers only)
deploy_edge() {
    log_info "Deploying telemetry streamers to edge cluster"
    
    check_context "$EDGE_CONTEXT" "edge"
    create_namespace "$EDGE_CONTEXT" "edge"
    
    local helm_args=(
        "upgrade" "--install" "telemetry-edge"
        "$HELM_CHART_DIR"
        "--namespace" "$NAMESPACE"
        "--kube-context" "$EDGE_CONTEXT"
        "--values" "$HELM_CHART_DIR/values-edge-cluster.yaml"
        "--set" "externalRedis.host=$REDIS_HOST"
        "--set" "externalRedis.password=$REDIS_PASSWORD"
        "--wait"
        "--timeout" "10m"
    )
    
    if [[ -n "$DRY_RUN" ]]; then
        helm_args+=("$DRY_RUN")
    fi
    
    if [[ -n "$DEBUG" ]]; then
        helm_args+=("$DEBUG")
    fi
    
    log_info "Executing: helm ${helm_args[*]}"
    
    if helm "${helm_args[@]}"; then
        log_success "Successfully deployed streamers to edge cluster"
        
        if [[ -z "$DRY_RUN" ]]; then
            log_info "Waiting for streamers to be ready..."
            kubectl --context="$EDGE_CONTEXT" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=streamer --timeout=300s
            log_success "Streamers are ready in edge cluster"
        fi
    else
        log_error "Failed to deploy streamers to edge cluster"
        return 1
    fi
}

# Deploy to central cluster (collectors, API gateway, database)
deploy_central() {
    log_info "Deploying collectors and API gateway to central cluster"
    
    check_context "$CENTRAL_CONTEXT" "central"
    create_namespace "$CENTRAL_CONTEXT" "central"
    
    local helm_args=(
        "upgrade" "--install" "telemetry-central"
        "$HELM_CHART_DIR"
        "--namespace" "$NAMESPACE"
        "--kube-context" "$CENTRAL_CONTEXT"
        "--values" "$HELM_CHART_DIR/values-central-cluster.yaml"
        "--set" "externalRedis.host=$REDIS_HOST"
        "--set" "externalRedis.password=$REDIS_PASSWORD"
        "--set" "postgresql.auth.password=$DB_PASSWORD"
        "--wait"
        "--timeout" "15m"
    )
    
    if [[ -n "$DRY_RUN" ]]; then
        helm_args+=("$DRY_RUN")
    fi
    
    if [[ -n "$DEBUG" ]]; then
        helm_args+=("$DEBUG")
    fi
    
    log_info "Executing: helm ${helm_args[*]}"
    
    if helm "${helm_args[@]}"; then
        log_success "Successfully deployed central components"
        
        if [[ -z "$DRY_RUN" ]]; then
            log_info "Waiting for central components to be ready..."
            kubectl --context="$CENTRAL_CONTEXT" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=collector --timeout=300s
            kubectl --context="$CENTRAL_CONTEXT" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=api-gateway --timeout=300s
            log_success "Central components are ready"
        fi
    else
        log_error "Failed to deploy central components"
        return 1
    fi
}

# Deploy shared Redis infrastructure
deploy_shared() {
    log_info "Deploying shared Redis infrastructure"
    
    check_context "$SHARED_CONTEXT" "shared"
    
    # Create shared infrastructure namespace
    local shared_namespace="shared-infrastructure"
    if kubectl --context="$SHARED_CONTEXT" get namespace "$shared_namespace" &>/dev/null; then
        log_info "Namespace '$shared_namespace' already exists"
    else
        kubectl --context="$SHARED_CONTEXT" create namespace "$shared_namespace"
        log_success "Created namespace '$shared_namespace'"
    fi
    
    # Deploy Redis using Bitnami chart
    local helm_args=(
        "upgrade" "--install" "redis-cluster"
        "oci://registry-1.docker.io/bitnamicharts/redis-cluster"
        "--namespace" "$shared_namespace"
        "--kube-context" "$SHARED_CONTEXT"
        "--set" "auth.password=$REDIS_PASSWORD"
        "--set" "tls.enabled=true"
        "--set" "tls.authClients=true"
        "--set" "persistence.enabled=true"
        "--set" "persistence.size=50Gi"
        "--set" "redis.resources.requests.memory=1Gi"
        "--set" "redis.resources.requests.cpu=500m"
        "--set" "redis.resources.limits.memory=2Gi"
        "--set" "redis.resources.limits.cpu=1000m"
        "--wait"
        "--timeout" "15m"
    )
    
    if [[ -n "$DRY_RUN" ]]; then
        helm_args+=("$DRY_RUN")
    fi
    
    if [[ -n "$DEBUG" ]]; then
        helm_args+=("$DEBUG")
    fi
    
    log_info "Executing: helm ${helm_args[*]}"
    
    if helm "${helm_args[@]}"; then
        log_success "Successfully deployed Redis cluster"
        
        if [[ -z "$DRY_RUN" ]]; then
            log_info "Waiting for Redis cluster to be ready..."
            kubectl --context="$SHARED_CONTEXT" -n "$shared_namespace" wait --for=condition=ready pod -l app.kubernetes.io/name=redis-cluster --timeout=600s
            log_success "Redis cluster is ready"
            
            # Get Redis service information
            log_info "Redis cluster service information:"
            kubectl --context="$SHARED_CONTEXT" -n "$shared_namespace" get svc -l app.kubernetes.io/name=redis-cluster
        fi
    else
        log_error "Failed to deploy Redis cluster"
        return 1
    fi
}

# Check deployment status
check_status() {
    local overall_status=0
    
    if [[ -n "$EDGE_CONTEXT" ]]; then
        log_info "Checking edge cluster status"
        if kubectl --context="$EDGE_CONTEXT" get namespace "$NAMESPACE" &>/dev/null; then
            echo "Edge Cluster ($EDGE_CONTEXT):"
            kubectl --context="$EDGE_CONTEXT" -n "$NAMESPACE" get pods,svc,hpa 2>/dev/null || {
                log_warning "No resources found in edge cluster"
                overall_status=1
            }
            echo ""
        else
            log_warning "Namespace '$NAMESPACE' not found in edge cluster"
            overall_status=1
        fi
    fi
    
    if [[ -n "$CENTRAL_CONTEXT" ]]; then
        log_info "Checking central cluster status"
        if kubectl --context="$CENTRAL_CONTEXT" get namespace "$NAMESPACE" &>/dev/null; then
            echo "Central Cluster ($CENTRAL_CONTEXT):"
            kubectl --context="$CENTRAL_CONTEXT" -n "$NAMESPACE" get pods,svc,hpa 2>/dev/null || {
                log_warning "No resources found in central cluster"
                overall_status=1
            }
            echo ""
        else
            log_warning "Namespace '$NAMESPACE' not found in central cluster"
            overall_status=1
        fi
    fi
    
    if [[ -n "$SHARED_CONTEXT" ]]; then
        log_info "Checking shared infrastructure status"
        if kubectl --context="$SHARED_CONTEXT" get namespace "shared-infrastructure" &>/dev/null; then
            echo "Shared Infrastructure ($SHARED_CONTEXT):"
            kubectl --context="$SHARED_CONTEXT" -n "shared-infrastructure" get pods,svc 2>/dev/null || {
                log_warning "No resources found in shared infrastructure"
                overall_status=1
            }
            echo ""
        else
            log_warning "Namespace 'shared-infrastructure' not found"
            overall_status=1
        fi
    fi
    
    return $overall_status
}

# Cleanup deployments
cleanup() {
    log_warning "This will delete all telemetry pipeline deployments. Are you sure? (y/N)"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        log_info "Cleanup cancelled"
        return 0
    fi
    
    if [[ -n "$EDGE_CONTEXT" ]]; then
        log_info "Cleaning up edge cluster"
        helm --kube-context="$EDGE_CONTEXT" uninstall telemetry-edge -n "$NAMESPACE" 2>/dev/null || true
        kubectl --context="$EDGE_CONTEXT" delete namespace "$NAMESPACE" --ignore-not-found=true
    fi
    
    if [[ -n "$CENTRAL_CONTEXT" ]]; then
        log_info "Cleaning up central cluster"
        helm --kube-context="$CENTRAL_CONTEXT" uninstall telemetry-central -n "$NAMESPACE" 2>/dev/null || true
        kubectl --context="$CENTRAL_CONTEXT" delete namespace "$NAMESPACE" --ignore-not-found=true
    fi
    
    if [[ -n "$SHARED_CONTEXT" ]]; then
        log_info "Cleaning up shared infrastructure"
        helm --kube-context="$SHARED_CONTEXT" uninstall redis-cluster -n "shared-infrastructure" 2>/dev/null || true
        kubectl --context="$SHARED_CONTEXT" delete namespace "shared-infrastructure" --ignore-not-found=true
    fi
    
    log_success "Cleanup completed"
}

# Main execution
main() {
    if [[ -z "$COMMAND" ]]; then
        log_error "No command specified"
        usage
        exit 1
    fi
    
    validate_params
    
    case $COMMAND in
        deploy-edge)
            deploy_edge
            ;;
        deploy-central)
            deploy_central
            ;;
        deploy-shared)
            deploy_shared
            ;;
        deploy-all)
            log_info "Starting full cross-cluster deployment"
            deploy_shared
            deploy_edge
            deploy_central
            log_success "Full deployment completed successfully"
            ;;
        status)
            check_status
            ;;
        cleanup)
            cleanup
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
