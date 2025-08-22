#!/bin/bash

# Local scaling script for telemetry pipeline components
# Usage: ./scripts/scale-local.sh [component] [instances]
# Example: ./scripts/scale-local.sh streamer 3

set -e

COMPONENT=${1:-"all"}
INSTANCES=${2:-2}
BASE_DIR=$(dirname "$(dirname "$(realpath "$0")")")
PID_DIR="$BASE_DIR/.pids"

# Create PID directory
mkdir -p "$PID_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}[$(date +'%H:%M:%S')]${NC} ✅ $1"
}

warn() {
    echo -e "${YELLOW}[$(date +'%H:%M:%S')]${NC} ⚠️  $1"
}

error() {
    echo -e "${RED}[$(date +'%H:%M:%S')]${NC} ❌ $1"
}

# Function to stop all instances of a component
stop_component() {
    local component=$1
    log "Stopping all $component instances..."
    
    # Kill processes by pattern
    pkill -f "nexus-$component" || true
    
    # Remove PID files
    rm -f "$PID_DIR/$component-"*.pid
    
    success "Stopped all $component instances"
}

# Function to start multiple instances of a component
start_streamers() {
    local instances=$1
    log "Starting $instances streamer instances..."
    
    for i in $(seq 1 $instances); do
        local streamer_id="streamer-$i"
        local pid_file="$PID_DIR/streamer-$i.pid"
        local log_file="$BASE_DIR/logs/streamer-$i.log"
        
        # Create logs directory
        mkdir -p "$BASE_DIR/logs"
        
        # Start streamer with unique ID
        CLUSTER_ID=local-cluster \
        ETCD_ENDPOINTS=localhost:2379 \
        CSV_FILE="${CSV_FILE:-dcgm_metrics_20250718_134233.csv}" \
        STREAMER_ID="$streamer_id" \
        LOG_LEVEL="${LOG_LEVEL:-info}" \
        nohup "$BASE_DIR/bin/nexus-streamer" > "$log_file" 2>&1 &
        
        local pid=$!
        echo $pid > "$pid_file"
        
        success "Started streamer instance $i (PID: $pid, ID: $streamer_id)"
        sleep 1 # Stagger startup
    done
}

start_collectors() {
    local instances=$1
    log "Starting $instances collector instances..."
    
    for i in $(seq 1 $instances); do
        local collector_id="collector-$(date +%s)-$i"
        local pid_file="$PID_DIR/collector-$i.pid"
        local log_file="$BASE_DIR/logs/collector-$i.log"
        
        # Create logs directory
        mkdir -p "$BASE_DIR/logs"
        
        # Start collector with unique ID
        CLUSTER_ID=local-cluster \
        ETCD_ENDPOINTS=localhost:2379 \
        COLLECTOR_ID="$collector_id" \
        LOG_LEVEL="${LOG_LEVEL:-info}" \
        nohup "$BASE_DIR/bin/nexus-collector" > "$log_file" 2>&1 &
        
        local pid=$!
        echo $pid > "$pid_file"
        
        success "Started collector instance $i (PID: $pid, ID: $collector_id)"
        sleep 1 # Stagger startup
    done
}

start_gateways() {
    local instances=$1
    log "Starting $instances gateway instances..."
    
    for i in $(seq 1 $instances); do
        local port=$((8080 + i - 1))
        local pid_file="$PID_DIR/gateway-$i.pid"
        local log_file="$BASE_DIR/logs/gateway-$i.log"
        
        # Create logs directory
        mkdir -p "$BASE_DIR/logs"
        
        # Start gateway on different ports
        CLUSTER_ID=local-cluster \
        ETCD_ENDPOINTS=localhost:2379 \
        PORT="$port" \
        ENABLE_GRAPHQL=true \
        LOG_LEVEL="${LOG_LEVEL:-info}" \
        nohup "$BASE_DIR/bin/nexus-gateway" > "$log_file" 2>&1 &
        
        local pid=$!
        echo $pid > "$pid_file"
        
        success "Started gateway instance $i (PID: $pid, Port: $port)"
        echo "  • REST API: http://localhost:$port"
        echo "  • GraphQL: http://localhost:$port/graphql"
        echo "  • WebSocket: ws://localhost:$port/ws"
        sleep 2 # Stagger startup more for gateways
    done
}

