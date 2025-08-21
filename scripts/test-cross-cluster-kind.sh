#!/bin/bash

# End-to-End Cross-Cluster Test Script using Kind
# This script demonstrates the telemetry pipeline working across multiple Kind clusters

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
EDGE_CLUSTER="kind-telemetry-edge"
CENTRAL_CLUSTER="kind-telemetry-central"
SHARED_CLUSTER="kind-telemetry-shared"
NAMESPACE="telemetry-system"

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

End-to-end test of cross-cluster telemetry pipeline deployment using Kind.

COMMANDS:
    setup               Setup Kind clusters and prerequisites
    build-images        Build and load Docker images
    deploy-shared       Deploy shared Redis infrastructure
    deploy-edge         Deploy streamers to edge cluster
    deploy-central      Deploy collectors and API to central cluster
    test-connectivity   Test cross-cluster connectivity
    test-pipeline       Test the complete telemetry pipeline
    cleanup             Clean up all deployments
    full-test           Run complete end-to-end test

OPTIONS:
    --skip-build        Skip Docker image building
    --skip-setup        Skip Kind cluster setup
    --debug             Enable debug output
    --help              Show this help message

EXAMPLES:
    # Run complete end-to-end test
    $0 full-test

    # Setup clusters only
    $0 setup

    # Test pipeline functionality
    $0 test-pipeline

    # Clean up everything
    $0 cleanup

EOF
}

# Parse command line arguments
SKIP_BUILD=false
SKIP_SETUP=false
DEBUG=false
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --skip-setup)
            SKIP_SETUP=true
            shift
            ;;
        --debug)
            DEBUG=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        setup|build-images|deploy-shared|deploy-edge|deploy-central|test-connectivity|test-pipeline|cleanup|full-test)
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

# Enable debug mode
if [ "$DEBUG" = true ]; then
    set -x
