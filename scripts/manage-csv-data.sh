#!/bin/bash

# CSV Data Management Script for Telemetry Pipeline
# This script helps manage CSV data in Kubernetes for the telemetry pipeline

set -e

NAMESPACE=${NAMESPACE:-telemetry-system}
CONFIGMAP_NAME=${CONFIGMAP_NAME:-telemetry-csv-data}
PVC_NAME=${PVC_NAME:-telemetry-csv-pvc}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print usage
print_usage() {
    echo "CSV Data Management Script for Telemetry Pipeline"
    echo ""
    echo "Usage: $0 <command> [options]"
    echo ""
    echo "Commands:"
    echo "  create-configmap <csv_file>     Create ConfigMap from CSV file"
    echo "  update-configmap <csv_file>     Update existing ConfigMap with new CSV data"
    echo "  create-pvc [size]               Create PVC for CSV storage (default: 1Gi)"
    echo "  upload-to-pvc <csv_file>        Upload CSV file to PVC"
    echo "  list-csv-sources                List all CSV data sources in namespace"
    echo "  validate-csv <csv_file>         Validate CSV file format"
    echo "  deploy-with-configmap <csv_file> Complete workflow: create ConfigMap and deploy"
    echo "  deploy-with-pvc <csv_file>      Complete workflow: create PVC, upload CSV, and deploy"
    echo "  deploy-with-url <url>           Deploy with URL-based CSV source"
    echo ""
    echo "Environment Variables:"
    echo "  NAMESPACE      Kubernetes namespace (default: telemetry-system)"
    echo "  CONFIGMAP_NAME ConfigMap name (default: telemetry-csv-data)"
    echo "  PVC_NAME       PVC name (default: telemetry-csv-pvc)"
    echo ""
    echo "Examples:"
    echo "  $0 create-configmap ./my_telemetry_data.csv"
    echo "  $0 create-pvc 5Gi"
    echo "  $0 upload-to-pvc ./my_telemetry_data.csv"
    echo "  $0 deploy-with-url https://example.com/data.csv"
}

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Check if kubectl is available
check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed or not in PATH"
        exit 1
    fi
}

# Check if namespace exists
check_namespace() {
    if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
        log_warn "Namespace '$NAMESPACE' does not exist. Creating it..."
        kubectl create namespace "$NAMESPACE"
        log_info "Namespace '$NAMESPACE' created"
    fi
}

# Validate CSV file
validate_csv() {
    local csv_file="$1"
    
    if [[ ! -f "$csv_file" ]]; then
        log_error "CSV file '$csv_file' does not exist"
        return 1
    fi
    
    # Check if file is not empty
    if [[ ! -s "$csv_file" ]]; then
        log_error "CSV file '$csv_file' is empty"
        return 1
    fi
    
    # Check if file has expected headers (basic validation)
    local first_line=$(head -n 1 "$csv_file")
    if [[ ! "$first_line" =~ timestamp.*metric_name.*gpu_id ]]; then
        log_warn "CSV file may not have expected headers. Expected: timestamp, metric_name, gpu_id, ..."
        log_warn "Found: $first_line"
        read -p "Continue anyway? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            return 1
        fi
    fi
    
    log_info "CSV file '$csv_file' validation passed"
    return 0
}

# Create ConfigMap from CSV file
create_configmap() {
    local csv_file="$1"
    
    log_step "Creating ConfigMap '$CONFIGMAP_NAME' from CSV file '$csv_file'"
    
    validate_csv "$csv_file" || exit 1
    check_namespace
    
    # Delete existing ConfigMap if it exists
    if kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_warn "ConfigMap '$CONFIGMAP_NAME' already exists. Deleting it..."
        kubectl delete configmap "$CONFIGMAP_NAME" -n "$NAMESPACE"
    fi
    
    # Create new ConfigMap
    kubectl create configmap "$CONFIGMAP_NAME" \
        --from-file=telemetry.csv="$csv_file" \
        -n "$NAMESPACE"
    
    log_info "ConfigMap '$CONFIGMAP_NAME' created successfully"
    
    # Show ConfigMap info
    kubectl describe configmap "$CONFIGMAP_NAME" -n "$NAMESPACE"
}