# Function to show status of running instances
show_status() {
    log "Component Status:"
    echo ""
    
    # Check streamers
    local streamer_count=0
    for pid_file in "$PID_DIR"/streamer-*.pid; do
        if [[ -f "$pid_file" ]]; then
            local pid=$(cat "$pid_file")
            if ps -p "$pid" > /dev/null 2>&1; then
                streamer_count=$((streamer_count + 1))
            else
                rm -f "$pid_file"
            fi
        fi
    done
    echo "📤 Streamers: $streamer_count running"
    
    # Check collectors
    local collector_count=0
    for pid_file in "$PID_DIR"/collector-*.pid; do
        if [[ -f "$pid_file" ]]; then
            local pid=$(cat "$pid_file")
            if ps -p "$pid" > /dev/null 2>&1; then
                collector_count=$((collector_count + 1))
            else
                rm -f "$pid_file"
            fi
        fi
    done
    echo "📥 Collectors: $collector_count running"
    
    # Check gateways
    local gateway_count=0
    for pid_file in "$PID_DIR"/gateway-*.pid; do
        if [[ -f "$pid_file" ]]; then
            local pid=$(cat "$pid_file")
            if ps -p "$pid" > /dev/null 2>&1; then
                gateway_count=$((gateway_count + 1))
            else
                rm -f "$pid_file"
            fi
        fi
    done
    echo "🌐 Gateways: $gateway_count running"
    
    echo ""
    if [[ $streamer_count -gt 0 || $collector_count -gt 0 || $gateway_count -gt 0 ]]; then
        success "Pipeline is running with $((streamer_count + collector_count + gateway_count)) total instances"
        echo ""
        echo "📊 To monitor:"
        echo "  • Logs: tail -f $BASE_DIR/logs/*.log"
        echo "  • Status: ./scripts/scale-local.sh status"
        echo "  • Stop all: ./scripts/scale-local.sh stop"
    else
        warn "No instances are currently running"
    fi
}

# Function to stop all components
stop_all() {
    log "Stopping all pipeline components..."
    stop_component "streamer"
    stop_component "collector" 
    stop_component "gateway"
    
    # Clean up log files older than 1 hour
    find "$BASE_DIR/logs" -name "*.log" -mmin +60 -delete 2>/dev/null || true
    
    success "All components stopped"
}

# Main script logic
case "$COMPONENT" in
    "streamer")
        if [[ "$INSTANCES" == "0" ]]; then
            stop_component "streamer"
        else
            stop_component "streamer"
            start_streamers "$INSTANCES"
        fi
        ;;
    "collector")
        if [[ "$INSTANCES" == "0" ]]; then
            stop_component "collector"
        else
            stop_component "collector"
            start_collectors "$INSTANCES"
        fi
        ;;
    "gateway")
        if [[ "$INSTANCES" == "0" ]]; then
            stop_component "gateway"
        else
            stop_component "gateway"
            start_gateways "$INSTANCES"
        fi
        ;;
    "all")
        if [[ "$INSTANCES" == "0" ]]; then
            stop_all
        else
            log "🚀 Starting scaled pipeline with $INSTANCES instances of each component"
            echo ""
            
            # Build all components first
            log "Building all components..."
            cd "$BASE_DIR"
            make build-nexus-streamer build-nexus-collector build-nexus-gateway
            
            # Stop existing instances
            stop_all
            
            # Start scaled instances
            start_collectors "$INSTANCES"
            sleep 2
            start_gateways "$INSTANCES"
            sleep 2
            start_streamers "$INSTANCES"
            
            echo ""
            success "Scaled pipeline started successfully!"
            show_status
        fi
        ;;
    "status")
        show_status
        ;;
    "stop")
        stop_all
        ;;
    "logs")
        log "Showing recent logs from all components..."
        if [[ -d "$BASE_DIR/logs" ]]; then
            tail -f "$BASE_DIR/logs"/*.log
        else
            warn "No log files found in $BASE_DIR/logs"
        fi
        ;;
    *)
        echo "Usage: $0 [component] [instances]"
        echo ""
        echo "Components:"
        echo "  streamer   - Scale telemetry streamers"
        echo "  collector  - Scale telemetry collectors" 
        echo "  gateway    - Scale API gateways"
        echo "  all        - Scale all components"
        echo ""
        echo "Commands:"
        echo "  status     - Show current status"
        echo "  stop       - Stop all components"
        echo "  logs       - Follow logs from all components"
        echo ""
        echo "Examples:"
        echo "  $0 all 3           # Start 3 instances of each component"
        echo "  $0 streamer 5      # Scale streamers to 5 instances"
        echo "  $0 collector 2     # Scale collectors to 2 instances"
        echo "  $0 stop            # Stop all components"
        echo "  $0 status          # Show status"
        exit 1
        ;;
esac