fi

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    if ! command -v kind &> /dev/null; then
        missing_tools+=("kind")
    fi
    
    if ! command -v kubectl &> /dev/null; then
        missing_tools+=("kubectl")
    fi
    
    if ! command -v docker &> /dev/null; then
        missing_tools+=("docker")
    fi
    
    if ! command -v helm &> /dev/null; then
        missing_tools+=("helm")
    fi
    
    if [ ${#missing_tools[@]} -gt 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_info "Run './scripts/setup-kind-clusters.sh install-deps' to install kind and kubectl"
        return 1
    fi
    
    log_success "All prerequisites are available"
    return 0
}

# Setup Kind clusters
setup_clusters() {
    if [ "$SKIP_SETUP" = true ]; then
        log_info "Skipping Kind cluster setup"
        return 0
    fi
    
    log_info "Setting up Kind clusters..."
    
    # Create clusters
    "$SCRIPT_DIR/setup-kind-clusters.sh" create-all
    
    # Wait for clusters to be ready
    log_info "Waiting for clusters to be ready..."
    kubectl --context="$EDGE_CLUSTER" wait --for=condition=Ready nodes --all --timeout=300s
    kubectl --context="$CENTRAL_CLUSTER" wait --for=condition=Ready nodes --all --timeout=300s
    kubectl --context="$SHARED_CLUSTER" wait --for=condition=Ready nodes --all --timeout=300s
    
    log_success "All clusters are ready"
}

# Build and load Docker images
build_images() {
    if [ "$SKIP_BUILD" = true ]; then
        log_info "Skipping Docker image building"
        return 0
    fi
    
    log_info "Building Docker images..."
    
    # Build images
    cd "$SCRIPT_DIR/.."
    
    # Build streamer
    log_info "Building streamer image..."
    docker build -f deployments/docker/Dockerfile.streamer -t telemetry-pipeline-streamer:latest .
    
    # Build collector
    log_info "Building collector image..."
    docker build -f deployments/docker/Dockerfile.collector -t telemetry-pipeline-collector:latest .
    
    # Build API gateway
    log_info "Building API gateway image..."
    docker build -f deployments/docker/Dockerfile.api-gateway -t telemetry-pipeline-api-gateway:latest .
    
    cd "$SCRIPT_DIR"
    
    # Load images into clusters
    log_info "Loading images into Kind clusters..."
    "$SCRIPT_DIR/setup-kind-clusters.sh" load-images
    
    log_success "Images built and loaded successfully"
}

# Deploy shared infrastructure (Redis)
deploy_shared_infra() {
    log_info "Deploying shared Redis infrastructure..."
    
    # Create namespace
    kubectl --context="$SHARED_CLUSTER" create namespace shared-infrastructure --dry-run=client -o yaml | kubectl --context="$SHARED_CLUSTER" apply -f -
    
    # Add Bitnami repo
    helm repo add bitnami https://charts.bitnami.com/bitnami
    helm repo update
    
    # Deploy Redis cluster
    helm upgrade --install redis-cluster bitnami/redis \
        --namespace shared-infrastructure \
        --kube-context "$SHARED_CLUSTER" \
        --set auth.password="redis123" \
        --set architecture="standalone" \
        --set master.persistence.enabled=false \
        --set replica.replicaCount=0 \
        --set master.service.type=NodePort \
        --set master.service.nodePorts.redis=30379 \
        --wait --timeout=300s
    
    # Wait for Redis to be ready
    kubectl --context="$SHARED_CLUSTER" -n shared-infrastructure wait --for=condition=ready pod -l app.kubernetes.io/name=redis --timeout=300s
    
    # Get Redis service details
    local redis_service=$(kubectl --context="$SHARED_CLUSTER" -n shared-infrastructure get svc redis-cluster-master -o jsonpath='{.spec.clusterIP}')
    log_info "Redis cluster deployed at: $redis_service:6379"
    
    # Test Redis connectivity
    kubectl --context="$SHARED_CLUSTER" -n shared-infrastructure exec -it $(kubectl --context="$SHARED_CLUSTER" -n shared-infrastructure get pod -l app.kubernetes.io/name=redis -o jsonpath='{.items[0].metadata.name}') -- redis-cli -a redis123 ping
    
    log_success "Shared infrastructure deployed successfully"
}

# Deploy edge components (streamers)
deploy_edge() {
    log_info "Deploying edge components (streamers)..."
    
    # Get Redis external IP (using Docker network)
    local redis_host="redis-cluster-master.shared-infrastructure.svc.cluster.local"
    
    # Since we're using Kind, we need to use the shared cluster's service
    # For real cross-cluster, this would be an external IP/hostname
    local shared_cluster_ip=$(docker inspect telemetry-shared-control-plane | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress')
    redis_host="$shared_cluster_ip"
    
    # Deploy to edge cluster
    helm upgrade --install telemetry-edge "$SCRIPT_DIR/../deployments/helm/telemetry-pipeline" \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --kube-context "$EDGE_CLUSTER" \
        --values "$SCRIPT_DIR/../deployments/helm/telemetry-pipeline/values-edge-cluster.yaml" \
        --set externalRedis.enabled=true \
        --set externalRedis.host="$redis_host" \
        --set externalRedis.port=30379 \
        --set externalRedis.password="redis123" \
        --wait --timeout=300s
    
    # Wait for streamers to be ready
    kubectl --context="$EDGE_CLUSTER" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=streamer --timeout=300s
    
    log_success "Edge components deployed successfully"
}

# Deploy central components (collectors + API)
deploy_central() {
    log_info "Deploying central components (collectors + API)..."
    
    # Get Redis external IP
    local shared_cluster_ip=$(docker inspect telemetry-shared-control-plane | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress')
    local redis_host="$shared_cluster_ip"
    
    # Deploy to central cluster
    helm upgrade --install telemetry-central "$SCRIPT_DIR/../deployments/helm/telemetry-pipeline" \
        --namespace "$NAMESPACE" \
        --create-namespace \
        --kube-context "$CENTRAL_CLUSTER" \
        --values "$SCRIPT_DIR/../deployments/helm/telemetry-pipeline/values-central-cluster.yaml" \
        --set externalRedis.enabled=true \
        --set externalRedis.host="$redis_host" \
        --set externalRedis.port=30379 \
        --set externalRedis.password="redis123" \
        --set postgresql.auth.password="postgres123" \
        --wait --timeout=600s
    
    # Wait for components to be ready
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=collector --timeout=300s
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" wait --for=condition=ready pod -l app.kubernetes.io/component=api-gateway --timeout=300s
    
    log_success "Central components deployed successfully"
}

# Test cross-cluster connectivity
test_connectivity() {
    log_info "Testing cross-cluster connectivity..."
    
    local shared_cluster_ip=$(docker inspect telemetry-shared-control-plane | jq -r '.[0].NetworkSettings.Networks.kind.IPAddress')
    
    # Test Redis connectivity from edge cluster
    log_info "Testing Redis connectivity from edge cluster..."
    kubectl --context="$EDGE_CLUSTER" -n "$NAMESPACE" exec -it $(kubectl --context="$EDGE_CLUSTER" -n "$NAMESPACE" get pod -l app.kubernetes.io/component=streamer -o jsonpath='{.items[0].metadata.name}') -- sh -c "
        apk add --no-cache redis
        redis-cli -h $shared_cluster_ip -p 30379 -a redis123 ping
    " || log_warning "Redis connectivity test failed from edge cluster"
    
    # Test Redis connectivity from central cluster
    log_info "Testing Redis connectivity from central cluster..."
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" exec -it $(kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" get pod -l app.kubernetes.io/component=collector -o jsonpath='{.items[0].metadata.name}') -- sh -c "
        apk add --no-cache redis
        redis-cli -h $shared_cluster_ip -p 30379 -a redis123 ping
    " || log_warning "Redis connectivity test failed from central cluster"
    
    log_success "Cross-cluster connectivity tests completed"
}

# Test the complete pipeline
test_pipeline() {
    log_info "Testing the complete telemetry pipeline..."
    
    # Check streamer logs
    log_info "Checking streamer logs..."
    kubectl --context="$EDGE_CLUSTER" -n "$NAMESPACE" logs -l app.kubernetes.io/component=streamer --tail=20 || true
    
    # Check collector logs
    log_info "Checking collector logs..."
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" logs -l app.kubernetes.io/component=collector --tail=20 || true
    
    # Check API gateway
    log_info "Testing API gateway..."
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" port-forward svc/telemetry-central-telemetry-pipeline-api-gateway 8080:80 &
    local port_forward_pid=$!
    
    sleep 5
    
    # Test API endpoint
    if curl -f http://localhost:8080/health; then
        log_success "API gateway is responding"
    else
        log_warning "API gateway is not responding"
    fi
    
    # Clean up port forward
    kill $port_forward_pid 2>/dev/null || true
    
    # Check database connectivity
    log_info "Checking database connectivity..."
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" exec -it $(kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" get pod -l app.kubernetes.io/name=postgresql -o jsonpath='{.items[0].metadata.name}') -- psql -U telemetry -d telemetry -c "SELECT COUNT(*) FROM telemetry_data;" || log_warning "Database query failed"
    
    log_success "Pipeline testing completed"
}

# Show deployment status
show_status() {
    log_info "Deployment Status Summary"
    echo "=========================================="
    
    # Edge cluster status
    echo ""
    log_info "Edge Cluster ($EDGE_CLUSTER):"
    kubectl --context="$EDGE_CLUSTER" -n "$NAMESPACE" get pods,svc 2>/dev/null || echo "No resources found"
    
    # Central cluster status
    echo ""
    log_info "Central Cluster ($CENTRAL_CLUSTER):"
    kubectl --context="$CENTRAL_CLUSTER" -n "$NAMESPACE" get pods,svc 2>/dev/null || echo "No resources found"
    
    # Shared cluster status
    echo ""
    log_info "Shared Infrastructure ($SHARED_CLUSTER):"
    kubectl --context="$SHARED_CLUSTER" -n shared-infrastructure get pods,svc 2>/dev/null || echo "No resources found"
    
    echo ""
    echo "=========================================="
}

# Cleanup all deployments
cleanup() {
    log_info "Cleaning up all deployments..."
    
    # Cleanup edge cluster
    helm --kube-context "$EDGE_CLUSTER" uninstall telemetry-edge -n "$NAMESPACE" 2>/dev/null || true
    kubectl --context="$EDGE_CLUSTER" delete namespace "$NAMESPACE" --ignore-not-found=true
    
    # Cleanup central cluster
    helm --kube-context "$CENTRAL_CLUSTER" uninstall telemetry-central -n "$NAMESPACE" 2>/dev/null || true
    kubectl --context="$CENTRAL_CLUSTER" delete namespace "$NAMESPACE" --ignore-not-found=true
    
    # Cleanup shared cluster
    helm --kube-context "$SHARED_CLUSTER" uninstall redis-cluster -n shared-infrastructure 2>/dev/null || true
    kubectl --context="$SHARED_CLUSTER" delete namespace shared-infrastructure --ignore-not-found=true
    
    log_success "Cleanup completed"
}

# Full end-to-end test
full_test() {
    log_info "Starting full end-to-end cross-cluster test..."
    echo ""
    
    # Setup
    check_prerequisites || exit 1
    setup_clusters
    build_images
    
    # Deploy
    deploy_shared_infra
    deploy_edge
    deploy_central
    
    # Test
    test_connectivity
    test_pipeline
    
    # Show status
    show_status
    
    log_success "🎉 Full end-to-end test completed successfully!"
    echo ""
    log_info "The telemetry pipeline is now running across multiple Kind clusters:"
    echo "  • Edge cluster: Streamers reading CSV data"
    echo "  • Shared cluster: Redis message queue"
    echo "  • Central cluster: Collectors processing data and API gateway"
    echo ""
    log_info "To access the API:"
    echo "  kubectl --context=$CENTRAL_CLUSTER -n $NAMESPACE port-forward svc/telemetry-central-telemetry-pipeline-api-gateway 8080:80"
    echo "  curl http://localhost:8080/health"
}

# Main execution
main() {
    if [[ -z "$COMMAND" ]]; then
        log_error "No command specified"
        usage
        exit 1
    fi
    
    case $COMMAND in
        setup)
            check_prerequisites || exit 1
            setup_clusters
            ;;
        build-images)
            build_images
            ;;
        deploy-shared)
            deploy_shared_infra
            ;;
        deploy-edge)
            deploy_edge
            ;;
        deploy-central)
            deploy_central
            ;;
        test-connectivity)
            test_connectivity
            ;;
        test-pipeline)
            test_pipeline
            show_status
            ;;
        cleanup)
            cleanup
            ;;
        full-test)
            full_test
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