# Update existing ConfigMap
update_configmap() {
    local csv_file="$1"
    
    log_step "Updating ConfigMap '$CONFIGMAP_NAME' with CSV file '$csv_file'"
    
    validate_csv "$csv_file" || exit 1
    
    # Check if ConfigMap exists
    if ! kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_error "ConfigMap '$CONFIGMAP_NAME' does not exist. Use 'create-configmap' first."
        exit 1
    fi
    
    # Update ConfigMap
    kubectl create configmap "$CONFIGMAP_NAME" \
        --from-file=telemetry.csv="$csv_file" \
        -n "$NAMESPACE" \
        --dry-run=client -o yaml | kubectl replace -f -
    
    log_info "ConfigMap '$CONFIGMAP_NAME' updated successfully"
}

# Create PVC for CSV storage
create_pvc() {
    local size="${1:-1Gi}"
    
    log_step "Creating PVC '$PVC_NAME' with size '$size'"
    
    check_namespace
    
    # Check if PVC already exists
    if kubectl get pvc "$PVC_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_warn "PVC '$PVC_NAME' already exists"
        return 0
    fi
    
    # Create PVC
    kubectl apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: $PVC_NAME
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: telemetry-pipeline
    app.kubernetes.io/component: csv-storage
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: $size
EOF
    
    log_info "PVC '$PVC_NAME' created successfully"
    
    # Wait for PVC to be bound
    log_info "Waiting for PVC to be bound..."
    kubectl wait --for=condition=Bound pvc/"$PVC_NAME" -n "$NAMESPACE" --timeout=60s
    
    kubectl describe pvc "$PVC_NAME" -n "$NAMESPACE"
}

# Upload CSV file to PVC
upload_to_pvc() {
    local csv_file="$1"
    
    log_step "Uploading CSV file '$csv_file' to PVC '$PVC_NAME'"
    
    validate_csv "$csv_file" || exit 1
    
    # Check if PVC exists
    if ! kubectl get pvc "$PVC_NAME" -n "$NAMESPACE" &> /dev/null; then
        log_error "PVC '$PVC_NAME' does not exist. Use 'create-pvc' first."
        exit 1
    fi
    
    # Create temporary pod to upload file
    local temp_pod="csv-uploader-$(date +%s)"
    
    log_info "Creating temporary pod '$temp_pod' to upload CSV file..."
    
    kubectl run "$temp_pod" \
        --image=busybox \
        --rm -i --restart=Never \
        --overrides="{
          \"spec\": {
            \"containers\": [{
              \"name\": \"$temp_pod\",
              \"image\": \"busybox\",
              \"command\": [\"sleep\", \"300\"],
              \"volumeMounts\": [{
                \"mountPath\": \"/data\",
                \"name\": \"csv-volume\"
              }]
            }],
            \"volumes\": [{
              \"name\": \"csv-volume\",
              \"persistentVolumeClaim\": {
                \"claimName\": \"$PVC_NAME\"
              }
            }]
          }
        }" \
        -n "$NAMESPACE" &
    
    # Wait for pod to be ready
    sleep 5
    kubectl wait --for=condition=Ready pod/"$temp_pod" -n "$NAMESPACE" --timeout=60s
    
    # Copy file to pod
    log_info "Copying CSV file to PVC..."
    kubectl cp "$csv_file" "$NAMESPACE/$temp_pod:/data/telemetry_data.csv"
    
    # Verify file was copied
    kubectl exec "$temp_pod" -n "$NAMESPACE" -- ls -la /data/
    
    log_info "CSV file uploaded successfully to PVC '$PVC_NAME'"
    
    # Clean up
    kubectl delete pod "$temp_pod" -n "$NAMESPACE" --force --grace-period=0 &> /dev/null || true
}

# List all CSV data sources
list_csv_sources() {
    log_step "Listing CSV data sources in namespace '$NAMESPACE'"
    
    echo ""
    echo "=== ConfigMaps ==="
    kubectl get configmaps -n "$NAMESPACE" -l app.kubernetes.io/name=telemetry-pipeline 2>/dev/null || echo "No telemetry ConfigMaps found"
    
    echo ""
    echo "=== PVCs ==="
    kubectl get pvc -n "$NAMESPACE" -l app.kubernetes.io/name=telemetry-pipeline 2>/dev/null || echo "No telemetry PVCs found"
    
    echo ""
    echo "=== Current Streamer Configuration ==="
    kubectl get configmap -n "$NAMESPACE" -o yaml | grep -A 5 -B 5 "csv-source-type" || echo "No streamer configuration found"
}

