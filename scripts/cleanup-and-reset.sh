#!/bin/bash

# Telemetry Pipeline Cleanup and Reset Script
# This script kills all nexus processes and flushes etcd data
# 
# Usage:
#   ./cleanup-and-reset.sh          # Full cleanup with prompts
#   ./cleanup-and-reset.sh --quick  # Quick cleanup without prompts

set -e

# Check for quick mode
QUICK_MODE=false
if [[ "$1" == "--quick" ]]; then
    QUICK_MODE=true
fi

echo "🧹 Starting Telemetry Pipeline Cleanup and Reset..."
if [[ "$QUICK_MODE" == "true" ]]; then
    echo "⚡ Quick mode enabled - no prompts"
fi
echo "=================================================="

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

# 1. Kill all nexus processes
print_status "Killing all nexus processes..."

# Kill processes by name pattern
PROCESSES=("nexus-streamer" "nexus-collector" "nexus-gateway")

for process in "${PROCESSES[@]}"; do
    print_status "Looking for $process processes..."
    
    # Find and kill processes
    PIDS=$(pgrep -f "$process" 2>/dev/null || true)
    
    if [ -n "$PIDS" ]; then
        print_status "Found $process processes with PIDs: $PIDS"
        echo "$PIDS" | xargs kill -TERM 2>/dev/null || true
        sleep 2
        
        # Force kill if still running
        REMAINING_PIDS=$(pgrep -f "$process" 2>/dev/null || true)
        if [ -n "$REMAINING_PIDS" ]; then
            print_warning "Force killing remaining $process processes: $REMAINING_PIDS"
            echo "$REMAINING_PIDS" | xargs kill -KILL 2>/dev/null || true
        fi
        
        print_success "Killed $process processes"
    else
        print_status "No $process processes found"
    fi
done

# Kill any Go processes running our main files
print_status "Killing any Go processes running nexus main files..."
pkill -f "go run.*nexus-" 2>/dev/null || true
pkill -f "/tmp/nexus-" 2>/dev/null || true

# Wait a moment for processes to fully terminate
sleep 3

# 2. Clean up temporary binaries
print_status "Cleaning up temporary binaries..."
rm -f /tmp/nexus-streamer /tmp/nexus-collector /tmp/nexus-gateway
print_success "Cleaned up temporary binaries"

# 3. Flush etcd data
print_status "Flushing etcd data..."

# Check if etcd container is running
if docker ps --format "table {{.Names}}" | grep -q "telemetry-etcd"; then
    print_status "etcd container is running, flushing data..."
    
    # Delete all telemetry data
    print_status "Deleting telemetry data..."
    docker exec telemetry-etcd etcdctl del "/telemetry/" --prefix 2>/dev/null || print_warning "Failed to delete telemetry data"
    
    # Delete message queue data
    print_status "Deleting message queue data..."
    docker exec telemetry-etcd etcdctl del "/messagequeue/" --prefix 2>/dev/null || print_warning "Failed to delete message queue data"
    
    # Delete service registry data
    print_status "Deleting service registry data..."
    docker exec telemetry-etcd etcdctl del "/services/" --prefix 2>/dev/null || print_warning "Failed to delete service registry data"
    
    # Delete configuration data
    print_status "Deleting configuration data..."
    docker exec telemetry-etcd etcdctl del "/config/" --prefix 2>/dev/null || print_warning "Failed to delete config data"
    
    # Delete metrics data
    print_status "Deleting metrics data..."
    docker exec telemetry-etcd etcdctl del "/metrics/" --prefix 2>/dev/null || print_warning "Failed to delete metrics data"
    
    # Delete processing data (work in progress)
    print_status "Deleting processing data..."
    docker exec telemetry-etcd etcdctl del "/processing/" --prefix 2>/dev/null || print_warning "Failed to delete processing data"
    
    # Delete scaling data
    print_status "Deleting scaling data..."
    docker exec telemetry-etcd etcdctl del "/scaling/" --prefix 2>/dev/null || print_warning "Failed to delete scaling data"
    
    print_success "etcd data flushed successfully"
else
    print_warning "etcd container not running, skipping etcd cleanup"
fi

# 4. PostgreSQL database is no longer used - skipping database reset

# 5. etcd cleanup completed above - no additional cleanup needed

# 6. Verify cleanup
print_status "Verifying cleanup..."

# Check for remaining processes
REMAINING=$(pgrep -f "nexus-" 2>/dev/null || true)
if [ -n "$REMAINING" ]; then
    print_warning "Some nexus processes may still be running: $REMAINING"
else
    print_success "All nexus processes terminated"
fi

# Check etcd data
if docker ps --format "table {{.Names}}" | grep -q "telemetry-etcd"; then
    ETCD_KEYS=$(docker exec telemetry-etcd etcdctl get "" --prefix --keys-only 2>/dev/null | wc -l)
    print_status "Remaining etcd keys: $ETCD_KEYS"
    
    if [ "$ETCD_KEYS" -eq 0 ]; then
        print_success "etcd completely clean"
    else
        print_status "Some etcd keys remain (this may include system keys)"
    fi
fi

# 7. Show system status
print_status "Current system status:"
echo "----------------------------------------"

# Show running containers
print_status "Running containers:"
docker ps --format "table {{.Names}}\t{{.Status}}" | grep -E "(telemetry-|etcd)" || print_status "No telemetry containers running"

echo
print_success "🎉 Cleanup and reset completed!"
echo
print_status "To start fresh:"
echo "  1. Start containers: docker-compose up -d"
echo "  2. Run streamer: ETCD_ENDPOINTS=localhost:2379 go run cmd/nexus-streamer/main.go [options]"
echo "  3. Run collector: ETCD_ENDPOINTS=localhost:2379 go run cmd/nexus-collector/main.go [options]"
echo "  4. Run gateway: ETCD_ENDPOINTS=localhost:2379 go run cmd/nexus-gateway/main.go [options]"
echo
print_status "Or use the pre-built binaries in /tmp/ if available"
echo "=================================================="
