# Consolidated Setup Commands

This document describes the new consolidated setup commands added to the Makefile for easier environment setup.

## Overview

Two new primary commands have been added to simplify environment setup:

1. **`make setup-local-env`** - Sets up a complete local development environment using Docker
2. **`make setup-kind-env`** - Sets up a cross-cluster Kubernetes environment using Kind

## Local Environment Setup

### Command: `make setup-local-env`

Sets up a complete local development environment with PostgreSQL, Redis, and configurable instances of all services.

#### Usage
```bash
# Default configuration (2 instances each)
make setup-local-env

# Custom instance counts
make setup-local-env STREAMER_INSTANCES=3 COLLECTOR_INSTANCES=4 API_GW_INSTANCES=2
```

#### What it does:
1. **Builds Docker images** for all services with `latest` tags
2. **Modifies Docker Compose configuration** to support scaling
3. **Starts all services** with specified instance counts:
   - PostgreSQL database (1 instance)
   - Redis message queue (1 instance) 
   - Telemetry streamers (configurable)
   - Telemetry collectors (configurable)
   - API gateways (configurable)
   - Adminer database UI (1 instance)
4. **Waits for services** to be healthy and ready
5. **Provides access information** for all services

#### Services Access:
- **PostgreSQL**: `localhost:5433` (user: postgres, password: postgres, db: telemetry)
- **Redis**: `localhost:6379`
- **API Gateway**: `http://localhost:8080`
- **Adminer (DB UI)**: `http://localhost:8081`

#### Configuration Parameters:
- `STREAMER_INSTANCES` - Number of streamer instances (default: 2)
- `COLLECTOR_INSTANCES` - Number of collector instances (default: 2)
- `API_GW_INSTANCES` - Number of API gateway instances (default: 2)

## Kind Environment Setup

### Command: `make setup-kind-env`

Sets up a cross-cluster Kubernetes environment with two Kind clusters for testing distributed deployments.

#### Usage
```bash
# Default configuration
make setup-kind-env

# Custom configuration
make setup-kind-env STREAMER_INSTANCES=4 COLLECTOR_INSTANCES=6 API_GW_INSTANCES=3 KIND_EDGE_NODES=3 KIND_CENTRAL_NODES=4
```

#### What it does:
1. **Creates two Kind clusters**:
   - **Edge cluster**: Runs 1 streamer instance (closest to data sources)
   - **Central cluster**: Runs remaining streamers, all collectors, API gateways, database, and Redis
2. **Builds and loads Docker images** into both clusters
3. **Deploys services** using Helm with cross-cluster configuration
4. **Sets up networking** between clusters via shared Redis
5. **Configures port forwarding** for local access

#### Cluster Architecture:
```
┌─────────────────┐    Redis     ┌──────────────────────┐
│   Edge Cluster  │◄────────────►│   Central Cluster    │
│                 │   Message    │                      │
│ • 1 Streamer    │   Queue      │ • N-1 Streamers     │
│                 │              │ • N Collectors       │
│                 │              │ • N API Gateways     │
│                 │              │ • PostgreSQL         │
│                 │              │ • Redis              │
└─────────────────┘              └──────────────────────┘
```

#### Configuration Parameters:
- `STREAMER_INSTANCES` - Total streamers (1 in edge, rest in central) (default: 2)
- `COLLECTOR_INSTANCES` - Collectors in central cluster (default: 2)
- `API_GW_INSTANCES` - API gateways in central cluster (default: 2)
- `KIND_EDGE_NODES` - Nodes in edge cluster (default: 2)
- `KIND_CENTRAL_NODES` - Nodes in central cluster (default: 3)

#### Services Access (via port-forward):
- **API Gateway**: `http://localhost:8080`
- **PostgreSQL**: `localhost:5432`

## Management Commands

### Local Environment
- `make local-status` - Show status of all local services
- `make local-logs [SERVICE=all|streamer|collector|api-gateway|postgres|redis]` - Show service logs
- `make local-cleanup` - Stop and remove all local services

### Kind Environment  
- `make kind-status` - Show status of Kind clusters and deployments
- `make kind-logs [CLUSTER=edge|central] [COMPONENT=streamer|collector|api-gateway]` - Show logs
- `make kind-cleanup` - Clean up Kind clusters and resources

## Examples

### Local Development
```bash
# Start local environment with 3 streamers, 2 collectors, 1 API gateway
make setup-local-env STREAMER_INSTANCES=3 COLLECTOR_INSTANCES=2 API_GW_INSTANCES=1

# Check status
make local-status

# View all logs
make local-logs

# View only streamer logs  
make local-logs SERVICE=streamer

# Clean up when done
make local-cleanup
```

### Cross-Cluster Testing
```bash
# Setup Kind environment with custom scaling
make setup-kind-env STREAMER_INSTANCES=5 COLLECTOR_INSTANCES=8 API_GW_INSTANCES=3

# Check cluster status
make kind-status

# View logs from central cluster collectors
make kind-logs CLUSTER=central COMPONENT=collector

# View logs from edge cluster
make kind-logs CLUSTER=edge

# Clean up when done
make kind-cleanup
```

## Benefits

1. **Single Command Setup**: No need to remember multiple commands or steps
2. **Configurable Scaling**: Easy to test different instance counts
3. **Cross-Cluster Testing**: Realistic distributed deployment simulation  
4. **Automated Networking**: Handles inter-cluster communication setup
5. **Health Checking**: Waits for services to be ready before completion
6. **Easy Cleanup**: Simple commands to tear down environments
7. **Comprehensive Logging**: Easy access to logs from all services

## Troubleshooting

### Local Environment Issues
- **Services not starting**: Check Docker daemon is running
- **Port conflicts**: Ensure ports 5433, 6379, 8080, 8081 are available
- **Resource issues**: Increase Docker resource limits

### Kind Environment Issues  
- **Cluster creation fails**: Ensure Kind and kubectl are installed
- **Image loading fails**: Check Docker images were built successfully
- **Port forwarding fails**: Check no other processes using ports 8080, 5432
- **Cross-cluster communication**: Verify Redis connectivity between clusters

### Getting Help
- Run `make help` to see all available commands
- Check individual service logs using the logging commands
- Use status commands to verify service health
