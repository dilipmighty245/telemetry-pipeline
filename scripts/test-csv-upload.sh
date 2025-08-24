#!/bin/bash

# Test script to verify CSV upload and reprocessing functionality

set -e

echo "Testing CSV upload and reprocessing functionality..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
STREAMER_URL="http://localhost:8081"
UPLOAD_ENDPOINT="$STREAMER_URL/api/v1/csv/upload"
STATUS_ENDPOINT="$STREAMER_URL/api/v1/status"
TEST_CSV_FILE="/tmp/test_telemetry.csv"

# Function to create a test CSV file with specified record count and timestamp offset
create_test_csv() {
    local record_count=$1
    local timestamp_offset=$2
    local filename=$3
    
    echo "Creating test CSV with $record_count records (timestamp offset: ${timestamp_offset}s)"
    
    # Create CSV header
    cat > "$filename" << EOF
timestamp,gpu_id,uuid,device,model_name,hostname,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
EOF

    # Generate test data
    base_timestamp=$(date -j -f "%Y-%m-%d %H:%M:%S" "2024-01-01 00:00:00" "+%s" 2>/dev/null || date -r 1704067200 "+%s")
    base_timestamp=$((base_timestamp + timestamp_offset))
    
    for i in $(seq 1 $record_count); do
        timestamp=$(date -r $((base_timestamp + i)) '+%Y-%m-%d %H:%M:%S')
        gpu_id=$((i % 4))  # 0-3 for 4 GPUs
        uuid="GPU-$(printf "%08x" $((i + timestamp_offset)))"
        device="nvidia$gpu_id"
        model_name="Tesla V100"
        hostname="test-host-$((i % 3 + 1))"  # 3 different hostnames
        gpu_util=$(awk "BEGIN {printf \"%.1f\", rand()*100}")
        mem_util=$(awk "BEGIN {printf \"%.1f\", rand()*100}")
        mem_used=$(awk "BEGIN {printf \"%.0f\", rand()*32000}")
        mem_free=$((32000 - mem_used))
        temp=$(awk "BEGIN {printf \"%.0f\", 30 + rand()*50}")
        power=$(awk "BEGIN {printf \"%.0f\", 100 + rand()*200}")
        sm_clock=$(awk "BEGIN {printf \"%.0f\", 1000 + rand()*500}")
        mem_clock=$(awk "BEGIN {printf \"%.0f\", 800 + rand()*400}")
        
        echo "$timestamp,$gpu_id,$uuid,$device,$model_name,$hostname,$gpu_util,$mem_util,$mem_used,$mem_free,$temp,$power,$sm_clock,$mem_clock" >> "$filename"
    done
    
    echo "Created test CSV file: $filename"
}

# Function to upload CSV file
upload_csv() {
    local filename=$1
    local description=$2
    
    echo -e "\n${BLUE}Uploading CSV file: $filename${NC}"
    echo "Description: $description"
    
    response=$(curl -s -X POST \
        -F "file=@$filename" \
        -F "description=$description" \
        "$UPLOAD_ENDPOINT")
    
    echo "Response: $response"
    
    # Check if upload was successful
    success=$(echo "$response" | grep -o '"success":[^,}]*' | cut -d':' -f2 | tr -d ' "')
    if [ "$success" = "true" ]; then
        echo -e "${GREEN}✅ Upload successful${NC}"
        
        # Extract details
        record_count=$(echo "$response" | grep -o '"record_count":[^,}]*' | cut -d':' -f2 | tr -d ' "')
        md5_hash=$(echo "$response" | grep -o '"md5_hash":"[^"]*"' | cut -d':' -f2 | tr -d '"')
        file_id=$(echo "$response" | grep -o '"file_id":"[^"]*"' | cut -d':' -f2 | tr -d '"')
        
        echo "  Records processed: $record_count"
        echo "  MD5 hash: $md5_hash"
        echo "  File ID: $file_id"
        
        return 0
    else
        echo -e "${RED}❌ Upload failed${NC}"
        error=$(echo "$response" | grep -o '"error":"[^"]*"' | cut -d':' -f2- | tr -d '"')
        echo "  Error: $error"
        return 1
    fi
}

