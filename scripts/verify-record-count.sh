#!/bin/bash

# Script to verify all CSV records are processed through the telemetry pipeline

set -e

echo "🔍 Verifying Telemetry Pipeline Record Processing"
echo "================================================"

# Check CSV file record count
CSV_FILE="dcgm_metrics_20250718_134233.csv"
if [[ -f "$CSV_FILE" ]]; then
    TOTAL_LINES=$(wc -l < "$CSV_FILE")
    CSV_RECORDS=$((TOTAL_LINES - 1))  # Subtract header line
    echo "📊 CSV File: $CSV_FILE"
    echo "   Total lines: $TOTAL_LINES"
    echo "   Data records: $CSV_RECORDS"
else
    echo "❌ CSV file not found: $CSV_FILE"
    exit 1
fi

# Function to check etcd for records
check_etcd_records() {
    echo ""
    echo "🔍 Checking etcd for processed records..."
    
    # Check if etcd is running
    if ! command -v etcdctl &> /dev/null; then
        echo "⚠️  etcdctl not found. Please install etcd client tools."
        return 1
    fi
    
    # Set etcd endpoints
    export ETCDCTL_API=3
    ETCD_ENDPOINT=${ETCD_ENDPOINTS:-localhost:2379}
    
    # Check message queue
    echo "📨 Checking message queue..."
    QUEUE_COUNT=$(etcdctl --endpoints="$ETCD_ENDPOINT" get --prefix "/telemetry/queue/" --count-only 2>/dev/null || echo "0")
    echo "   Messages in queue: $QUEUE_COUNT"
    
    # Check stored telemetry data
    echo "💾 Checking stored telemetry data..."
    DATA_COUNT=$(etcdctl --endpoints="$ETCD_ENDPOINT" get --prefix "/telemetry/clusters/default-cluster/hosts/" --keys-only 2>/dev/null | grep "/data/" | wc -l || echo "0")
    echo "   Stored telemetry records: $DATA_COUNT"
    
    # Check registered GPUs
    echo "🖥️  Checking registered GPUs..."
    GPU_COUNT=$(etcdctl --endpoints="$ETCD_ENDPOINT" get --prefix "/telemetry/clusters/default-cluster/hosts/" --keys-only 2>/dev/null | grep "/gpus/" | grep -v "/data/" | wc -l || echo "0")
    echo "   Registered GPUs: $GPU_COUNT"
    
    return 0
}

# Function to check via API
check_api_records() {
    echo ""
    echo "🌐 Checking API Gateway for records..."
    
    API_ENDPOINT=${API_ENDPOINT:-http://localhost:8080}
    
    # Check if API is responding
    if curl -s "$API_ENDPOINT/health" > /dev/null 2>&1; then
        echo "✅ API Gateway is responding"
        
        # Get GPU count
        GPU_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus" 2>/dev/null || echo '{"count":0}')
        GPU_API_COUNT=$(echo "$GPU_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2 || echo "0")
        echo "   GPUs via API: $GPU_API_COUNT"
        
        # Sample telemetry for first GPU if available
        if [[ "$GPU_API_COUNT" -gt 0 ]]; then
            FIRST_GPU=$(echo "$GPU_RESPONSE" | grep -o '"uuid":"[^"]*' | head -1 | cut -d':' -f2 | tr -d '"')
            if [[ -n "$FIRST_GPU" ]]; then
                echo "   Checking telemetry for GPU: $FIRST_GPU"
                TELEMETRY_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus/$FIRST_GPU/telemetry?limit=1000" 2>/dev/null || echo '{"count":0}')
                TELEMETRY_COUNT=$(echo "$TELEMETRY_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2 || echo "0")
                echo "   Telemetry records for this GPU: $TELEMETRY_COUNT"
                
                # Test time-based filtering
                echo "   Testing time-based queries..."
                START_TIME="2025-07-18T20:42:34Z"
                END_TIME="2025-07-18T20:42:36Z"
                TIME_FILTERED_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus/$FIRST_GPU/telemetry?start_time=$START_TIME&end_time=$END_TIME&limit=100" 2>/dev/null || echo '{"count":0}')
                TIME_FILTERED_COUNT=$(echo "$TIME_FILTERED_RESPONSE" | grep -o '"count":[0-9]*' | cut -d':' -f2 || echo "0")
                echo "   Records in time range ($START_TIME to $END_TIME): $TIME_FILTERED_COUNT"
            fi
        fi
    else
        echo "❌ API Gateway not responding at $API_ENDPOINT"
        echo "   Make sure nexus-gateway is running: make run-nexus-gateway"
    fi
}

# Function to show processing statistics
show_processing_stats() {
    echo ""
    echo "📈 Processing Statistics Summary"
    echo "==============================="
    echo "Expected records to process: $CSV_RECORDS"
    
    if [[ "$DATA_COUNT" -gt 0 ]]; then
        PERCENTAGE=$(echo "scale=2; $DATA_COUNT * 100 / $CSV_RECORDS" | bc -l 2>/dev/null || echo "0")
        echo "Records processed: $DATA_COUNT ($PERCENTAGE%)"
        
        if [[ "$DATA_COUNT" -eq "$CSV_RECORDS" ]]; then
            echo "✅ All records processed successfully!"
        elif [[ "$DATA_COUNT" -gt 0 ]]; then
            echo "⚠️  Partial processing - check if streamer/collector are still running"
        fi
    else
        echo "❌ No records found - pipeline may not be running"
    fi
}

# Function to show how to run the pipeline
show_pipeline_commands() {
    echo ""
    echo "🚀 Pipeline Commands"
    echo "==================="
    echo "To run the complete pipeline:"
    echo ""
    echo "# Terminal 1 - Start etcd (if not running)"
    echo "make setup-etcd"
    echo ""
    echo "# Terminal 2 - Start collector"
    echo "make run-nexus-collector"
    echo ""
    echo "# Terminal 3 - Start API gateway"  
    echo "make run-nexus-gateway"
    echo ""
    echo "# Terminal 4 - Start streamer (processes CSV)"
    echo "make run-nexus-streamer"
    echo ""
    echo "# Check this verification script"
    echo "./scripts/verify-record-count.sh"
    echo ""
    echo "📅 Time-based Query Examples:"
    echo "# Get all telemetry for a GPU"
    echo 'curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry"'
    echo ""
    echo "# Get telemetry in specific time range"
    echo 'curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?start_time=2025-07-18T20:42:34Z&end_time=2025-07-18T20:42:36Z"'
    echo ""
    echo "# Get recent telemetry with limit"
    echo 'curl "http://localhost:8080/api/v1/gpus/GPU-5fd4f087-86f3-7a43-b711-4771313afc50/telemetry?limit=100"'
}

# Main execution
main() {
    # Check etcd records
    if check_etcd_records; then
        # Check API records
        check_api_records
        
        # Show statistics
        show_processing_stats
    else
        echo "⚠️  Could not connect to etcd. Is it running?"
        echo "   Start etcd with: make setup-etcd"
    fi
    
    # Always show pipeline commands for reference
    show_pipeline_commands
}

# Run main function
main "$@"
