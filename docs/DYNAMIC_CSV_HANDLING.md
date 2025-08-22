# Dynamic CSV File Handling

## Overview

This guide covers multiple methods to dynamically pass CSV files to both local and Kubernetes streamers. The telemetry pipeline supports flexible CSV data ingestion through various mechanisms.

## 🏠 Local Development - CSV Methods

### Method 1: Environment Variables (Recommended)

```bash
# Set CSV file path via environment variable
export CSV_FILE="/path/to/your/telemetry_data.csv"
make run-nexus-streamer

# Or inline
CSV_FILE="./custom_data.csv" make run-nexus-streamer
```

### Method 2: Command Line Arguments

```bash
# Build first
make build-nexus

# Run with custom CSV file
./bin/nexus-streamer -csv="/path/to/your/data.csv" -batch-size=50 -stream-interval=2s -loop=true
```

### Method 3: Multiple Files with Scaling

```bash
# Scale with different CSV files per streamer
CSV_FILE="dataset1.csv" STREAMER_INSTANCES=2 make scale-streamers
CSV_FILE="dataset2.csv" STREAMER_INSTANCES=2 make scale-streamers

# Or use the scaling script directly
./scripts/scale-local.sh start streamers 3 dataset1.csv
./scripts/scale-local.sh start streamers 2 dataset2.csv
```

### Method 4: Configuration File Override

```bash
# Create custom config
cat > custom_streamer.env << EOF
CSV_FILE=/data/production_metrics.csv
BATCH_SIZE=200
STREAM_INTERVAL=500ms
LOOP_MODE=true
LOG_LEVEL=debug
EOF

# Source and run
source custom_streamer.env
make run-nexus-streamer
```

## ☸️ Kubernetes - CSV Methods

### Method 1: ConfigMap (Small Files < 1MB)

#### Create ConfigMap from CSV
```bash
# Single command approach
make csv-create-configmap CSV_FILE="./my_telemetry_data.csv"

# Manual approach
kubectl create configmap telemetry-csv-data \
  --from-file=dcgm_metrics_20250718_134233.csv="./my_telemetry_data.csv" \
  --namespace telemetry-system

# Deploy with ConfigMap
make csv-deploy-configmap CSV_FILE="./my_telemetry_data.csv"
```

#### Update Existing ConfigMap
```bash
# Update with new CSV data
make csv-update-configmap CSV_FILE="./updated_data.csv"

# Or manually
kubectl create configmap telemetry-csv-data \
  --from-file=dcgm_metrics_20250718_134233.csv="./updated_data.csv" \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Method 2: Persistent Volume (Large Files > 1MB)

#### Create PVC and Upload CSV
```bash
# Complete workflow
make csv-deploy-pvc CSV_FILE="./large_dataset.csv" SIZE=5Gi

# Step by step
make csv-create-pvc SIZE=2Gi
make csv-upload-pvc CSV_FILE="./large_dataset.csv"
```

#### Upload Multiple CSV Files
```bash
# Create larger PVC
make csv-create-pvc SIZE=10Gi

# Upload multiple files
make csv-upload-pvc CSV_FILE="./dataset1.csv"
make csv-upload-pvc CSV_FILE="./dataset2.csv"
make csv-upload-pvc CSV_FILE="./dataset3.csv"

# List uploaded files
kubectl exec -n telemetry-system \
  $(kubectl get pods -n telemetry-system -l app=nexus-streamer -o name | head -1) \
  -- ls -la /data/
```

### Method 3: URL Download (Remote Files)

#### Deploy with Remote CSV
```bash
# Deploy streamer that downloads CSV from URL
make csv-deploy-url CSV_URL="https://example.com/telemetry_data.csv"

# Multiple URLs with custom Helm values
cat > custom-values.yaml << EOF
nexusStreamer:
  env:
    CSV_FILE: "remote_data.csv"
  csvData:
    url:
      enabled: true
      downloadUrl: "https://my-storage.com/gpu_metrics.csv"
EOF

helm upgrade telemetry-pipeline ./deployments/helm/telemetry-pipeline \
  -f custom-values.yaml --namespace telemetry-system
