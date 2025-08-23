#!/bin/bash

# Demo script for CSV Upload API functionality
# This script demonstrates the streamlined CSV upload and processing

set -e

# Configuration
GATEWAY_URL="http://localhost:8080"
API_BASE="${GATEWAY_URL}/api/v1"
DEMO_CSV_FILE="demo_telemetry.csv"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Function to check if gateway is running
check_gateway() {
    print_status "Checking if Nexus Gateway is running..."
    if curl -s "${GATEWAY_URL}/health" > /dev/null; then
        print_success "Gateway is running at ${GATEWAY_URL}"
    else
        print_error "Gateway is not running at ${GATEWAY_URL}"
        print_status "Please start the gateway with: go run cmd/nexus-gateway/main.go"
        exit 1
    fi
}

# Function to create demo CSV file
create_demo_csv() {
    print_status "Creating demo CSV file: ${DEMO_CSV_FILE}"
    
    cat > "${DEMO_CSV_FILE}" << 'EOF'
timestamp,gpu_id,hostname,uuid,device,modelName,gpu_utilization,memory_utilization,memory_used_mb,memory_free_mb,temperature,power_draw,sm_clock_mhz,memory_clock_mhz
2025-01-18T10:00:00Z,0,demo-host-001,GPU-12345678-1234-1234-1234-123456789abc,nvidia0,NVIDIA H100 80GB HBM3,85.5,72.3,58000,22000,65.2,350.8,1410,1215
2025-01-18T10:00:01Z,1,demo-host-001,GPU-87654321-4321-4321-4321-cba987654321,nvidia1,NVIDIA H100 80GB HBM3,92.1,68.7,55000,25000,67.8,380.2,1420,1220
2025-01-18T10:00:02Z,0,demo-host-002,GPU-abcdef12-3456-7890-abcd-ef1234567890,nvidia0,NVIDIA A100 80GB PCIe,78.9,75.2,60000,20000,62.5,320.5,1395,1200
2025-01-18T10:00:03Z,1,demo-host-002,GPU-fedcba98-7654-3210-fedc-ba9876543210,nvidia1,NVIDIA A100 80GB PCIe,88.3,71.8,57500,22500,64.1,340.7,1405,1210
2025-01-18T10:00:04Z,0,demo-host-003,GPU-11111111-2222-3333-4444-555555555555,nvidia0,NVIDIA RTX 4090,95.2,82.4,22000,2000,71.3,420.1,2520,1313
EOF

    print_success "Demo CSV file created with 5 sample records"
}

# Function to upload and process CSV file
upload_and_process_csv() {
    print_status "Uploading and processing CSV file..."
    
    UPLOAD_RESPONSE=$(curl -s -X POST \
        -F "file=@${DEMO_CSV_FILE}" \
        "${API_BASE}/csv/upload")
    
    if echo "${UPLOAD_RESPONSE}" | jq -e '.success' > /dev/null 2>&1; then
        TOTAL_RECORDS=$(echo "${UPLOAD_RESPONSE}" | jq -r '.total_records')
        PROCESSED_RECORDS=$(echo "${UPLOAD_RESPONSE}" | jq -r '.processed_records')
        SKIPPED_RECORDS=$(echo "${UPLOAD_RESPONSE}" | jq -r '.skipped_records')
        PROCESSING_TIME=$(echo "${UPLOAD_RESPONSE}" | jq -r '.processing_time')
        RECORDS_PER_SEC=$(echo "${UPLOAD_RESPONSE}" | jq -r '.records_per_second')
        FILE_SIZE=$(echo "${UPLOAD_RESPONSE}" | jq -r '.size')
        
        print_success "File processed successfully!"
        echo "  Filename: $(echo "${UPLOAD_RESPONSE}" | jq -r '.filename')"
        echo "  Size: ${FILE_SIZE} bytes"
        echo "  Total Records: ${TOTAL_RECORDS}"
        echo "  Processed Records: ${PROCESSED_RECORDS}"
        echo "  Skipped Records: ${SKIPPED_RECORDS}"
        echo "  Processing Time: ${PROCESSING_TIME}"
        echo "  Records/Second: ${RECORDS_PER_SEC}"
        echo "  Status: $(echo "${UPLOAD_RESPONSE}" | jq -r '.status')"
        echo "  Message: $(echo "${UPLOAD_RESPONSE}" | jq -r '.message')"
    else
        print_error "Upload and processing failed:"
        echo "${UPLOAD_RESPONSE}" | jq '.'
        exit 1
    fi
}



# Function to test error cases
test_error_cases() {
    print_status "Testing error cases..."
    
    # Test 1: Upload invalid file
    echo "invalid,csv,content" > invalid.csv
    print_status "Testing upload with invalid CSV (missing required headers)..."
    
    INVALID_RESPONSE=$(curl -s -X POST \
        -F "file=@invalid.csv" \
        "${API_BASE}/csv/upload")
    
    if echo "${INVALID_RESPONSE}" | jq -e '.success == false' > /dev/null 2>&1; then
        print_success "✓ Correctly rejected invalid CSV file"
        ERROR_MSG=$(echo "${INVALID_RESPONSE}" | jq -r '.error')
        echo "  Error: ${ERROR_MSG}"
    else
        print_warning "Invalid CSV was not properly rejected"
    fi
    
    rm -f invalid.csv
}

# Function to cleanup
cleanup() {
    print_status "Cleaning up demo files..."
    
    # Remove local files (no server files to clean up since we don't store them)
    rm -f "${DEMO_CSV_FILE}"
    
    print_success "Cleanup completed"
}

# Function to show API documentation
show_api_docs() {
    print_status "Streamlined CSV Upload API:"
    echo ""
    echo "📤 Upload and Process CSV File (Single Endpoint):"
    echo "  POST ${API_BASE}/csv/upload"
    echo "  - Form data: file (required)"
    echo "  - Immediately processes and streams data to telemetry pipeline"
    echo "  - No file storage - memory efficient"
    echo ""
    echo "Example usage:"
    echo "  curl -X POST -F \"file=@data.csv\" ${API_BASE}/csv/upload"
    echo ""
    echo "📚 API Documentation:"
    echo "  ${GATEWAY_URL}/swagger/"
    echo ""
}

# Main execution
main() {
    echo "🚀 CSV Upload API Demo Script"
    echo "================================"
    echo ""
    
    case "${1:-demo}" in
        "demo")
            check_gateway
            create_demo_csv
            upload_and_process_csv
            test_error_cases
            echo ""
            print_success "Demo completed successfully! 🎉"
            echo ""
            show_api_docs
            ;;
        "cleanup")
            cleanup
            ;;
        "docs")
            show_api_docs
            ;;
        "help"|"-h"|"--help")
            echo "Usage: $0 [command]"
            echo ""
            echo "Commands:"
            echo "  demo     - Run full demonstration (default)"
            echo "  cleanup  - Clean up demo files"
            echo "  docs     - Show API documentation"
            echo "  help     - Show this help message"
            echo ""
            ;;
        *)
            print_error "Unknown command: $1"
            echo "Use '$0 help' for usage information"
            exit 1
            ;;
    esac
}

# Trap cleanup on exit
trap cleanup EXIT

# Run main function
main "$@"
