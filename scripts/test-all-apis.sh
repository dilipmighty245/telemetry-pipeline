#!/bin/bash

# Test script for all available APIs
# This script tests all REST endpoints to verify they're working

set -e

BASE_URL="${BASE_URL:-http://localhost:8080}"
CLUSTER_ID="${CLUSTER_ID:-local-cluster}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} ✅ $1"
}

error() {
    echo -e "${RED}[$(date +'%H:%M:%S')]${NC} ❌ $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')]${NC} ⚠️  $1"
}

# Function to test an endpoint
test_endpoint() {
    local method=$1
    local endpoint=$2
    local description=$3
    local expected_status=${4:-200}
    
    echo -n "Testing $method $endpoint ... "
    
    if [[ "$method" == "GET" ]]; then
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" "$BASE_URL$endpoint" 2>/dev/null || echo "HTTPSTATUS:000")
    else
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" "$BASE_URL$endpoint" 2>/dev/null || echo "HTTPSTATUS:000")
    fi
    
    http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [[ "$http_code" == "$expected_status" ]]; then
        success "$description (HTTP $http_code)"
        return 0
    else
        error "$description (HTTP $http_code)"
        if [[ ! -z "$body" && "$body" != "HTTPSTATUS"* ]]; then
            echo "Response: $body" | head -c 200
            echo "..."
        fi
        return 1
    fi
}

# Function to test GraphQL
test_graphql() {
    echo -n "Testing GraphQL endpoint ... "
    
    query='{"query": "{ gpus { id uuid hostname } }"}'
    response=$(curl -s -w "HTTPSTATUS:%{http_code}" \
        -X POST "$BASE_URL/graphql" \
        -H "Content-Type: application/json" \
        -d "$query" 2>/dev/null || echo "HTTPSTATUS:000")
    
    http_code=$(echo "$response" | grep -o "HTTPSTATUS:[0-9]*" | cut -d: -f2)
    
    if [[ "$http_code" == "200" ]]; then
        success "GraphQL API (HTTP $http_code)"
        return 0
    else
        error "GraphQL API (HTTP $http_code)"
        return 1
    fi
}

# Main test function
main() {
    log "🚀 Testing All Telemetry Pipeline APIs"
    log "Base URL: $BASE_URL"
    echo ""
    
    local passed=0
    local failed=0
    local total=0
    
    # Test core APIs (assignment requirements)
    log "📊 Core Telemetry APIs (Assignment Requirements)"
    echo "=================================================="
    
    if test_endpoint "GET" "/health" "Health check"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/api/v1/gpus" "List all GPUs"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    # Get a sample GPU UUID for telemetry testing
    gpu_uuid=$(curl -s "$BASE_URL/api/v1/gpus" | jq -r '.data[0].uuid' 2>/dev/null || echo "")
    if [[ ! -z "$gpu_uuid" && "$gpu_uuid" != "null" ]]; then
        if test_endpoint "GET" "/api/v1/gpus/$gpu_uuid/telemetry" "Query telemetry by GPU UUID"; then ((passed++)); else ((failed++)); fi
        ((total++))
        
        if test_endpoint "GET" "/api/v1/gpus/$gpu_uuid/telemetry?limit=5" "Query telemetry with limit"; then ((passed++)); else ((failed++)); fi
        ((total++))
        
        if test_endpoint "GET" "/api/v1/gpus/$gpu_uuid/telemetry?start_time=2025-07-18T20:42:34Z&end_time=2025-07-18T20:42:36Z" "Query telemetry with time range"; then ((passed++)); else ((failed++)); fi
        ((total++))
    else
        warn "No GPU UUID found, skipping telemetry tests"
        ((failed+=3))
        ((total+=3))
    fi
    
    echo ""
    log "🔍 Extended Hierarchical APIs"
    echo "============================="
    
    # Cluster APIs
    if test_endpoint "GET" "/api/v1/clusters" "List clusters"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID" "Get cluster details"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/stats" "Get cluster stats"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    # Host APIs
    if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/hosts" "List hosts in cluster"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/api/v1/hosts" "List all hosts (shortcut)"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    # Get a sample hostname for host-specific tests
    hostname=$(curl -s "$BASE_URL/api/v1/hosts" | jq -r '.data[0].hostname' 2>/dev/null || echo "")
    if [[ ! -z "$hostname" && "$hostname" != "null" ]]; then
        if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/hosts/$hostname" "Get host details"; then ((passed++)); else ((failed++)); fi
        ((total++))
        
        if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/hosts/$hostname/gpus" "List GPUs on host"; then ((passed++)); else ((failed++)); fi
        ((total++))
        
        if test_endpoint "GET" "/api/v1/hosts/$hostname/gpus" "List GPUs by hostname (shortcut)"; then ((passed++)); else ((failed++)); fi
        ((total++))
        
        # GPU-specific APIs
        gpu_id=$(curl -s "$BASE_URL/api/v1/hosts/$hostname/gpus" | jq -r '.data[0].id' 2>/dev/null || echo "")
        if [[ ! -z "$gpu_id" && "$gpu_id" != "null" ]]; then
            if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/hosts/$hostname/gpus/$gpu_id" "Get specific GPU details"; then ((passed++)); else ((failed++)); fi
            ((total++))
            
            if test_endpoint "GET" "/api/v1/clusters/$CLUSTER_ID/hosts/$hostname/gpus/$gpu_id/metrics" "Get GPU metrics"; then ((passed++)); else ((failed++)); fi
            ((total++))
        else
            warn "No GPU ID found for host $hostname, skipping GPU-specific tests"
            ((failed+=2))
            ((total+=2))
        fi
    else
        warn "No hostname found, skipping host-specific tests"
        ((failed+=5))
        ((total+=5))
    fi
    
    # Telemetry APIs
    if test_endpoint "GET" "/api/v1/telemetry" "Global telemetry data"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/api/v1/telemetry/latest" "Latest telemetry"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    echo ""
    log "🔍 GraphQL & Documentation APIs"
    echo "==============================="
    
    # GraphQL
    if test_graphql; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/graphql" "GraphQL Playground"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    # Documentation
    if test_endpoint "GET" "/swagger/" "Swagger UI"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/docs" "API docs redirect" "301"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/swagger/spec.json" "OpenAPI JSON spec"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    if test_endpoint "GET" "/swagger/spec.yaml" "OpenAPI YAML spec"; then ((passed++)); else ((failed++)); fi
    ((total++))
    
    echo ""
    log "📊 Test Results Summary"
    echo "======================"
    echo "Total endpoints tested: $total"
    success "Passed: $passed"
    if [[ $failed -gt 0 ]]; then
        error "Failed: $failed"
    else
        success "Failed: $failed"
    fi
    
    local success_rate=$((passed * 100 / total))
    echo "Success rate: $success_rate%"
    
    if [[ $failed -eq 0 ]]; then
        echo ""
        success "🎉 All API endpoints are working correctly!"
        return 0
    else
        echo ""
        error "❌ Some API endpoints failed. Check the output above for details."
        return 1
    fi
}

# Run main function
main "$@"