# Deploy with ConfigMap workflow
deploy_with_configmap() {
    local csv_file="$1"
    
    log_step "Complete workflow: ConfigMap deployment"
    
    create_configmap "$csv_file"
    
    log_info "Deploying telemetry pipeline with ConfigMap CSV source..."
    
    # Check if helm chart exists
    local chart_path="./deployments/helm/telemetry-pipeline"
    if [[ ! -d "$chart_path" ]]; then
        log_error "Helm chart not found at '$chart_path'. Make sure you're in the project root."
        exit 1
    fi
    
    helm upgrade --install telemetry-pipeline "$chart_path" \
        --namespace "$NAMESPACE" --create-namespace \
        --values "$chart_path/values-csv-configmap.yaml" \
        --set streamer.config.csvSource.configMapName="$CONFIGMAP_NAME"
    
    log_info "Deployment completed successfully!"
}

# Deploy with PVC workflow
deploy_with_pvc() {
    local csv_file="$1"
    local size="${2:-1Gi}"
    
    log_step "Complete workflow: PVC deployment"
    
    create_pvc "$size"
    upload_to_pvc "$csv_file"
    
    log_info "Deploying telemetry pipeline with PVC CSV source..."
    
    # Check if helm chart exists
    local chart_path="./deployments/helm/telemetry-pipeline"
    if [[ ! -d "$chart_path" ]]; then
        log_error "Helm chart not found at '$chart_path'. Make sure you're in the project root."
        exit 1
    fi
    
    helm upgrade --install telemetry-pipeline "$chart_path" \
        --namespace "$NAMESPACE" --create-namespace \
        --values "$chart_path/values-csv-pvc.yaml" \
        --set streamer.config.csvSource.pvcName="$PVC_NAME"
    
    log_info "Deployment completed successfully!"
}

# Deploy with URL workflow
deploy_with_url() {
    local url="$1"
    
    if [[ -z "$url" ]]; then
        log_error "URL is required"
        exit 1
    fi
    
    log_step "Complete workflow: URL deployment"
    
    log_info "Deploying telemetry pipeline with URL CSV source: $url"
    
    # Check if helm chart exists
    local chart_path="./deployments/helm/telemetry-pipeline"
    if [[ ! -d "$chart_path" ]]; then
        log_error "Helm chart not found at '$chart_path'. Make sure you're in the project root."
        exit 1
    fi
    
    helm upgrade --install telemetry-pipeline "$chart_path" \
        --namespace "$NAMESPACE" --create-namespace \
        --values "$chart_path/values-csv-url.yaml" \
        --set streamer.config.csvSource.url="$url"
    
    log_info "Deployment completed successfully!"
    log_info "CSV will be downloaded from: $url"
}

# Main script logic
main() {
    check_kubectl
    
    case "${1:-}" in
        "create-configmap")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            create_configmap "$2"
            ;;
        "update-configmap")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            update_configmap "$2"
            ;;
        "create-pvc")
            create_pvc "${2:-1Gi}"
            ;;
        "upload-to-pvc")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            upload_to_pvc "$2"
            ;;
        "list-csv-sources")
            list_csv_sources
            ;;
        "validate-csv")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            validate_csv "$2"
            ;;
        "deploy-with-configmap")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            deploy_with_configmap "$2"
            ;;
        "deploy-with-pvc")
            if [[ -z "${2:-}" ]]; then
                log_error "CSV file is required"
                print_usage
                exit 1
            fi
            deploy_with_pvc "$2" "${3:-1Gi}"
            ;;
        "deploy-with-url")
            if [[ -z "${2:-}" ]]; then
                log_error "URL is required"
                print_usage
                exit 1
            fi
            deploy_with_url "$2"
            ;;
        "help"|"--help"|"-h")
            print_usage
            ;;
        *)
            log_error "Unknown command: ${1:-}"
            print_usage
            exit 1
            ;;
    esac
}

# Run main function
main "$@"
