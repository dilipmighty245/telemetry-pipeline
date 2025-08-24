#!/bin/bash

# Script to test time-based telemetry queries

set -e

echo "🕒 Testing Time-Based Telemetry Queries"
echo "======================================="

API_ENDPOINT=${API_ENDPOINT:-http://localhost:8080}

# Check if API is responding
if ! curl -s "$API_ENDPOINT/health" > /dev/null 2>&1; then
    echo "❌ API Gateway not responding at $API_ENDPOINT"
    echo "   Make sure nexus-gateway is running: make run-nexus-gateway"
    exit 1
fi

echo "✅ API Gateway is responding"
echo ""

# Get list of GPUs
echo "📋 Getting list of GPUs..."
GPU_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus" 2>/dev/null)
GPU_COUNT=$(echo "$GPU_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo "0")

if [[ "$GPU_COUNT" -eq 0 ]]; then
    echo "❌ No GPUs found. Make sure the pipeline is running and has processed data."
    exit 1
fi

echo "✅ Found $GPU_COUNT GPUs"

# Get first GPU UUID for testing
FIRST_GPU_UUID=$(echo "$GPU_RESPONSE" | jq -r '.data[0].uuid // empty' 2>/dev/null || echo "")

if [[ -z "$FIRST_GPU_UUID" ]]; then
    echo "❌ Could not extract GPU UUID from response"
    exit 1
fi

echo "🎯 Testing with GPU: $FIRST_GPU_UUID"
echo ""

# Test 1: Get all telemetry for the GPU
echo "📊 Test 1: Get all telemetry for GPU"
echo "Query: GET /api/v1/gpus/$FIRST_GPU_UUID/telemetry"
ALL_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus/$FIRST_GPU_UUID/telemetry" 2>/dev/null)
ALL_COUNT=$(echo "$ALL_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo "0")
echo "Result: $ALL_COUNT total records"
echo ""

if [[ "$ALL_COUNT" -eq 0 ]]; then
    echo "⚠️  No telemetry data found. The streamer may not have run yet."
    echo "   Run: make run-nexus-streamer"
    echo ""
fi

# Test 2: Get telemetry in specific time range (first 2 seconds of CSV data)
echo "📊 Test 2: Get telemetry in time range (2025-07-18T20:42:34Z to 2025-07-18T20:42:36Z)"
START_TIME="2025-07-18T20:42:34Z"
END_TIME="2025-07-18T20:42:36Z"
QUERY_URL="$API_ENDPOINT/api/v1/gpus/$FIRST_GPU_UUID/telemetry?start_time=$START_TIME&end_time=$END_TIME"
echo "Query: GET /api/v1/gpus/$FIRST_GPU_UUID/telemetry?start_time=$START_TIME&end_time=$END_TIME"

TIME_RESPONSE=$(curl -s "$QUERY_URL" 2>/dev/null)
TIME_COUNT=$(echo "$TIME_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo "0")
echo "Result: $TIME_COUNT records in time range"
echo ""

# Test 3: Get telemetry with limit
echo "📊 Test 3: Get telemetry with limit (first 10 records)"
echo "Query: GET /api/v1/gpus/$FIRST_GPU_UUID/telemetry?limit=10"
LIMITED_RESPONSE=$(curl -s "$API_ENDPOINT/api/v1/gpus/$FIRST_GPU_UUID/telemetry?limit=10" 2>/dev/null)
LIMITED_COUNT=$(echo "$LIMITED_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo "0")
echo "Result: $LIMITED_COUNT records (limited to 10)"
echo ""

# Test 4: Get telemetry in narrow time range (1 second)
echo "📊 Test 4: Get telemetry in narrow time range (1 second window)"
NARROW_END="2025-07-18T20:42:35Z"
NARROW_QUERY_URL="$API_ENDPOINT/api/v1/gpus/$FIRST_GPU_UUID/telemetry?start_time=$START_TIME&end_time=$NARROW_END"
echo "Query: GET /api/v1/gpus/$FIRST_GPU_UUID/telemetry?start_time=$START_TIME&end_time=$NARROW_END"

NARROW_RESPONSE=$(curl -s "$NARROW_QUERY_URL" 2>/dev/null)
NARROW_COUNT=$(echo "$NARROW_RESPONSE" | jq -r '.count // 0' 2>/dev/null || echo "0")
echo "Result: $NARROW_COUNT records in 1-second window"
echo ""

# Show sample data if available
if [[ "$ALL_COUNT" -gt 0 ]]; then
    echo "📄 Sample telemetry record:"
    echo "$ALL_RESPONSE" | jq -r '.data[0] // empty' 2>/dev/null | head -10
    echo ""
fi

# Summary
echo "📈 Summary:"
echo "==========="
echo "• Total records for GPU: $ALL_COUNT"
echo "• Records in 2-second range: $TIME_COUNT"  
echo "• Records in 1-second range: $NARROW_COUNT"
echo "• Limited query result: $LIMITED_COUNT"
echo ""

if [[ "$ALL_COUNT" -gt 0 ]]; then
    echo "✅ Time-based querying is working correctly!"
    
    # Show time range of actual data
    if command -v jq &> /dev/null; then
        FIRST_TIMESTAMP=$(echo "$ALL_RESPONSE" | jq -r '.data[0].timestamp // empty' 2>/dev/null)
        LAST_TIMESTAMP=$(echo "$ALL_RESPONSE" | jq -r '.data[-1].timestamp // empty' 2>/dev/null)
        
        if [[ -n "$FIRST_TIMESTAMP" && -n "$LAST_TIMESTAMP" ]]; then
            echo ""
            echo "⏰ Actual data time range:"
            echo "   First record: $FIRST_TIMESTAMP"
            echo "   Last record:  $LAST_TIMESTAMP"
        fi
    fi
else
    echo "⚠️  No telemetry data available for testing."
    echo "   Make sure to run the streamer: make run-nexus-streamer"
fi

echo ""
echo "🔗 Manual test commands:"
echo "curl \"$API_ENDPOINT/api/v1/gpus\""
echo "curl \"$API_ENDPOINT/api/v1/gpus/$FIRST_GPU_UUID/telemetry\""
echo "curl \"$QUERY_URL\""