# Function to check service status
check_service_status() {
    echo -e "\n${BLUE}Checking service status...${NC}"
    
    status_response=$(curl -s "$STATUS_ENDPOINT" || echo "ERROR")
    
    if [ "$status_response" = "ERROR" ]; then
        echo -e "${RED}❌ Service not available at $STATUS_ENDPOINT${NC}"
        return 1
    else
        echo -e "${GREEN}✅ Service is available${NC}"
        echo "Status: $status_response"
        return 0
    fi
}

# Function to calculate file hash
get_file_hash() {
    local filename=$1
    md5sum "$filename" | cut -d' ' -f1
}

# Main test execution
main() {
    echo -e "${YELLOW}=== CSV Upload and Reprocessing Test ===${NC}"
    
    # Check if service is running
    if ! check_service_status; then
        echo -e "${RED}Please start the nexus-streamer service first${NC}"
        exit 1
    fi
    
    # Test 1: Upload initial CSV file
    echo -e "\n${YELLOW}Test 1: Initial CSV upload${NC}"
    create_test_csv 2470 0 "$TEST_CSV_FILE"
    initial_hash=$(get_file_hash "$TEST_CSV_FILE")
    echo "Initial file hash: $initial_hash"
    
    if upload_csv "$TEST_CSV_FILE" "Initial upload with 2470 records"; then
        echo -e "${GREEN}✅ Initial upload successful${NC}"
    else
        echo -e "${RED}❌ Initial upload failed${NC}"
        exit 1
    fi
    
    # Test 2: Upload same file again (should work as files are deleted after processing)
    echo -e "\n${YELLOW}Test 2: Re-upload same file${NC}"
    if upload_csv "$TEST_CSV_FILE" "Re-upload of same file"; then
        echo -e "${GREEN}✅ Re-upload successful${NC}"
    else
        echo -e "${RED}❌ Re-upload failed${NC}"
        exit 1
    fi
    
    # Test 3: Upload file with different timestamps
    echo -e "\n${YELLOW}Test 3: Upload with different timestamps${NC}"
    create_test_csv 2470 3600 "${TEST_CSV_FILE}.v2"  # 1 hour offset
    modified_hash=$(get_file_hash "${TEST_CSV_FILE}.v2")
    echo "Modified file hash: $modified_hash"
    
    if [ "$initial_hash" = "$modified_hash" ]; then
        echo -e "${YELLOW}⚠️  Warning: File hashes are identical despite timestamp changes${NC}"
    else
        echo -e "${GREEN}✅ File hashes are different as expected${NC}"
    fi
    
    if upload_csv "${TEST_CSV_FILE}.v2" "Upload with different timestamps (1 hour offset)"; then
        echo -e "${GREEN}✅ Modified timestamp upload successful${NC}"
    else
        echo -e "${RED}❌ Modified timestamp upload failed${NC}"
        exit 1
    fi
    
    # Test 4: Upload file with completely different data
    echo -e "\n${YELLOW}Test 4: Upload with different data${NC}"
    create_test_csv 1000 7200 "${TEST_CSV_FILE}.v3"  # Different record count and 2 hour offset
    different_hash=$(get_file_hash "${TEST_CSV_FILE}.v3")
    echo "Different data file hash: $different_hash"
    
    if upload_csv "${TEST_CSV_FILE}.v3" "Upload with different data (1000 records, 2 hour offset)"; then
        echo -e "${GREEN}✅ Different data upload successful${NC}"
    else
        echo -e "${RED}❌ Different data upload failed${NC}"
        exit 1
    fi
    
    # Cleanup
    rm -f "$TEST_CSV_FILE" "${TEST_CSV_FILE}.v2" "${TEST_CSV_FILE}.v3"
    
    # Final status check
    check_service_status
    
    echo -e "\n${GREEN}🎉 All CSV upload tests passed!${NC}"
    echo -e "${BLUE}The service successfully processes CSV files multiple times, even with the same data.${NC}"
}

# Run the main function
main "$@"
