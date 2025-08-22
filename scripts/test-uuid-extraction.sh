#!/bin/bash

# Script to test UUID extraction with just a few records

set -e

echo "🧪 Testing UUID Extraction from CSV"
echo "==================================="

# Create a test CSV with just first 5 records
echo "📝 Creating test CSV with first 5 records..."
head -6 dcgm_metrics_20250718_134233.csv > test_sample.csv
echo "✅ Created test_sample.csv with $(wc -l < test_sample.csv) lines"

# Show what we're testing with
echo ""
echo "📊 Sample data:"
echo "Header:"
head -1 test_sample.csv
echo ""
echo "First record:"
head -2 test_sample.csv | tail -1 | cut -d',' -f1,3,4,5,6,7
echo ""

# Build the components
echo "🔨 Building components..."
make build-nexus-streamer build-nexus-collector build-nexus-gateway

echo ""
echo "🚀 Ready to test! Run these commands in separate terminals:"
echo ""
echo "# Terminal 1 - Start collector with debug logging"
echo "LOG_LEVEL=debug make run-nexus-collector"
echo ""
echo "# Terminal 2 - Start API gateway"
echo "make run-nexus-gateway"
echo ""
echo "# Terminal 3 - Stream test CSV"
echo "CSV_FILE=test_sample.csv LOG_LEVEL=debug make run-nexus-streamer"
echo ""
echo "# Terminal 4 - Check results"
echo "curl http://localhost:8080/api/v1/gpus | jq ."
echo ""
echo "Expected: GPUs should have proper UUIDs like 'GPU-5fd4f087-86f3-7a43-b711-4771313afc50'"
