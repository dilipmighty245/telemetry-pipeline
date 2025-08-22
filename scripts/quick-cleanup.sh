#!/bin/bash

# Quick Cleanup Script - Kill processes and flush etcd
echo "🧹 Quick cleanup starting..."

# Kill all nexus processes
echo "Killing nexus processes..."
pkill -f "nexus-streamer" 2>/dev/null || true
pkill -f "nexus-collector" 2>/dev/null || true  
pkill -f "nexus-gateway" 2>/dev/null || true
pkill -f "go run.*nexus-" 2>/dev/null || true
pkill -f "/tmp/nexus-" 2>/dev/null || true

# Clean up binaries
rm -f /tmp/nexus-*

# Flush etcd data
echo "Flushing etcd data..."
if docker ps --format "table {{.Names}}" | grep -q "telemetry-etcd"; then
    docker exec telemetry-etcd etcdctl del "/" --prefix 2>/dev/null || true
    echo "✅ etcd flushed"
else
    echo "⚠️  etcd container not running"
fi

echo "✅ Quick cleanup completed!"
