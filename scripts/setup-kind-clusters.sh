#!/bin/bash

# Kind Multi-Cluster Setup Script for Telemetry Pipeline
# Creates multiple Kind clusters to test cross-cluster deployment scenarios

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_VERSION="v0.20.0"
KUBECTL_VERSION="v1.28.0"

# Cluster configurations
EDGE_CLUSTER_NAME="telemetry-edge"
CENTRAL_CLUSTER_NAME="telemetry-central"
SHARED_CLUSTER_NAME="telemetry-shared"

# Network configuration
EDGE_CLUSTER_SUBNET="172.18.0.0/16"
CENTRAL_CLUSTER_SUBNET="172.19.0.0/16"
SHARED_CLUSTER_SUBNET="172.20.0.0/16"

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

Create and manage Kind clusters for telemetry pipeline testing.

COMMANDS:
    create-all          Create all clusters (edge, central, shared)
    create-edge         Create edge cluster only
    create-central      Create central cluster only
    create-shared       Create shared infrastructure cluster only
    delete-all          Delete all clusters
    delete-edge         Delete edge cluster only
    delete-central      Delete central cluster only
    delete-shared       Delete shared infrastructure cluster only
    status              Show status of all clusters
    load-images         Load Docker images into all clusters
    setup-networking    Configure cross-cluster networking
    install-deps        Install required dependencies (kind, kubectl)

OPTIONS:
    --edge-nodes NUM        Number of nodes for edge cluster (default: 2)
    --central-nodes NUM     Number of nodes for central cluster (default: 3)
    --shared-nodes NUM      Number of nodes for shared cluster (default: 2)
    --registry-port PORT    Local registry port (default: 5001)
    --skip-registry        Skip local registry setup
    --help                  Show this help message

EXAMPLES:
    # Create all clusters with default configuration
    $0 create-all

    # Create edge cluster with 3 nodes
    $0 --edge-nodes 3 create-edge

    # Delete all clusters
    $0 delete-all

    # Check cluster status
    $0 status

    # Load images and test deployment
    $0 load-images

EOF
}

# Parse command line arguments
EDGE_NODES=2
CENTRAL_NODES=3
SHARED_NODES=2
REGISTRY_PORT=5001
SKIP_REGISTRY=false
COMMAND=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --edge-nodes)
            EDGE_NODES="$2"
            shift 2
            ;;
        --central-nodes)
            CENTRAL_NODES="$2"
            shift 2
            ;;
        --shared-nodes)
            SHARED_NODES="$2"
            shift 2
            ;;
        --registry-port)
            REGISTRY_PORT="$2"
            shift 2
            ;;
        --skip-registry)
            SKIP_REGISTRY=true
            shift
            ;;
        --help)
            usage
            exit 0
            ;;
        create-all|create-edge|create-central|create-shared|delete-all|delete-edge|delete-central|delete-shared|status|load-images|setup-networking|install-deps)
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

# Check if Kind is installed
check_kind() {
    if ! command -v kind &> /dev/null; then
        log_error "Kind is not installed. Run '$0 install-deps' to install it."
        return 1
    fi
    
    local version=$(kind version 2>/dev/null | grep -o 'v[0-9]\+\.[0-9]\+\.[0-9]\+' | head -1)
    log_info "Kind version: $version"
    return 0
}

# Check if kubectl is installed
check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Run '$0 install-deps' to install it."
        return 1
    fi
    
    local version=$(kubectl version --client -o yaml 2>/dev/null | grep gitVersion | cut -d'"' -f4)
    log_info "kubectl version: $version"
    return 0
}