```

### Method 4: Init Container with Multiple Sources

```yaml
# Custom deployment with init container
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nexus-streamer-multi-csv
spec:
  template:
    spec:
      initContainers:
      - name: csv-downloader
        image: curlimages/curl:latest
        command:
        - /bin/sh
        - -c
        - |
          curl -o /data/dataset1.csv "https://source1.com/data.csv"
          curl -o /data/dataset2.csv "https://source2.com/data.csv"
          curl -o /data/dataset3.csv "https://source3.com/data.csv"
        volumeMounts:
        - name: csv-data
          mountPath: /data
      containers:
      - name: nexus-streamer
        env:
        - name: CSV_FILE
          value: "/data/dataset1.csv"  # Can be changed per replica
        volumeMounts:
        - name: csv-data
          mountPath: /data
      volumes:
      - name: csv-data
        emptyDir: {}
```

## 🔄 Dynamic Switching Methods

### Method 1: Runtime File Switching (Local)

```bash
# Create file watcher script
cat > switch_csv.sh << EOF
#!/bin/bash
while true; do
    if [ -f "switch_to_file.txt" ]; then
        NEW_CSV=\$(cat switch_to_file.txt)
        echo "Switching to CSV: \$NEW_CSV"
        pkill -f nexus-streamer
        sleep 2
        CSV_FILE="\$NEW_CSV" make run-nexus-streamer &
        rm switch_to_file.txt
    fi
    sleep 5
done
EOF

chmod +x switch_csv.sh

# Usage: echo "new_dataset.csv" > switch_to_file.txt
```

### Method 2: Kubernetes ConfigMap Hot Reload

```bash
# Update ConfigMap triggers pod restart
update_csv_k8s() {
    local csv_file="$1"
    kubectl create configmap telemetry-csv-data \
      --from-file=dcgm_metrics_20250718_134233.csv="$csv_file" \
      --dry-run=client -o yaml | kubectl apply -f -
    
    # Force pod restart to pick up new data
    kubectl rollout restart deployment/nexus-streamer -n telemetry-system
}

# Usage
update_csv_k8s "./new_production_data.csv"
```

### Method 3: Multiple Streamers with Different CSV Files

```yaml
# Helm values for multiple streamers
nexusStreamer:
  replicas: 3
  instances:
    - name: "streamer-dataset1"
      csvFile: "dataset1.csv"
      configMap: "csv-data-1"
    - name: "streamer-dataset2" 
      csvFile: "dataset2.csv"
      configMap: "csv-data-2"
    - name: "streamer-dataset3"
      csvFile: "dataset3.csv"
      configMap: "csv-data-3"
```

## 📁 File Organization Strategies

### Strategy 1: Time-Based Organization

```bash
# Organize by date/time
data/
├── 2025-01/
│   ├── gpu_metrics_2025-01-15.csv
│   ├── gpu_metrics_2025-01-16.csv
│   └── gpu_metrics_2025-01-17.csv
├── 2025-02/
│   ├── gpu_metrics_2025-02-01.csv
│   └── gpu_metrics_2025-02-02.csv

