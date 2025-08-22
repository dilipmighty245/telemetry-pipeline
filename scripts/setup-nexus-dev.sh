#!/bin/bash

# Nexus Development Setup Script
# This script sets up a complete Nexus-enhanced telemetry pipeline development environment

set -e

echo "🚀 Setting up Nexus-Enhanced Telemetry Pipeline Development Environment"
echo "=================================================================="

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

# Check prerequisites
print_status "Checking prerequisites..."

# Check Go
if ! command -v go &> /dev/null; then
    print_error "Go is not installed. Please install Go 1.21+ and try again."
    exit 1
fi

GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
print_success "Go $GO_VERSION is installed"

# Check Docker
if ! command -v docker &> /dev/null; then
    print_error "Docker is not installed. Please install Docker and try again."
    exit 1
fi

print_success "Docker is installed"

# Check if we're in the right directory
if [[ ! -f "go.mod" ]] || [[ ! -d "cmd" ]]; then
    print_error "Please run this script from the telemetry-pipeline root directory"
    exit 1
fi

# Step 1: Install dependencies
print_status "Installing Go dependencies..."
go mod tidy
go mod download
print_success "Go dependencies installed"

# Step 2: Build all components
print_status "Building all components..."
make build
make build-nexus
print_success "All components built successfully"

# Step 3: Setup infrastructure
print_status "Setting up infrastructure..."

# Check if etcd is already running
if docker ps | grep -q "telemetry-etcd"; then
    print_warning "etcd container already running"
else
    print_status "Starting etcd..."
    docker run -d \
        --name telemetry-etcd \
        -p 2379:2379 \
        -p 2380:2380 \
        -e ETCD_NAME=telemetry-etcd \
        -e ETCD_DATA_DIR=/etcd-data \
        -e ETCD_LISTEN_CLIENT_URLS=http://0.0.0.0:2379 \
        -e ETCD_ADVERTISE_CLIENT_URLS=http://localhost:2379 \
        -e ETCD_LISTEN_PEER_URLS=http://0.0.0.0:2380 \
        -e ETCD_INITIAL_ADVERTISE_PEER_URLS=http://localhost:2380 \
        -e ETCD_INITIAL_CLUSTER=telemetry-etcd=http://localhost:2380 \
        -e ETCD_INITIAL_CLUSTER_TOKEN=telemetry-cluster \
        -e ETCD_INITIAL_CLUSTER_STATE=new \
        -e ALLOW_NONE_AUTHENTICATION=yes \
        quay.io/coreos/etcd:v3.5.10
    
    # Wait for etcd to be ready
    print_status "Waiting for etcd to be ready..."
    for i in {1..30}; do
        if docker exec telemetry-etcd etcdctl endpoint health &>/dev/null; then
            break
        fi
        sleep 1
    done
    print_success "etcd is running and healthy"
fi

# Check if Redis is already running
if docker ps | grep -q "telemetry-redis"; then
    print_warning "Redis container already running"
else
    print_status "Starting Redis..."
    docker run -d \
        --name telemetry-redis \
        -p 6379:6379 \
        redis:7-alpine redis-server --maxmemory 256mb --maxmemory-policy allkeys-lru
    
    # Wait for Redis to be ready
    print_status "Waiting for Redis to be ready..."
    for i in {1..30}; do
        if docker exec telemetry-redis redis-cli ping | grep -q "PONG"; then
            break
        fi
        sleep 1
    done
    print_success "Redis is running and healthy"
fi

# Check if PostgreSQL is already running
if docker ps | grep -q "telemetry-postgres"; then
    print_warning "PostgreSQL container already running"
else
    print_status "Starting PostgreSQL..."
    docker run -d \
        --name telemetry-postgres \
        -p 5433:5432 \
        -e POSTGRES_DB=telemetry \
        -e POSTGRES_USER=postgres \
        -e POSTGRES_PASSWORD=postgres \
        postgres:15-alpine
    
    # Wait for PostgreSQL to be ready
    print_status "Waiting for PostgreSQL to be ready..."
    for i in {1..60}; do
        if docker exec telemetry-postgres pg_isready -U postgres &>/dev/null; then
            break
        fi
        sleep 1
    done
    print_success "PostgreSQL is running and healthy"
