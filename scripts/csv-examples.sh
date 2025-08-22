#!/bin/bash

# CSV Examples Script for Telemetry Pipeline
# Demonstrates various methods to pass CSV files dynamically

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m' # No Color

print_header() {
    echo -e "${BLUE}================================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}================================================${NC}"
}

print_step() {
    echo -e "${GREEN}[STEP]${NC} $1"
}

print_info() {
    echo -e "${YELLOW}[INFO]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Create sample CSV files for examples
create_sample_csv_files() {
    print_header "Creating Sample CSV Files"
    
    local csv_dir="$PROJECT_ROOT/sample_csv_data"
    mkdir -p "$csv_dir"
    
    # Sample 1: Small dataset
    cat > "$csv_dir/small_sample.csv" << 'EOF'
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-sample-001,NVIDIA H100,sample-host-1,,,,"85.5","datacenter=sample"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-sample-001,NVIDIA H100,sample-host-1,,,,"72.3","datacenter=sample"
2025-07-18T20:42:36Z,DCGM_FI_DEV_MEM_COPY_UTIL,0,nvidia0,GPU-sample-001,NVIDIA H100,sample-host-1,,,,"45.2","datacenter=sample"
2025-07-18T20:42:37Z,DCGM_FI_DEV_GPU_UTIL,1,nvidia1,GPU-sample-002,NVIDIA H100,sample-host-1,,,,"92.1","datacenter=sample"
2025-07-18T20:42:38Z,DCGM_FI_DEV_GPU_TEMP,1,nvidia1,GPU-sample-002,NVIDIA H100,sample-host-1,,,,"68.7","datacenter=sample"
EOF

    # Sample 2: Multi-host dataset
    cat > "$csv_dir/multi_host.csv" << 'EOF'
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-host1-001,NVIDIA A100,host-1,,,,"75.5","cluster=demo"
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-host2-001,NVIDIA A100,host-2,,,,"82.3","cluster=demo"
2025-07-18T20:42:34Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-host3-001,NVIDIA A100,host-3,,,,"91.7","cluster=demo"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-host1-001,NVIDIA A100,host-1,,,,"65.2","cluster=demo"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-host2-001,NVIDIA A100,host-2,,,,"70.8","cluster=demo"
2025-07-18T20:42:35Z,DCGM_FI_DEV_GPU_TEMP,0,nvidia0,GPU-host3-001,NVIDIA A100,host-3,,,,"73.4","cluster=demo"
EOF

    # Sample 3: Time series dataset
    cat > "$csv_dir/time_series.csv" << 'EOF'
timestamp,metric_name,gpu_id,device,uuid,modelName,Hostname,container,pod,namespace,value,labels_raw
2025-07-18T20:40:00Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-ts-001,NVIDIA H100,ts-host,,,,"50.0","series=demo"
2025-07-18T20:41:00Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-ts-001,NVIDIA H100,ts-host,,,,"65.5","series=demo"
2025-07-18T20:42:00Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-ts-001,NVIDIA H100,ts-host,,,,"78.2","series=demo"
2025-07-18T20:43:00Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-ts-001,NVIDIA H100,ts-host,,,,"85.7","series=demo"
2025-07-18T20:44:00Z,DCGM_FI_DEV_GPU_UTIL,0,nvidia0,GPU-ts-001,NVIDIA H100,ts-host,,,,"92.3","series=demo"
EOF

    print_step "Created sample CSV files:"
    ls -la "$csv_dir/"
    echo ""
}

# Example 1: Local environment variable method
example_local_env_var() {
    print_header "Example 1: Local Environment Variable Method"
    
    local csv_file="$PROJECT_ROOT/sample_csv_data/small_sample.csv"
    
    print_step "Setting CSV_FILE environment variable"
    echo "export CSV_FILE=\"$csv_file\""
    
    print_step "Running streamer with environment variable"
    echo "make run-nexus-streamer"
    
    print_info "Alternative inline method:"
    echo "CSV_FILE=\"$csv_file\" make run-nexus-streamer"
    
    print_info "With additional configuration:"
    echo "CSV_FILE=\"$csv_file\" BATCH_SIZE=10 STREAM_INTERVAL=2s LOG_LEVEL=debug make run-nexus-streamer"
    echo ""
}

# Example 2: Local command line method
example_local_command_line() {
    print_header "Example 2: Local Command Line Arguments"
    
    local csv_file="$PROJECT_ROOT/sample_csv_data/multi_host.csv"
    
    print_step "Build the streamer first"
    echo "make build-nexus"
    
    print_step "Run with command line arguments"
    echo "./bin/nexus-streamer \\"
    echo "  -csv=\"$csv_file\" \\"
    echo "  -batch-size=5 \\"
    echo "  -stream-interval=1s \\"
    echo "  -loop=true \\"
    echo "  -log-level=info"
    
    print_info "Available command line options:"
    echo "  -csv           : CSV file path"
    echo "  -batch-size    : Records per batch"
    echo "  -stream-interval : Time between batches"
    echo "  -loop          : Continuous processing"
    echo "  -log-level     : Logging verbosity"
    echo ""
}

# Example 3: Local scaling with different CSV files
example_local_scaling() {
    print_header "Example 3: Local Scaling with Different CSV Files"
    
    print_step "Start multiple streamers with different CSV files"
    echo "# Terminal 1 - Small dataset"
    echo "CSV_FILE=\"./sample_csv_data/small_sample.csv\" STREAMER_ID=\"small-data\" make run-nexus-streamer"
    echo ""
    echo "# Terminal 2 - Multi-host dataset"
    echo "CSV_FILE=\"./sample_csv_data/multi_host.csv\" STREAMER_ID=\"multi-host\" make run-nexus-streamer"
    echo ""
    echo "# Terminal 3 - Time series dataset"
    echo "CSV_FILE=\"./sample_csv_data/time_series.csv\" STREAMER_ID=\"time-series\" make run-nexus-streamer"
    
    print_step "Using the scaling script"
    echo "./scripts/scale-local.sh start streamers 3 ./sample_csv_data/small_sample.csv"
    
    print_step "Monitor scaling status"
    echo "make scale-status"
    
    print_step "Stop all scaled instances"
    echo "make scale-stop"
    echo ""
}

# Example 4: Kubernetes ConfigMap method
example_k8s_configmap() {
    print_header "Example 4: Kubernetes ConfigMap Method (Small Files)"
    
    local csv_file="$PROJECT_ROOT/sample_csv_data/small_sample.csv"
    
    print_step "Create ConfigMap from CSV file"
    echo "make csv-create-configmap CSV_FILE=\"$csv_file\""
    
    print_step "Alternative manual method"
    echo "kubectl create configmap telemetry-csv-data \\"
    echo "  --from-file=dcgm_metrics_20250718_134233.csv=\"$csv_file\" \\"
    echo "  --namespace telemetry-system"
    
    print_step "Deploy with ConfigMap"
    echo "make csv-deploy-configmap CSV_FILE=\"$csv_file\""
    
    print_step "Update existing ConfigMap with new data"
    echo "make csv-update-configmap CSV_FILE=\"./sample_csv_data/multi_host.csv\""
    
    print_step "Verify ConfigMap contents"
    echo "kubectl get configmap telemetry-csv-data -o yaml -n telemetry-system"
    echo ""
}

# Example 5: Kubernetes PVC method
example_k8s_pvc() {
    print_header "Example 5: Kubernetes PVC Method (Large Files)"
    
    local csv_file="$PROJECT_ROOT/dcgm_metrics_20250718_134233.csv"
    
    print_step "Complete PVC workflow (recommended)"
    echo "make csv-deploy-pvc CSV_FILE=\"$csv_file\" SIZE=2Gi"
    
    print_step "Step-by-step PVC method"
    echo "# 1. Create PVC"
    echo "make csv-create-pvc SIZE=5Gi"
    echo ""
    echo "# 2. Upload CSV to PVC"
    echo "make csv-upload-pvc CSV_FILE=\"$csv_file\""
    echo ""
    echo "# 3. Deploy streamer with PVC"
    echo "helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \\"
    echo "  --set nexusStreamer.csvData.persistentVolume.enabled=true \\"
    echo "  --namespace telemetry-system"
    
    print_step "Upload multiple CSV files to same PVC"
    echo "make csv-upload-pvc CSV_FILE=\"./sample_csv_data/small_sample.csv\""
    echo "make csv-upload-pvc CSV_FILE=\"./sample_csv_data/multi_host.csv\""
    echo "make csv-upload-pvc CSV_FILE=\"./sample_csv_data/time_series.csv\""
    
    print_step "List files in PVC"
    echo "kubectl exec -n telemetry-system \\"
    echo "  \$(kubectl get pods -n telemetry-system -l app=nexus-streamer -o name | head -1) \\"
    echo "  -- ls -la /data/"
    echo ""
}

# Example 6: Kubernetes URL download method
example_k8s_url() {
    print_header "Example 6: Kubernetes URL Download Method"
    
    print_step "Deploy with remote CSV URL"
    echo "make csv-deploy-url CSV_URL=\"https://example.com/gpu_telemetry_data.csv\""
    
    print_step "Custom Helm values with URL"
    echo "cat > url-values.yaml << EOF"
    echo "nexusStreamer:"
    echo "  env:"
    echo "    CSV_FILE: \"remote_data.csv\""
    echo "  csvData:"
    echo "    url:"
    echo "      enabled: true"
    echo "      downloadUrl: \"https://my-storage.com/telemetry_metrics.csv\""
    echo "EOF"
    echo ""
    echo "helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline \\"
    echo "  -f url-values.yaml --namespace telemetry-system"
    
    print_step "Multiple URL sources with init container"
    echo "# See docs/DYNAMIC_CSV_HANDLING.md for advanced init container examples"
    echo ""
}

# Example 7: Dynamic switching methods
example_dynamic_switching() {
    print_header "Example 7: Dynamic CSV Switching"
    
    print_step "Local runtime switching"
    echo "# Create switch trigger file"
    echo "echo \"./sample_csv_data/multi_host.csv\" > switch_to_file.txt"
    echo ""
    echo "# The file watcher script will automatically switch"
    echo "# (See scripts in docs/DYNAMIC_CSV_HANDLING.md)"
    
    print_step "Kubernetes hot reload"
    echo "# Function to update CSV in K8s"
    echo "update_csv_k8s() {"
    echo "    kubectl create configmap telemetry-csv-data \\"
    echo "      --from-file=dcgm_metrics_20250718_134233.csv=\"\$1\" \\"
    echo "      --dry-run=client -o yaml | kubectl apply -f -"
    echo "    kubectl rollout restart deployment/nexus-streamer -n telemetry-system"
    echo "}"
    echo ""
    echo "# Usage"
    echo "update_csv_k8s \"./sample_csv_data/time_series.csv\""
    
    print_step "Automated rotation"
    echo "./scripts/k8s_csv_rotator.sh ./sample_csv_data 300  # Rotate every 5 minutes"
    echo ""
}

# Example 8: Monitoring and validation
example_monitoring() {
    print_header "Example 8: Monitoring and Validation"
    
    print_step "Validate CSV before processing"
    echo "make csv-validate CSV_FILE=\"./sample_csv_data/small_sample.csv\""
    
    print_step "Monitor processing progress"
    echo "# Check etcd for processed records"
    echo "etcdctl get --prefix \"/telemetry/\" --keys-only | wc -l"
    echo ""
    echo "# Watch processing in real-time"
    echo "watch -n 2 'etcdctl get --prefix \"/telemetry/\" --keys-only | wc -l'"
    
    print_step "Check API for results"
    echo "# List all GPUs"
    echo "curl -s http://localhost:8080/api/v1/gpus | jq '.'"
    echo ""
    echo "# Get telemetry for specific GPU"
    echo "curl -s \"http://localhost:8080/api/v1/gpus/GPU-sample-001/telemetry\" | jq '.'"
    
    print_step "Verify record counts"
    echo "./scripts/verify-record-count.sh"
    echo ""
}

# Example 9: Production workflows
example_production_workflows() {
    print_header "Example 9: Production Workflows"
    
    print_step "Multi-environment deployment"
    echo "# Deploy to development"
    echo "CSV_FILE=\"./sample_csv_data/small_sample.csv\" \\"
    echo "NAMESPACE=\"telemetry-dev\" \\"
    echo "make csv-deploy-configmap"
    echo ""
    echo "# Deploy to staging"
    echo "CSV_FILE=\"./sample_csv_data/multi_host.csv\" \\"
    echo "NAMESPACE=\"telemetry-staging\" \\"
    echo "make csv-deploy-pvc SIZE=5Gi"
    echo ""
    echo "# Deploy to production"
    echo "CSV_FILE=\"./dcgm_metrics_20250718_134233.csv\" \\"
    echo "NAMESPACE=\"telemetry-prod\" \\"
    echo "make csv-deploy-pvc SIZE=10Gi"
    
    print_step "Batch processing multiple files"
    echo "./scripts/batch_csv_processor.sh ./sample_csv_data 2"
    
    print_step "Automated testing pipeline"
    echo "# Test all sample files"
    echo "for csv_file in ./sample_csv_data/*.csv; do"
    echo "    echo \"Testing: \$csv_file\""
    echo "    make csv-validate CSV_FILE=\"\$csv_file\""
    echo "    CSV_FILE=\"\$csv_file\" timeout 30 make run-nexus-streamer"
    echo "done"
    echo ""
}

# Main menu
show_menu() {
    print_header "CSV Dynamic Handling Examples"
    echo "Choose an example to see:"
    echo ""
    echo "Local Development:"
    echo "  1) Environment Variables"
    echo "  2) Command Line Arguments" 
    echo "  3) Local Scaling with Multiple CSV Files"
    echo ""
    echo "Kubernetes Deployment:"
    echo "  4) ConfigMap Method (Small Files)"
    echo "  5) PVC Method (Large Files)"
    echo "  6) URL Download Method"
    echo ""
    echo "Advanced:"
    echo "  7) Dynamic CSV Switching"
    echo "  8) Monitoring and Validation"
    echo "  9) Production Workflows"
    echo ""
    echo "Utility:"
    echo "  0) Create Sample CSV Files"
    echo "  q) Quit"
    echo ""
}

# Main execution
main() {
    cd "$PROJECT_ROOT"
    
    while true; do
        show_menu
        read -p "Enter your choice: " choice
        echo ""
        
        case $choice in
            0) create_sample_csv_files ;;
            1) example_local_env_var ;;
            2) example_local_command_line ;;
            3) example_local_scaling ;;
            4) example_k8s_configmap ;;
            5) example_k8s_pvc ;;
            6) example_k8s_url ;;
            7) example_dynamic_switching ;;
            8) example_monitoring ;;
            9) example_production_workflows ;;
            q|Q) 
                print_info "Goodbye!"
                exit 0
                ;;
            *)
                print_error "Invalid choice. Please try again."
                ;;
        esac
        
        echo ""
        read -p "Press Enter to continue..."
        clear
    done
}

# Run main function if script is executed directly
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