# Install dependencies
install_deps() {
    log_info "Installing Kind and kubectl..."
    
    # Detect OS
    local os=""
    case "$(uname -s)" in
        Darwin*) os="darwin" ;;
        Linux*) os="linux" ;;
        *) log_error "Unsupported OS: $(uname -s)"; exit 1 ;;
    esac
    
    # Detect architecture
    local arch=""
    case "$(uname -m)" in
        x86_64) arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *) log_error "Unsupported architecture: $(uname -m)"; exit 1 ;;
    esac
    
    # Install Kind
    if ! command -v kind &> /dev/null; then
        log_info "Installing Kind ${KIND_VERSION}..."
        curl -Lo ./kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-${os}-${arch}
        chmod +x ./kind
        sudo mv ./kind /usr/local/bin/kind
        log_success "Kind installed successfully"
    else
        log_info "Kind is already installed"
    fi
    
    # Install kubectl
    if ! command -v kubectl &> /dev/null; then
        log_info "Installing kubectl ${KUBECTL_VERSION}..."
        curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/${os}/${arch}/kubectl"
        chmod +x kubectl
        sudo mv kubectl /usr/local/bin/kubectl
        log_success "kubectl installed successfully"
    else
        log_info "kubectl is already installed"
    fi
}

# Create Kind cluster configuration
create_cluster_config() {
    local cluster_name=$1
    local node_count=$2
    local subnet=$3
    local config_file="/tmp/${cluster_name}-config.yaml"
    
    cat > "$config_file" << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${cluster_name}
networking:
  ipFamily: ipv4
  apiServerAddress: "127.0.0.1"
  apiServerPort: 0
  podSubnet: "${subnet}"
  serviceSubnet: "10.96.0.0/12"
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "cluster-role=control-plane,cluster-name=${cluster_name}"
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
  - containerPort: 30443
    hostPort: 8443
    protocol: TCP
EOF

    # Add worker nodes
    for ((i=1; i<node_count; i++)); do
        cat >> "$config_file" << EOF
- role: worker
  kubeadmConfigPatches:
  - |
    kind: JoinConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "cluster-role=worker,cluster-name=${cluster_name}"
EOF
    done
    
    echo "$config_file"
}

# Create local registry
create_registry() {
    if [ "$SKIP_REGISTRY" = true ]; then
        log_info "Skipping local registry setup"
        return 0
    fi
    
    local registry_name="kind-registry"
    local registry_port=$REGISTRY_PORT
    
    # Check if registry already exists
    if docker ps -a --format '{{.Names}}' | grep -q "^${registry_name}$"; then
        log_info "Local registry already exists"
        return 0
    fi
    
    log_info "Creating local Docker registry on port ${registry_port}..."
    
    docker run -d --restart=always -p "127.0.0.1:${registry_port}:5000" --name "${registry_name}" registry:2
    
    # Wait for registry to be ready
    sleep 5
    
    log_success "Local registry created at localhost:${registry_port}"
}

# Connect cluster to registry
connect_registry() {
    local cluster_name=$1
    local registry_name="kind-registry"
    local registry_port=$REGISTRY_PORT
    
    if [ "$SKIP_REGISTRY" = true ]; then
        return 0
    fi
    
    log_info "Connecting ${cluster_name} to local registry..."
    
    # Connect the registry to the cluster network if not already connected
    if ! docker network ls | grep -q "kind"; then
        docker network create kind
    fi
    
    docker network connect "kind" "${registry_name}" 2>/dev/null || true
    
    # Document the local registry
    kubectl --context="kind-${cluster_name}" apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: local-registry-hosting
  namespace: kube-public
data:
  localRegistryHosting.v1: |
    host: "localhost:${registry_port}"
    help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

# Create a single cluster
create_cluster() {
    local cluster_name=$1
    local node_count=$2
    local subnet=$3
    
    log_info "Creating ${cluster_name} cluster with ${node_count} nodes..."
    
    # Check if cluster already exists
    if kind get clusters | grep -q "^${cluster_name}$"; then
        log_warning "Cluster ${cluster_name} already exists"
        return 0
    fi
    
    local config_file=$(create_cluster_config "$cluster_name" "$node_count" "$subnet")
    
    # Create the cluster
    kind create cluster --config "$config_file"
    
    # Wait for cluster to be ready
    kubectl --context="kind-${cluster_name}" wait --for=condition=Ready nodes --all --timeout=300s
    
    # Connect to registry
    connect_registry "$cluster_name"
    
    # Label nodes with cluster type
    local cluster_type=""
    case $cluster_name in
        *edge*) cluster_type="edge" ;;
        *central*) cluster_type="central" ;;
        *shared*) cluster_type="shared" ;;
    esac
    
    kubectl --context="kind-${cluster_name}" label nodes --all node-type="$cluster_type" --overwrite
    
    # Clean up config file
    rm -f "$config_file"
    
    log_success "Cluster ${cluster_name} created successfully"
}

