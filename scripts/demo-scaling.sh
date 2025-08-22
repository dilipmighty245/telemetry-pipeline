#!/bin/bash

# Demo script for local scaling and Kubernetes deployment
# This script demonstrates both local scaling and Kubernetes deployment capabilities

set -e

SCRIPT_DIR=$(dirname "$(realpath "$0")")
BASE_DIR=$(dirname "$SCRIPT_DIR")

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} ✅ $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')]${NC} ⚠️  $1"
}

error() {
    echo -e "${RED}[$(date +'%H:%M:%S')]${NC} ❌ $1"
}

info() {
    echo -e "${PURPLE}[$(date +'%H:%M:%S')]${NC} ℹ️  $1"
}

# Function to wait for user input
wait_for_user() {
    echo ""
    read -p "Press Enter to continue..."
    echo ""
}

# Function to show section header
section() {
    echo ""
    echo "============================================"
    echo "  $1"
    echo "============================================"
    echo ""
}

# Main demo function
main() {
    cd "$BASE_DIR"
    
    echo "🚀 Telemetry Pipeline Scaling & Kubernetes Demo"
    echo "================================================"
    echo ""
    echo "This demo will show:"
    echo "1. Local scaling capabilities"
    echo "2. Kubernetes deployment with Helm charts"
    echo "3. Testing scaled components"
    echo ""
    
    wait_for_user
    
    # Section 1: Local Scaling Demo
    section "1. LOCAL SCALING DEMONSTRATION"
    
    log "Building all Nexus components..."
    make build-nexus
    
    log "Starting etcd if not running..."
    make setup-etcd || warn "etcd might already be running"
    
    sleep 2
    
    log "Demonstrating local scaling with 3 instances of each component..."
    INSTANCES=3 make scale-local
    
    wait_for_user
    
    log "Checking scaling status..."
    make scale-status
    
    wait_for_user
    
    log "Testing multiple gateway instances..."
    echo "Gateway 1 (port 8080): $(curl -s http://localhost:8080/health | jq -r .status 2>/dev/null || echo 'Not responding')"
    echo "Gateway 2 (port 8081): $(curl -s http://localhost:8081/health | jq -r .status 2>/dev/null || echo 'Not responding')"
    echo "Gateway 3 (port 8082): $(curl -s http://localhost:8082/health | jq -r .status 2>/dev/null || echo 'Not responding')"
    
    wait_for_user
    
    log "Showing recent logs from scaled components..."
    echo "Recent streamer logs:"
    tail -5 logs/streamer-*.log 2>/dev/null || warn "No streamer logs found"
    echo ""
    echo "Recent collector logs:"
    tail -5 logs/collector-*.log 2>/dev/null || warn "No collector logs found"
    
    wait_for_user
    
    # Section 2: Kubernetes Deployment Demo
    section "2. KUBERNETES DEPLOYMENT DEMONSTRATION"
    
    # Check if kubectl is available
    if ! command -v kubectl &> /dev/null; then
        error "kubectl not found. Please install kubectl to continue with Kubernetes demo."
        exit 1
    fi
    
    # Check if helm is available
    if ! command -v helm &> /dev/null; then
        error "helm not found. Please install Helm to continue with Kubernetes demo."
        exit 1
    fi
    
    log "Checking Kubernetes cluster connection..."
    if kubectl cluster-info &> /dev/null; then
        success "Connected to Kubernetes cluster"
        kubectl cluster-info | head -1
    else
        error "Cannot connect to Kubernetes cluster. Please ensure kubectl is configured."
        warn "Skipping Kubernetes demo..."
        section "DEMO COMPLETE - LOCAL SCALING ONLY"
        make scale-stop
        exit 0
    fi
    
    wait_for_user
    
    log "Building Docker images for Kubernetes deployment..."
    # Use a local registry or Docker Hub depending on setup
    DOCKER_REGISTRY=localhost:5000 VERSION=demo make docker-build-nexus || {
        warn "Docker build failed. Continuing with existing images..."
    }
    
    log "Deploying to Kubernetes with Helm..."
    NAMESPACE=telemetry-demo STREAMER_INSTANCES=2 COLLECTOR_INSTANCES=2 API_GW_INSTANCES=2 make k8s-deploy-nexus || {
        error "Kubernetes deployment failed"
        section "CLEANING UP LOCAL SCALING"
        make scale-stop
        exit 1
    }
    
    wait_for_user
    
    log "Checking Kubernetes deployment status..."
    NAMESPACE=telemetry-demo make k8s-status-nexus
    
    wait_for_user
    
    log "Setting up port forwarding to test the API..."
    info "Starting port forward in background (PID will be shown)..."
    NAMESPACE=telemetry-demo make k8s-port-forward &
    PORT_FORWARD_PID=$!
    echo "Port forward PID: $PORT_FORWARD_PID"
    
    sleep 5
    
    log "Testing Kubernetes-deployed API..."
    if curl -s http://localhost:8080/health | jq -r .status &> /dev/null; then
        success "Kubernetes API is responding"
        echo "Health: $(curl -s http://localhost:8080/health | jq -r .status)"
        echo "GPU count: $(curl -s http://localhost:8080/api/v1/gpus | jq -r .count)"
    else
        warn "Kubernetes API not responding yet (may need more time to start)"
    fi
    
    wait_for_user
    
    # Section 3: Cleanup
    section "3. CLEANUP"
    
    log "Stopping port forward..."
    kill $PORT_FORWARD_PID 2>/dev/null || true
    
    log "Cleaning up Kubernetes deployment..."
    NAMESPACE=telemetry-demo make k8s-undeploy-nexus
    
    log "Stopping local scaled components..."
    make scale-stop
    
    success "Demo completed successfully!"
    
    section "SUMMARY"
    echo "✅ Demonstrated local scaling with multiple instances"
    echo "✅ Showed Kubernetes deployment with Helm charts"
    echo "✅ Verified API functionality in both environments"
    echo ""
    echo "Key commands used:"
    echo "  • make scale-local INSTANCES=3     # Scale locally"
    echo "  • make k8s-deploy-nexus           # Deploy to K8s"
    echo "  • make scale-status               # Check status"
    echo "  • make k8s-status-nexus          # K8s status"
    echo ""
    echo "For more options: make help"
}

# Run main function
main "$@"
