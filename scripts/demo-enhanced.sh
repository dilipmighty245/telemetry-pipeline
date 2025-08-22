#!/bin/bash

# Enhanced Telemetry Pipeline Demo Script
# This script demonstrates the enhanced telemetry pipeline with resilience features

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
ETCD_PORT=2379
GATEWAY_PORT=8080
CSV_FILE="$PROJECT_ROOT/dcgm_metrics_20250718_134233.csv"

echo -e "${BLUE}🚀 Enhanced Telemetry Pipeline Demo${NC}"
echo "=================================================="

# Check if binaries exist
check_binary() {
    if [ ! -f "$BIN_DIR/$1" ]; then
        echo -e "${RED}❌ Binary $1 not found. Please run 'make build-enhanced' first.${NC}"
        exit 1
    fi
}

echo -e "${YELLOW}📋 Checking prerequisites...${NC}"
check_binary "enhanced-collector"
check_binary "enhanced-streamer" 
check_binary "enhanced-gateway"

# Check if etcd is running
check_etcd() {
    if ! curl -s http://localhost:$ETCD_PORT/health > /dev/null 2>&1; then
        echo -e "${RED}❌ etcd is not running on port $ETCD_PORT${NC}"
        echo "Please start etcd first:"
        echo "  docker run -d --name etcd-demo -p 2379:2379 -p 2380:2380 \\"
        echo "    quay.io/coreos/etcd:v3.5.0 \\"
        echo "    /usr/local/bin/etcd \\"
        echo "    --data-dir=/etcd-data --name node1 \\"
        echo "    --initial-advertise-peer-urls http://localhost:2380 \\"
        echo "    --listen-peer-urls http://0.0.0.0:2380 \\"
        echo "    --advertise-client-urls http://localhost:2379 \\"
        echo "    --listen-client-urls http://0.0.0.0:2379 \\"
        echo "    --initial-cluster node1=http://localhost:2380"
        exit 1
    fi
    echo -e "${GREEN}✅ etcd is running${NC}"
}

check_etcd

# Check if CSV file exists
if [ ! -f "$CSV_FILE" ]; then
    echo -e "${RED}❌ CSV file not found: $CSV_FILE${NC}"
    echo "Please ensure the CSV file exists for the demo."
    exit 1
fi
echo -e "${GREEN}✅ CSV file found: $CSV_FILE${NC}"

# Set up environment variables for enhanced components
export ETCD_ENDPOINTS="localhost:2379"
export ETCD_DIAL_TIMEOUT="10s"
export MQ_BATCH_SIZE="50"
export MQ_MAX_RETRIES="3"
export MQ_PROCESSING_TIMEOUT="30s"
export COLLECTOR_WORKER_COUNT="2"
export COLLECTOR_BATCH_SIZE="100"
export STREAMER_BATCH_SIZE="25"
export STREAMER_STREAM_INTERVAL="2s"
export THROTTLE_ENABLED="true"
export THROTTLE_MAX_QUEUE_DEPTH="1000"
export THROTTLE_RATE="0.8"
export GATEWAY_PORT="8080"
export WS_ENABLED="true"
export WS_MAX_CONNECTIONS="100"
export LOG_LEVEL="info"

echo -e "${YELLOW}🔧 Configuration:${NC}"
echo "  - etcd: $ETCD_ENDPOINTS"
echo "  - Batch size: $MQ_BATCH_SIZE"
echo "  - Worker count: $COLLECTOR_WORKER_COUNT" 
echo "  - Throttling: $THROTTLE_ENABLED (max depth: $THROTTLE_MAX_QUEUE_DEPTH)"
echo "  - Gateway port: $GATEWAY_PORT"
echo ""

# Function to start component in background
start_component() {
    local name=$1
    local binary=$2
    local args=$3
    local pid_file="/tmp/${name}.pid"
    local log_file="/tmp/${name}.log"
    
    echo -e "${BLUE}🚀 Starting $name...${NC}"
    
    # Kill existing process if running
    if [ -f "$pid_file" ]; then
        local old_pid=$(cat "$pid_file")
        if ps -p "$old_pid" > /dev/null 2>&1; then
            echo "  Stopping existing $name (PID: $old_pid)"
            kill "$old_pid" 2>/dev/null || true
            sleep 2
        fi
    fi
    
    # Start new process
    cd "$PROJECT_ROOT"
    $BIN_DIR/$binary $args > "$log_file" 2>&1 &
    local pid=$!
    echo $pid > "$pid_file"
    
    echo -e "${GREEN}✅ $name started (PID: $pid, logs: $log_file)${NC}"
    sleep 2
}