fi

# Step 4: Initialize database schema
print_status "Initializing database schema..."
if command -v psql &> /dev/null; then
    # Create tables if they don't exist
    PGPASSWORD=postgres psql -h localhost -p 5433 -U postgres -d telemetry -c "
    CREATE TABLE IF NOT EXISTS gpu_telemetry (
        id SERIAL PRIMARY KEY,
        timestamp TIMESTAMP NOT NULL,
        gpu_id VARCHAR(255) NOT NULL,
        hostname VARCHAR(255) NOT NULL,
        gpu_utilization FLOAT,
        memory_utilization FLOAT,
        memory_used_mb FLOAT,
        memory_free_mb FLOAT,
        temperature FLOAT,
        power_draw FLOAT,
        sm_clock_mhz FLOAT,
        memory_clock_mhz FLOAT,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    CREATE INDEX IF NOT EXISTS idx_gpu_telemetry_timestamp ON gpu_telemetry(timestamp);
    CREATE INDEX IF NOT EXISTS idx_gpu_telemetry_gpu_id ON gpu_telemetry(gpu_id);
    CREATE INDEX IF NOT EXISTS idx_gpu_telemetry_hostname ON gpu_telemetry(hostname);
    " &>/dev/null
    print_success "Database schema initialized"
else
    print_warning "psql not found. Database schema will be created automatically by the application."
fi

# Step 5: Verify everything is working
print_status "Verifying setup..."

# Test etcd
if docker exec telemetry-etcd etcdctl endpoint health &>/dev/null; then
    print_success "✓ etcd is healthy"
else
    print_error "✗ etcd is not healthy"
fi

# Test Redis
if docker exec telemetry-redis redis-cli ping | grep -q "PONG"; then
    print_success "✓ Redis is healthy"
else
    print_error "✗ Redis is not healthy"
fi

# Test PostgreSQL
if docker exec telemetry-postgres pg_isready -U postgres &>/dev/null; then
    print_success "✓ PostgreSQL is healthy"
else
    print_error "✗ PostgreSQL is not healthy"
fi

# Test binaries
if [[ -f "bin/nexus-collector" ]]; then
    print_success "✓ Nexus collector built"
else
    print_error "✗ Nexus collector not found"
fi

if [[ -f "bin/nexus-api" ]]; then
    print_success "✓ Nexus API built"
else
    print_error "✗ Nexus API not found"
fi

echo ""
echo "🎉 Nexus-Enhanced Telemetry Pipeline Development Environment Setup Complete!"
echo "=========================================================================="
echo ""
echo "📊 Infrastructure Status:"
echo "  • etcd:       http://localhost:2379"
echo "  • Redis:      redis://localhost:6379"
echo "  • PostgreSQL: postgresql://postgres:postgres@localhost:5433/telemetry"
echo ""
echo "🚀 Quick Start Commands:"
echo "  # Start Nexus collector (in terminal 1)"
echo "  make run-nexus-collector"
echo ""
echo "  # Start Nexus API server (in terminal 2)"
echo "  make run-nexus-api"
echo ""
echo "  # Start traditional streamer (in terminal 3)"
echo "  make run-streamer"
echo ""
echo "📈 Access Points (once running):"
echo "  • Nexus API:    http://localhost:8080"
echo "  • GraphQL:      http://localhost:8080/graphql"
echo "  • WebSocket:    ws://localhost:8080/ws"
echo "  • Health:       http://localhost:8080/health"
echo ""
echo "🔧 Management Commands:"
echo "  make help                    # Show all available commands"
echo "  docker logs telemetry-etcd   # View etcd logs"
echo "  docker logs telemetry-redis  # View Redis logs"
echo "  docker logs telemetry-postgres # View PostgreSQL logs"
echo ""
echo "🧹 Cleanup:"
echo "  docker stop telemetry-etcd telemetry-redis telemetry-postgres"
echo "  docker rm telemetry-etcd telemetry-redis telemetry-postgres"
echo ""
echo "Happy coding! 🎯"