# Delete a single cluster
delete_cluster() {
    local cluster_name=$1
    
    log_info "Deleting ${cluster_name} cluster..."
    
    if kind get clusters | grep -q "^${cluster_name}$"; then
        kind delete cluster --name "$cluster_name"
        log_success "Cluster ${cluster_name} deleted successfully"
    else
        log_warning "Cluster ${cluster_name} does not exist"
    fi
}

# Show cluster status
show_status() {
    log_info "Checking Kind clusters status..."
    echo ""
    
    local clusters=$(kind get clusters 2>/dev/null || true)
    
    if [ -z "$clusters" ]; then
        log_warning "No Kind clusters found"
        return 0
    fi
    
    echo "Found clusters:"
    for cluster in $clusters; do
        echo "  - $cluster"
        
        # Check if cluster is accessible
        if kubectl --context="kind-${cluster}" cluster-info &>/dev/null; then
            local nodes=$(kubectl --context="kind-${cluster}" get nodes --no-headers | wc -l)
            local ready_nodes=$(kubectl --context="kind-${cluster}" get nodes --no-headers | grep -c " Ready " || echo "0")
            echo "    Status: Ready (${ready_nodes}/${nodes} nodes)"
            
            # Show node details
            kubectl --context="kind-${cluster}" get nodes -o wide --no-headers | while read line; do
                local node_name=$(echo $line | awk '{print $1}')
                local status=$(echo $line | awk '{print $2}')
                local role=$(echo $line | awk '{print $3}')
                echo "      ${node_name}: ${status} (${role})"
            done
        else
            echo "    Status: Not accessible"
        fi
        echo ""
    done
    
    # Check registry status
    if [ "$SKIP_REGISTRY" != true ]; then
        if docker ps --format '{{.Names}}' | grep -q "kind-registry"; then
            echo "Local registry: Running on port ${REGISTRY_PORT}"
        else
            echo "Local registry: Not running"
        fi
    fi
}

# Load Docker images into clusters
load_images() {
    log_info "Loading Docker images into Kind clusters..."
    
    # Build images if they don't exist
    if [ -f "$SCRIPT_DIR/../Makefile" ]; then
        log_info "Building Docker images..."
        cd "$SCRIPT_DIR/.."
        make docker-build-all || {
            log_warning "Failed to build images with make, trying individual builds..."
            docker build -f deployments/docker/Dockerfile.streamer -t telemetry-pipeline-streamer:latest .
            docker build -f deployments/docker/Dockerfile.collector -t telemetry-pipeline-collector:latest .
            docker build -f deployments/docker/Dockerfile.api-gateway -t telemetry-pipeline-api-gateway:latest .
        }
        cd "$SCRIPT_DIR"
    fi
    
    # List of images to load
    local images=(
        "telemetry-pipeline-streamer:latest"
        "telemetry-pipeline-collector:latest"
        "telemetry-pipeline-api-gateway:latest"
    )
    
    # Get all kind clusters
    local clusters=$(kind get clusters 2>/dev/null || true)
    
    for cluster in $clusters; do
        log_info "Loading images into ${cluster}..."
        for image in "${images[@]}"; do
            if docker images --format "{{.Repository}}:{{.Tag}}" | grep -q "^${image}$"; then
                kind load docker-image "$image" --name "$cluster"
                log_success "Loaded ${image} into ${cluster}"
            else
                log_warning "Image ${image} not found locally"
            fi
        done
    done
}

