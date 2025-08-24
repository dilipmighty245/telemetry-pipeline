#!/bin/bash

# Debug script to investigate CSV processing issues

set -e

echo "Debugging CSV processing issues..."

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

echo -e "${BLUE}=== CSV Processing Debug Information ===${NC}"

# Check if service is running
echo -e "\n${YELLOW}1. Service Status Check${NC}"
if curl -s "$STATUS_ENDPOINT" > /dev/null; then
    echo -e "${GREEN}✅ Service is running at $STREAMER_URL${NC}"
    status=$(curl -s "$STATUS_ENDPOINT")
    echo "Status response: $status"
else
    echo -e "${RED}❌ Service is not accessible at $STREAMER_URL${NC}"
    echo "Please ensure the nexus-streamer is running with HTTP server enabled"
    exit 1
fi

# Check etcd connectivity
echo -e "\n${YELLOW}2. etcd Connectivity Check${NC}"
if command -v etcdctl > /dev/null; then
    echo "Checking etcd connection..."
    if etcdctl --endpoints=localhost:2379 endpoint health > /dev/null 2>&1; then
        echo -e "${GREEN}✅ etcd is accessible${NC}"
        
        # Check message queue keys
        echo "Checking message queue keys..."
        queue_keys=$(etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/queue/" --keys-only 2>/dev/null | wc -l)
        echo "Message queue keys found: $queue_keys"
        
        if [ "$queue_keys" -gt 0 ]; then
            echo "Recent queue keys:"
            etcdctl --endpoints=localhost:2379 get --prefix "/telemetry/queue/" --keys-only 2>/dev/null | tail -5
        fi
    else
        echo -e "${RED}❌ etcd is not accessible${NC}"
        echo "This could be why CSV processing is failing"
    fi
else
    echo -e "${YELLOW}⚠️  etcdctl not found, skipping etcd checks${NC}"
fi

# Check for upload directory
echo -e "\n${YELLOW}3. Upload Directory Check${NC}"
upload_dir="/tmp/uploads"  # Default upload directory
if [ -d "$upload_dir" ]; then
    echo -e "${GREEN}✅ Upload directory exists: $upload_dir${NC}"
    file_count=$(ls -la "$upload_dir" 2>/dev/null | wc -l)
    echo "Files in upload directory: $((file_count - 2))"  # Subtract . and ..
    
    if [ "$file_count" -gt 2 ]; then
        echo "Recent files:"
        ls -lat "$upload_dir" | head -5
    fi
else
    echo -e "${YELLOW}⚠️  Upload directory not found: $upload_dir${NC}"
    echo "Will be created automatically on first upload"
fi

# Check log level and suggest debugging
echo -e "\n${YELLOW}4. Debugging Suggestions${NC}"
echo "To get more detailed logs, ensure the service is running with:"
echo "  export LOG_LEVEL=debug"
echo "  go run cmd/nexus-streamer/main.go"
echo ""
echo "Key things to look for in logs:"
echo "  - 'Starting CSV upload and processing' - confirms upload started"
echo "  - 'File saved successfully with MD5' - confirms file was saved"
echo "  - 'Starting to process CSV file' - confirms processing started"
echo "  - 'CSV validation successful' - confirms headers are valid"
echo "  - 'Processing batch X with Y records' - confirms batch processing"
echo "  - 'Published batch of X telemetry records to etcd queue' - confirms etcd write"

# Test a simple upload if user provides a file
if [ $# -eq 1 ] && [ -f "$1" ]; then
    echo -e "\n${YELLOW}5. Testing Upload of Provided File${NC}"
    csv_file="$1"
    echo "Testing upload of: $csv_file"
    
    # Check file format
    if [ ! -f "$csv_file" ]; then
        echo -e "${RED}❌ File not found: $csv_file${NC}"
        exit 1
    fi
    
    if ! head -1 "$csv_file" | grep -q "timestamp"; then
        echo -e "${RED}❌ File doesn't appear to have required 'timestamp' header${NC}"
        echo "First line: $(head -1 "$csv_file")"
        exit 1
    fi
    
    echo "File size: $(stat -f%z "$csv_file" 2>/dev/null || stat -c%s "$csv_file" 2>/dev/null) bytes"
    echo "Record count (approx): $(($(wc -l < "$csv_file") - 1))"
    echo "MD5 hash: $(md5sum "$csv_file" 2>/dev/null | cut -d' ' -f1 || md5 -q "$csv_file" 2>/dev/null)"
    
    echo -e "\n${BLUE}Uploading file...${NC}"
    response=$(curl -s -X POST -F "file=@$csv_file" -F "description=Debug test upload" "$UPLOAD_ENDPOINT")
    echo "Response: $response"
    
    # Parse response
    success=$(echo "$response" | grep -o '"success":[^,}]*' | cut -d':' -f2 | tr -d ' "')
    if [ "$success" = "true" ]; then
        echo -e "${GREEN}✅ Upload successful${NC}"
        record_count=$(echo "$response" | grep -o '"record_count":[^,}]*' | cut -d':' -f2 | tr -d ' "')
        echo "Records processed: $record_count"
    else
        echo -e "${RED}❌ Upload failed${NC}"
        error=$(echo "$response" | grep -o '"error":"[^"]*"' | cut -d':' -f2- | tr -d '"')
        echo "Error: $error"
    fi
else
    echo -e "\n${BLUE}Usage: $0 [csv-file-to-test]${NC}"
    echo "Provide a CSV file path to test upload functionality"
fi

echo -e "\n${BLUE}Debug information collection complete.${NC}"
