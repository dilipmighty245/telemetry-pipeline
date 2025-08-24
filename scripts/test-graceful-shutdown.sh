#!/bin/bash

# Test script to verify graceful shutdown of collector and streamer services

set -e

echo "Testing graceful shutdown of telemetry pipeline services..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to test a service
test_service() {
    local service_name=$1
    local service_cmd=$2
    
    echo -e "\n${YELLOW}Testing $service_name graceful shutdown...${NC}"
    
    # Start the service in background
    echo "Starting $service_name..."
    $service_cmd &
    local pid=$!
    
    # Wait a bit for service to start
    sleep 3
    
    # Check if process is still running
    if ! kill -0 $pid 2>/dev/null; then
        echo -e "${RED}❌ $service_name failed to start${NC}"
        return 1
    fi
    
    echo "Service started with PID: $pid"
    
    # Send SIGTERM
    echo "Sending SIGTERM to $service_name (PID: $pid)..."
    kill -TERM $pid
    
    # Wait for graceful shutdown (max 30 seconds)
    local timeout=30
    local count=0
    while kill -0 $pid 2>/dev/null && [ $count -lt $timeout ]; do
        sleep 1
        count=$((count + 1))
        if [ $((count % 5)) -eq 0 ]; then
            echo "Waiting for graceful shutdown... ${count}s"
        fi
    done
    
    # Check if process has exited
    if kill -0 $pid 2>/dev/null; then
        echo -e "${RED}❌ $service_name did not shutdown gracefully within ${timeout}s${NC}"
        kill -9 $pid 2>/dev/null || true
        return 1
    else
        echo -e "${GREEN}✅ $service_name shutdown gracefully${NC}"
        return 0
    fi
}

# Set required environment variables
export ETCD_ENDPOINTS="localhost:2379"
export CLUSTER_ID="test-cluster"
export LOG_LEVEL="info"

# Test collector
collector_success=0
if test_service "nexus-collector" "go run cmd/nexus-collector/main.go"; then
    collector_success=1
fi

# Test streamer  
streamer_success=0
if test_service "nexus-streamer" "go run cmd/nexus-streamer/main.go"; then
    streamer_success=1
fi

# Summary
echo -e "\n${YELLOW}=== Test Results ===${NC}"
if [ $collector_success -eq 1 ]; then
    echo -e "${GREEN}✅ Collector: Graceful shutdown working${NC}"
else
    echo -e "${RED}❌ Collector: Graceful shutdown failed${NC}"
fi

if [ $streamer_success -eq 1 ]; then
    echo -e "${GREEN}✅ Streamer: Graceful shutdown working${NC}"
else
    echo -e "${RED}❌ Streamer: Graceful shutdown failed${NC}"
fi

# Overall result
if [ $collector_success -eq 1 ] && [ $streamer_success -eq 1 ]; then
    echo -e "\n${GREEN}🎉 All services support graceful shutdown!${NC}"
    exit 0
else
    echo -e "\n${RED}💥 Some services failed graceful shutdown test${NC}"
    exit 1
fi