# Setup cross-cluster networking
setup_networking() {
    log_info "Setting up cross-cluster networking..."
    
    # This is a placeholder for more advanced networking setup
    # For basic Kind clusters, they can communicate through the host network
    
    local clusters=$(kind get clusters 2>/dev/null || true)
    
    for cluster in $clusters; do
        # Add cluster info ConfigMap
        kubectl --context="kind-${cluster}" apply -f - << EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-info
  namespace: kube-system
data:
  cluster-name: "${cluster}"
  cluster-type: "$(echo $cluster | sed 's/telemetry-//')"
EOF
    done
    
    log_success "Cross-cluster networking configured"
}

# Main execution
main() {
    if [[ -z "$COMMAND" ]]; then
        log_error "No command specified"
        usage
        exit 1
    fi
    
    case $COMMAND in
        install-deps)
            install_deps
            ;;
        create-all)
            check_kind || exit 1
            check_kubectl || exit 1
            create_registry
            create_cluster "$EDGE_CLUSTER_NAME" "$EDGE_NODES" "$EDGE_CLUSTER_SUBNET"
            create_cluster "$CENTRAL_CLUSTER_NAME" "$CENTRAL_NODES" "$CENTRAL_CLUSTER_SUBNET"
            create_cluster "$SHARED_CLUSTER_NAME" "$SHARED_NODES" "$SHARED_CLUSTER_SUBNET"
            setup_networking
            show_status
            log_success "All clusters created successfully!"
            echo ""
            log_info "Next steps:"
            echo "  1. Load images: $0 load-images"
            echo "  2. Deploy shared infrastructure: ./deploy-cross-cluster.sh --shared-context kind-${SHARED_CLUSTER_NAME} deploy-shared"
            echo "  3. Deploy edge components: ./deploy-cross-cluster.sh --edge-context kind-${EDGE_CLUSTER_NAME} deploy-edge"
            echo "  4. Deploy central components: ./deploy-cross-cluster.sh --central-context kind-${CENTRAL_CLUSTER_NAME} deploy-central"
            ;;
        create-edge)
            check_kind || exit 1
            check_kubectl || exit 1
            create_registry
            create_cluster "$EDGE_CLUSTER_NAME" "$EDGE_NODES" "$EDGE_CLUSTER_SUBNET"
            connect_registry "$EDGE_CLUSTER_NAME"
            ;;
        create-central)
            check_kind || exit 1
            check_kubectl || exit 1
            create_registry
            create_cluster "$CENTRAL_CLUSTER_NAME" "$CENTRAL_NODES" "$CENTRAL_CLUSTER_SUBNET"
            connect_registry "$CENTRAL_CLUSTER_NAME"
            ;;
        create-shared)
            check_kind || exit 1
            check_kubectl || exit 1
            create_registry
            create_cluster "$SHARED_CLUSTER_NAME" "$SHARED_NODES" "$SHARED_CLUSTER_SUBNET"
            connect_registry "$SHARED_CLUSTER_NAME"
            ;;
        delete-all)
            delete_cluster "$EDGE_CLUSTER_NAME"
            delete_cluster "$CENTRAL_CLUSTER_NAME"
            delete_cluster "$SHARED_CLUSTER_NAME"
            if [ "$SKIP_REGISTRY" != true ]; then
                docker stop kind-registry 2>/dev/null || true
                docker rm kind-registry 2>/dev/null || true
            fi
            log_success "All clusters deleted!"
            ;;
        delete-edge)
            delete_cluster "$EDGE_CLUSTER_NAME"
            ;;
        delete-central)
            delete_cluster "$CENTRAL_CLUSTER_NAME"
            ;;
        delete-shared)
            delete_cluster "$SHARED_CLUSTER_NAME"
            ;;
        status)
            show_status
            ;;
        load-images)
            load_images
            ;;
        setup-networking)
            setup_networking
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