# Function to stop component
stop_component() {
    local name=$1
    local pid_file="/tmp/${name}.pid"
    
    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if ps -p "$pid" > /dev/null 2>&1; then
            echo -e "${YELLOW}🛑 Stopping $name (PID: $pid)...${NC}"
            kill "$pid" 2>/dev/null || true
            sleep 3
            
            # Force kill if still running
            if ps -p "$pid" > /dev/null 2>&1; then
                echo "  Force killing $name..."
                kill -9 "$pid" 2>/dev/null || true
            fi
        fi
        rm -f "$pid_file"
    fi
}

# Function to show component status
show_status() {
    echo -e "${BLUE}📊 Component Status:${NC}"
    
    for component in enhanced-collector enhanced-streamer enhanced-gateway; do
        local pid_file="/tmp/${component}.pid"
        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file")
            if ps -p "$pid" > /dev/null 2>&1; then
                echo -e "  ${GREEN}✅ $component (PID: $pid)${NC}"
            else
                echo -e "  ${RED}❌ $component (not running)${NC}"
            fi
        else
            echo -e "  ${RED}❌ $component (not started)${NC}"
        fi
    done
    echo ""
}

# Function to show metrics
show_metrics() {
    echo -e "${BLUE}📈 Metrics:${NC}"
    
    # Try to get health status from gateway
    if curl -s http://localhost:$GATEWAY_PORT/api/v1/health > /dev/null 2>&1; then
        echo "  Gateway health:"
        curl -s http://localhost:$GATEWAY_PORT/api/v1/health | jq . 2>/dev/null || echo "  (health check response received)"
    else
        echo -e "  ${YELLOW}⚠️  Gateway not responding${NC}"
    fi
    
    echo ""
}

# Function to test API endpoints
test_api() {
    echo -e "${BLUE}🧪 Testing API endpoints:${NC}"
    
    local base_url="http://localhost:$GATEWAY_PORT/api/v1"
    
    # Test health endpoint
    echo "  Testing health endpoint..."
    if curl -s "$base_url/health" > /dev/null; then
        echo -e "    ${GREEN}✅ Health endpoint working${NC}"
    else
        echo -e "    ${RED}❌ Health endpoint failed${NC}"
    fi
    
    # Test GPUs endpoint
    echo "  Testing GPUs endpoint..."
    if curl -s "$base_url/gpus" > /dev/null; then
        echo -e "    ${GREEN}✅ GPUs endpoint working${NC}"
    else
        echo -e "    ${RED}❌ GPUs endpoint failed${NC}"
    fi
    
    echo ""
}

# Cleanup function
cleanup() {
    echo -e "${YELLOW}🧹 Cleaning up...${NC}"
    stop_component "enhanced-collector"
    stop_component "enhanced-streamer" 
    stop_component "enhanced-gateway"
    echo -e "${GREEN}✅ Cleanup complete${NC}"
}

# Set up signal handlers
trap cleanup EXIT
trap cleanup INT
trap cleanup TERM

# Main demo flow
echo -e "${BLUE}🎬 Starting Enhanced Telemetry Pipeline Demo${NC}"
echo ""

# Start components in order
start_component "enhanced-collector" "enhanced-collector" ""
start_component "enhanced-gateway" "enhanced-gateway" ""
start_component "enhanced-streamer" "enhanced-streamer" "--csv $CSV_FILE"

echo ""
echo -e "${GREEN}🎉 All components started!${NC}"
echo ""

# Show initial status
show_status

# Wait for components to initialize
echo -e "${YELLOW}⏳ Waiting for components to initialize (10 seconds)...${NC}"
sleep 10

# Show status and metrics
show_status
show_metrics

# Test API endpoints
test_api

# Show some usage examples
echo -e "${BLUE}💡 Usage Examples:${NC}"
echo ""
echo "1. List all GPUs:"
echo "   curl http://localhost:$GATEWAY_PORT/api/v1/gpus"
echo ""
echo "2. Get telemetry for GPU 0:"
echo "   curl http://localhost:$GATEWAY_PORT/api/v1/gpus/0/telemetry?limit=10"
echo ""
echo "3. WebSocket connection:"
echo "   wscat -c ws://localhost:$GATEWAY_PORT/api/v1/ws/telemetry"
echo ""
echo "4. View logs:"
echo "   tail -f /tmp/enhanced-collector.log"
echo "   tail -f /tmp/enhanced-streamer.log"
echo "   tail -f /tmp/enhanced-gateway.log"
echo ""

# Keep demo running
echo -e "${BLUE}🔄 Demo is running. Press Ctrl+C to stop.${NC}"
echo ""

# Monitor loop
while true; do
    sleep 30
    echo -e "${YELLOW}📊 Status update:${NC}"
    show_status
    show_metrics
done