# Script to process by date
process_by_date() {
    local date_folder="$1"
    for csv_file in data/${date_folder}/*.csv; do
        echo "Processing: $csv_file"
        CSV_FILE="$csv_file" make run-nexus-streamer
        sleep 10  # Wait for processing
        pkill -f nexus-streamer
    done
}

# Usage: process_by_date "2025-01"
```

### Strategy 2: Environment-Based Organization

```bash
# Organize by environment
data/
├── development/
│   ├── dev_gpu_metrics.csv
│   └── test_sample.csv
├── staging/
│   ├── staging_metrics.csv
│   └── load_test_data.csv
├── production/
│   ├── prod_cluster1_metrics.csv
│   ├── prod_cluster2_metrics.csv
│   └── prod_historical_data.csv

# Environment-specific deployment
deploy_env() {
    local env="$1"
    local csv_file="data/${env}/$(ls data/${env}/ | head -1)"
    
    CSV_FILE="$csv_file" \
    NAMESPACE="telemetry-${env}" \
    make csv-deploy-configmap
}
```

### Strategy 3: Cluster-Based Organization

```bash
# Organize by cluster/datacenter
data/
├── us-west-1/
│   ├── cluster-a-gpu-metrics.csv
│   └── cluster-b-gpu-metrics.csv
├── us-east-1/
│   ├── cluster-c-gpu-metrics.csv
│   └── cluster-d-gpu-metrics.csv
├── eu-central-1/
│   └── cluster-e-gpu-metrics.csv
```

## 🚀 Automation Scripts

### Script 1: Batch CSV Processor

```bash
#!/bin/bash
# batch_csv_processor.sh

CSV_DIR="${1:-./data}"
CONCURRENT_STREAMERS="${2:-3}"

echo "Processing CSV files from: $CSV_DIR"
echo "Concurrent streamers: $CONCURRENT_STREAMERS"

# Find all CSV files
csv_files=($(find "$CSV_DIR" -name "*.csv" -type f))
echo "Found ${#csv_files[@]} CSV files"

# Process in batches
for ((i=0; i<${#csv_files[@]}; i+=CONCURRENT_STREAMERS)); do
    batch_files=("${csv_files[@]:i:CONCURRENT_STREAMERS}")
    
    echo "Processing batch: ${batch_files[@]}"
    
    # Start streamers for this batch
    for ((j=0; j<${#batch_files[@]}; j++)); do
        csv_file="${batch_files[j]}"
        streamer_id="batch_${i}_${j}"
        
        CSV_FILE="$csv_file" \
        STREAMER_ID="$streamer_id" \
        LOG_LEVEL="info" \
        nohup make run-nexus-streamer > "logs/streamer_${streamer_id}.log" 2>&1 &
        
        echo "Started streamer $streamer_id for $csv_file"
    done
    
    # Wait for batch completion
    wait
    echo "Batch completed"
    sleep 5
done

echo "All CSV files processed"
```

### Script 2: Kubernetes CSV Rotator

```bash
#!/bin/bash
# k8s_csv_rotator.sh

NAMESPACE="${NAMESPACE:-telemetry-system}"
CSV_SOURCE_DIR="${1:-./csv_files}"
ROTATION_INTERVAL="${2:-300}"  # 5 minutes

rotate_csv() {
    local csv_files=($(ls "$CSV_SOURCE_DIR"/*.csv))
    local current_index=0
    
    while true; do
        local csv_file="${csv_files[$current_index]}"
        local filename=$(basename "$csv_file")
        
        echo "Rotating to: $filename"
        
        # Update ConfigMap
        kubectl create configmap telemetry-csv-data \
          --from-file=dcgm_metrics_20250718_134233.csv="$csv_file" \
          --namespace "$NAMESPACE" \
          --dry-run=client -o yaml | kubectl apply -f -
        
        # Restart deployment
        kubectl rollout restart deployment/nexus-streamer -n "$NAMESPACE"
        kubectl rollout status deployment/nexus-streamer -n "$NAMESPACE"
        
        echo "Switched to $filename, waiting ${ROTATION_INTERVAL}s"
        sleep "$ROTATION_INTERVAL"
        
        # Next file
        current_index=$(( (current_index + 1) % ${#csv_files[@]} ))
    done
}

# Usage: ./k8s_csv_rotator.sh ./production_csv_files 600
```

### Script 3: Multi-Environment Deployer

```bash
#!/bin/bash
# multi_env_deployer.sh

deploy_to_environments() {
    local csv_file="$1"
    local environments=("development" "staging" "production")
    
    for env in "${environments[@]}"; do
        echo "Deploying $csv_file to $env environment"
        
        # Environment-specific configuration
        case "$env" in
            "development")
                BATCH_SIZE=10
                STREAM_INTERVAL="5s"
                REPLICAS=1
                ;;
            "staging")
                BATCH_SIZE=50
                STREAM_INTERVAL="2s"
                REPLICAS=2
                ;;
            "production")
                BATCH_SIZE=100
                STREAM_INTERVAL="1s"
                REPLICAS=3
                ;;
        esac
        
        # Deploy with environment-specific values
        CSV_FILE="$csv_file" \
        NAMESPACE="telemetry-${env}" \
        helm upgrade --install "telemetry-pipeline-${env}" \
          ./deployments/helm/telemetry-pipeline \
          --set nexusStreamer.replicas="$REPLICAS" \
          --set nexusStreamer.env.BATCH_SIZE="$BATCH_SIZE" \
          --set nexusStreamer.env.STREAM_INTERVAL="$STREAM_INTERVAL" \
          --namespace "telemetry-${env}" \
          --create-namespace
        
        echo "Deployed to $env environment"
    done
}

# Usage: ./multi_env_deployer.sh "./gpu_metrics_v2.csv"
```

## 📊 Monitoring and Validation

### CSV Processing Monitoring

```bash
# Monitor CSV processing progress
monitor_csv_processing() {
    local csv_file="$1"
    local total_records=$(( $(wc -l < "$csv_file") - 1 ))  # Subtract header
    
    echo "Monitoring processing of $csv_file ($total_records records)"
    
    while true; do
        # Check processed records in etcd
        processed=$(etcdctl get --prefix "/telemetry/" --keys-only | wc -l)
        percentage=$(( processed * 100 / total_records ))
        
        echo "Progress: $processed/$total_records ($percentage%)"
        
        if [ "$processed" -ge "$total_records" ]; then
            echo "✅ CSV processing completed!"
            break
        fi
        
        sleep 10
    done
}
```

### Validation Scripts

```bash
# Validate CSV before processing
validate_csv_format() {
    local csv_file="$1"
    
    # Check file exists and is readable
    if [[ ! -f "$csv_file" || ! -r "$csv_file" ]]; then
        echo "❌ CSV file not accessible: $csv_file"
        return 1
    fi
    
    # Check header format
    local header=$(head -n 1 "$csv_file")
    local required_fields=("timestamp" "metric_name" "gpu_id" "uuid" "Hostname")
    
    for field in "${required_fields[@]}"; do
        if [[ ! "$header" =~ $field ]]; then
            echo "❌ Missing required field: $field"
            return 1
        fi
    done
    
    # Check record count
    local record_count=$(( $(wc -l < "$csv_file") - 1 ))
    if [ "$record_count" -eq 0 ]; then
        echo "❌ CSV file has no data records"
        return 1
    fi
    
    echo "✅ CSV validation passed: $record_count records"
    return 0
}
```

## 🛠️ Troubleshooting

### Common Issues and Solutions

#### Issue 1: File Not Found
```bash
# Check file permissions and path
ls -la /path/to/csv/file.csv
file /path/to/csv/file.csv

# Fix permissions
chmod 644 /path/to/csv/file.csv
```

#### Issue 2: ConfigMap Size Limit
```bash
# Check ConfigMap size (max ~1MB)
kubectl get configmap telemetry-csv-data -o yaml | wc -c

# If too large, use PVC instead
make csv-deploy-pvc CSV_FILE="large_file.csv" SIZE=5Gi
```

#### Issue 3: Pod Not Picking Up New CSV
```bash
# Force pod restart
kubectl rollout restart deployment/nexus-streamer -n telemetry-system

# Check pod logs
kubectl logs -f deployment/nexus-streamer -n telemetry-system
```

#### Issue 4: CSV Format Issues
```bash
# Validate CSV format
make csv-validate CSV_FILE="problematic_file.csv"

# Fix common issues
sed -i 's/\r$//' file.csv  # Remove Windows line endings
dos2unix file.csv          # Convert line endings
```

## 📝 Best Practices

### 1. File Management
- **Use descriptive filenames** with timestamps or versions
- **Validate CSV format** before deployment
- **Keep backup copies** of important datasets
- **Use appropriate storage method** (ConfigMap < 1MB, PVC > 1MB)

### 2. Performance Optimization
- **Batch processing**: Use appropriate batch sizes (100-1000 records)
- **Resource allocation**: Set proper CPU/memory limits
- **Parallel processing**: Scale streamers based on data volume
- **Monitoring**: Track processing progress and performance

### 3. Security Considerations
- **File permissions**: Ensure proper read permissions
- **Network security**: Use HTTPS for remote CSV downloads
- **Access control**: Implement proper RBAC for Kubernetes resources
- **Data encryption**: Encrypt sensitive CSV data at rest

### 4. Operational Guidelines
- **Test locally first** before Kubernetes deployment
- **Use staging environment** for validation
- **Monitor processing metrics** and set up alerts
- **Document CSV schemas** and data sources

This comprehensive approach ensures flexible, reliable, and scalable CSV data ingestion for both local development and production Kubernetes environments! 🚀
