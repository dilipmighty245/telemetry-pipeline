.PHONY: help build test clean run-streamer run-collector run-api-gateway run-all-optimized run-streamer-prod run-collector-prod run-api-gateway-prod docker-build docker-push helm-install helm-uninstall generate-swagger deps lint \
	helm-install-same-cluster helm-install-cross-cluster-edge helm-install-cross-cluster-central helm-install-cross-cluster-all \
	helm-uninstall-cross-cluster helm-template-edge helm-template-central docker-build-local docker-build-all \
	kind-setup kind-cleanup kind-status kind-load-images \
	deploy-status deployment-info validate-cross-cluster-env \
	validate-same-cluster validate-cross-cluster validate-connectivity validate-health validate-scaling \
	setup-local-env setup-kind-env local-status local-logs local-cleanup kind-logs kind-cleanup

# Default target
help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build configuration
BINARY_DIR := bin
DOCKER_REGISTRY := your-registry.com
PROJECT_NAME := telemetry-pipeline
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.1.0")

# Deployment configuration
EDGE_CONTEXT ?= 
CENTRAL_CONTEXT ?= 
SHARED_CONTEXT ?= 
REDIS_HOST ?= 
REDIS_PASSWORD ?= 
DB_PASSWORD ?= 
NAMESPACE ?= telemetry-system

# Instance configuration for setup commands
STREAMER_INSTANCES ?= 2
COLLECTOR_INSTANCES ?= 2
API_GW_INSTANCES ?= 2
KIND_EDGE_NODES ?= 2
KIND_CENTRAL_NODES ?= 3
DOCKER_COMPOSE_FILE ?= docker-compose.yml

# Go build flags
GO_BUILD_FLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(shell date -u '+%Y-%m-%d_%H:%M:%S')"

# Dependencies
deps: ## Install dependencies
	@echo "Installing dependencies..."
	go mod tidy
	go mod download
	@echo "Installing development tools..."
	go install github.com/swaggo/swag/cmd/swag@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Development tools installed in $(shell go env GOPATH)/bin/"
	@echo "Make sure $(shell go env GOPATH)/bin is in your PATH"

# Build targets
build: build-nexus ## Build all Nexus services

# Nexus build targets
build: build-streamer build-collector ## Build all core components (api-gateway removed, use nexus-gateway)

build-nexus: build-nexus-collector build-nexus-streamer build-nexus-gateway ## Build all three telemetry pipeline components with Nexus integration

build-nexus-collector: ## Build Nexus collector (component 2 of 3)
	@echo "Building Nexus collector (component 2 of 3)..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/nexus-collector ./cmd/nexus-collector
	@echo "✅ Nexus collector built successfully"

# build-nexus-api removed - functionality consolidated into nexus-gateway

build-nexus-streamer: ## Build Nexus streamer (component 1 of 3)
	@echo "Building Nexus streamer (component 1 of 3)..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/nexus-streamer ./cmd/nexus-streamer
	@echo "✅ Nexus streamer built successfully"

build-nexus-gateway: ## Build Nexus gateway (multi-protocol API)
	@echo "Building Nexus API Gateway (component 3 of 3)..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/nexus-gateway ./cmd/nexus-gateway
	@echo "✅ Nexus API Gateway built successfully"

# Generate OpenAPI specification
generate-swagger: ## Generate OpenAPI/Swagger specification
	@echo "Generating OpenAPI specification..."
	$(shell go env GOPATH)/bin/swag init -g cmd/api-gateway/main.go -o ./docs --parseInternal --parseDependency
	@echo "OpenAPI specification generated in ./docs/"

# Testing
test: ## Run unit tests
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests and show coverage report
	@echo "Coverage report:"
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report saved to coverage.html"

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	go test -v -tags=integration ./test/integration/...

# Linting
lint: ## Run linter
	@echo "Running linter..."
	golangci-lint run

# Run services locally (Nexus components only)

# Nexus run targets
run-nexus-collector: build-nexus-collector ## Run Nexus collector locally
	@echo "Starting Nexus collector (etcd-based message queue)..."
	@echo "Ensure etcd is running: make setup-etcd"
	@echo "This consumes messages directly from etcd queue (no Redis needed)"
	CLUSTER_ID=local-cluster \
	ETCD_ENDPOINTS=localhost:2379 \
	MESSAGE_QUEUE_PREFIX=/telemetry/queue \
	DB_HOST=localhost \
	DB_PORT=5433 \
	ENABLE_NEXUS=true \
	ENABLE_WATCH_API=true \
	LOG_LEVEL=info \
	./$(BINARY_DIR)/nexus-collector

# run-nexus-api removed - use run-nexus-gateway instead
	@echo "  • WebSocket: ws://localhost:8080/ws"
	CLUSTER_ID=local-cluster \
	ETCD_ENDPOINTS=localhost:2379 \
	PORT=8080 \
	ENABLE_GRAPHQL=true \
	ENABLE_WEBSOCKET=true \
	ENABLE_CORS=true \
	LOG_LEVEL=info \
	./$(BINARY_DIR)/nexus-api

run-nexus-streamer: build-nexus-streamer ## Run Nexus streamer locally (etcd-based)
	@echo "Starting Nexus streamer (etcd-based)..."
	@echo "Ensure etcd is running: make setup-etcd"
	@echo "This streams CSV data directly to etcd message queue"
	CLUSTER_ID=local-cluster \
	ETCD_ENDPOINTS=localhost:2379 \
	CSV_FILE=dcgm_metrics_20250718_134233.csv \
	BATCH_SIZE=100 \
	STREAM_INTERVAL=3s \
	LOOP_MODE=true \
	LOG_LEVEL=info \
	./$(BINARY_DIR)/nexus-streamer

run-nexus-gateway: build-nexus-gateway ## Run Nexus gateway locally (multi-protocol API)
	@echo "Starting Nexus gateway (multi-protocol API)..."
	@echo "Ensure etcd is running: make setup-etcd"
	@echo "Gateway will be available at:"
	@echo "  • REST API: http://localhost:8080"
	@echo "  • GraphQL: http://localhost:8080/graphql"
	@echo "  • WebSocket: ws://localhost:8080/ws"
	@echo "  • Health: http://localhost:8080/health"
	CLUSTER_ID=local-cluster \
	ETCD_ENDPOINTS=localhost:2379 \
	PORT=8080 \
	ENABLE_GRAPHQL=true \
	ENABLE_WEBSOCKET=true \
	ENABLE_CORS=true \
	LOG_LEVEL=info \
	./$(BINARY_DIR)/nexus-gateway

# Local scaling targets
scale-local: ## Scale all components locally (usage: make scale-local INSTANCES=3)
	@echo "🚀 Scaling pipeline locally with $(INSTANCES) instances of each component"
	./scripts/scale-local.sh all $(INSTANCES)

scale-streamers: ## Scale streamers locally (usage: make scale-streamers INSTANCES=5)
	@echo "📤 Scaling streamers to $(STREAMER_INSTANCES) instances"
	./scripts/scale-local.sh streamer $(STREAMER_INSTANCES)

scale-collectors: ## Scale collectors locally (usage: make scale-collectors INSTANCES=3)
	@echo "📥 Scaling collectors to $(COLLECTOR_INSTANCES) instances"
	./scripts/scale-local.sh collector $(COLLECTOR_INSTANCES)

scale-gateways: ## Scale gateways locally (usage: make scale-gateways INSTANCES=2)
	@echo "🌐 Scaling gateways to $(API_GW_INSTANCES) instances"
	./scripts/scale-local.sh gateway $(API_GW_INSTANCES)

scale-status: ## Show scaling status
	./scripts/scale-local.sh status

scale-stop: ## Stop all scaled components
	./scripts/scale-local.sh stop

scale-logs: ## Follow logs from all scaled components
	./scripts/scale-local.sh logs

# Nexus Kubernetes deployment targets
k8s-deploy-nexus: ## Deploy Nexus components to Kubernetes using Helm
	@echo "🚀 Deploying Nexus telemetry pipeline to Kubernetes..."
	helm upgrade --install telemetry-pipeline deployments/helm/telemetry-pipeline \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values deployments/helm/telemetry-pipeline/values-nexus.yaml \
		--set image.tag=$(VERSION) \
		--set global.imageRegistry=$(DOCKER_REGISTRY) \
		--set global.clusterId=$(NAMESPACE) \
		--set nexusStreamer.replicaCount=$(STREAMER_INSTANCES) \
		--set nexusCollector.replicaCount=$(COLLECTOR_INSTANCES) \
		--set nexusGateway.replicaCount=$(API_GW_INSTANCES) \
		--wait --timeout=10m
	@echo "✅ Nexus telemetry pipeline deployed successfully"

k8s-undeploy-nexus: ## Undeploy Nexus components from Kubernetes
	@echo "🗑️  Undeploying Nexus telemetry pipeline from Kubernetes..."
	helm uninstall telemetry-pipeline --namespace $(NAMESPACE) || true
	@echo "✅ Nexus telemetry pipeline undeployed"

k8s-status-nexus: ## Show Kubernetes deployment status for Nexus components
	@echo "📊 Nexus Telemetry Pipeline Status:"
	@echo ""
	@echo "Helm Release:"
	helm status telemetry-pipeline --namespace $(NAMESPACE) || echo "Not deployed"
	@echo ""
	@echo "Pods:"
	kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline || echo "No pods found"
	@echo ""
	@echo "Services:"
	kubectl get svc -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline || echo "No services found"

k8s-logs-nexus: ## Follow logs from Nexus components in Kubernetes
	@echo "📋 Following logs from Nexus components..."
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline --all-containers=true -f

k8s-port-forward: ## Port forward to Nexus Gateway in Kubernetes
	@echo "🔗 Port forwarding to Nexus Gateway (8080 -> 8080)..."
	kubectl port-forward -n $(NAMESPACE) svc/telemetry-pipeline-nexus-gateway 8080:8080

# Development workflow targets
dev-docker-build: ## Build Docker images for development
	@echo "🔨 Building Docker images for development..."
	DOCKER_REGISTRY=localhost:5000 VERSION=dev make docker-build-nexus

dev-k8s-deploy: ## Deploy to local Kubernetes for development
	@echo "🚀 Deploying to local Kubernetes for development..."
	DOCKER_REGISTRY=localhost:5000 VERSION=dev NAMESPACE=telemetry-dev make k8s-deploy-nexus

run-nexus-pipeline: ## Run complete Nexus pipeline (etcd-only, no Redis)
	@echo "🚀 Starting complete Nexus pipeline (etcd-only)"
	@echo "This will start all components in the background"
	@echo "Ensure etcd is running: make setup-etcd"
	@echo ""
	@echo "Starting Nexus collector..."
	CLUSTER_ID=local-cluster ETCD_ENDPOINTS=localhost:2379 DB_HOST=localhost DB_PORT=5433 ENABLE_NEXUS=true LOG_LEVEL=info ./$(BINARY_DIR)/nexus-collector &
	@sleep 2
	@echo "Starting Nexus gateway..."
	CLUSTER_ID=local-cluster ETCD_ENDPOINTS=localhost:2379 PORT=8080 ENABLE_GRAPHQL=true ENABLE_WEBSOCKET=true LOG_LEVEL=info ./$(BINARY_DIR)/nexus-gateway &
	@sleep 2
	@echo "Starting Nexus streamer..."
	CLUSTER_ID=local-cluster ETCD_ENDPOINTS=localhost:2379 CSV_FILE=dcgm_metrics_20250718_134233.csv BATCH_SIZE=100 LOG_LEVEL=info ./$(BINARY_DIR)/nexus-streamer &
	@echo ""
	@echo "✅ Nexus pipeline started!"
	@echo "📊 REST API: http://localhost:8080/api/v1"
	@echo "📈 GraphQL: http://localhost:8080/graphql"
	@echo "🔌 WebSocket: ws://localhost:8080/ws"
	@echo "🏥 Health: http://localhost:8080/health"
	@echo ""
	@echo "To stop: pkill -f 'nexus-'"

# Docker targets
docker-build: docker-build-nexus ## Build Docker images (Nexus components)

docker-build-nexus: ## Build Nexus Docker images
	@echo "Building Nexus Docker images..."
	docker build -f deployments/docker/Dockerfile.nexus-streamer -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-streamer:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.nexus-collector -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-collector:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.nexus-gateway -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-gateway:$(VERSION) .
	@echo "✅ Nexus Docker images built successfully"

docker-build-legacy: ## Build legacy Docker images (deprecated)
	@echo "Building legacy Docker images..."
	docker build -f deployments/docker/Dockerfile.streamer -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-streamer:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.collector -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-collector:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.api-gateway -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-api-gateway:$(VERSION) .

# Also tag with latest for local development
docker-build-local: ## Build Docker images with 'latest' tag for local development
	@echo "Building Docker images for local development..."
	docker build -f deployments/docker/Dockerfile.streamer -t $(PROJECT_NAME)-streamer:latest .
	docker build -f deployments/docker/Dockerfile.collector -t $(PROJECT_NAME)-collector:latest .
	docker build -f deployments/docker/Dockerfile.api-gateway -t $(PROJECT_NAME)-api-gateway:latest .

docker-build-all: docker-build docker-build-local ## Build all Docker image variants

docker-push: docker-build ## Push Docker images to registry
	@echo "Pushing Docker images..."
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-streamer:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-collector:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-api-gateway:$(VERSION)

# Kubernetes/Helm targets
helm-install: ## Install using Helm (same cluster - all components)
	@echo "Installing with Helm (same cluster)..."
	helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
		--namespace telemetry-system --create-namespace \
		--values ./deployments/helm/telemetry-pipeline/values-same-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		--wait --timeout=10m

helm-install-same-cluster: helm-install ## Alias for same cluster deployment

helm-install-cross-cluster-edge: ## Deploy streamers to edge cluster
	@echo "Deploying streamers to edge cluster..."
	@if [ -z "$(EDGE_CONTEXT)" ]; then echo "Error: EDGE_CONTEXT not set"; exit 1; fi
	@if [ -z "$(REDIS_HOST)" ]; then echo "Error: REDIS_HOST not set"; exit 1; fi
	@if [ -z "$(REDIS_PASSWORD)" ]; then echo "Error: REDIS_PASSWORD not set"; exit 1; fi
	helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
		--namespace telemetry-system --create-namespace \
		--kube-context $(EDGE_CONTEXT) \
		--values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		--set externalRedis.host=$(REDIS_HOST) \
		--set externalRedis.password=$(REDIS_PASSWORD) \
		--wait --timeout=10m

helm-install-cross-cluster-central: ## Deploy collectors and API to central cluster
	@echo "Deploying collectors and API to central cluster..."
	@if [ -z "$(CENTRAL_CONTEXT)" ]; then echo "Error: CENTRAL_CONTEXT not set"; exit 1; fi
	@if [ -z "$(REDIS_HOST)" ]; then echo "Error: REDIS_HOST not set"; exit 1; fi
	@if [ -z "$(REDIS_PASSWORD)" ]; then echo "Error: REDIS_PASSWORD not set"; exit 1; fi
	@if [ -z "$(DB_PASSWORD)" ]; then echo "Error: DB_PASSWORD not set"; exit 1; fi
	helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
		--namespace telemetry-system --create-namespace \
		--kube-context $(CENTRAL_CONTEXT) \
		--values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		--set externalRedis.host=$(REDIS_HOST) \
		--set externalRedis.password=$(REDIS_PASSWORD) \
		--set postgresql.auth.password=$(DB_PASSWORD) \
		--wait --timeout=15m

helm-install-cross-cluster-all: ## Deploy all components across clusters
	@echo "Deploying all components across clusters..."
	@./scripts/deploy-cross-cluster.sh \
		--edge-context $(EDGE_CONTEXT) \
		--central-context $(CENTRAL_CONTEXT) \
		--redis-host $(REDIS_HOST) \
		--redis-password $(REDIS_PASSWORD) \
		--db-password $(DB_PASSWORD) \
		deploy-all

helm-uninstall: ## Uninstall Helm release (same cluster)
	@echo "Uninstalling Helm release..."
	helm uninstall telemetry-pipeline --namespace telemetry-system

helm-uninstall-cross-cluster: ## Uninstall cross-cluster deployments
	@echo "Uninstalling cross-cluster deployments..."
	@if [ -n "$(EDGE_CONTEXT)" ]; then \
		helm uninstall telemetry-edge --namespace telemetry-system --kube-context $(EDGE_CONTEXT) || true; \
	fi
	@if [ -n "$(CENTRAL_CONTEXT)" ]; then \
		helm uninstall telemetry-central --namespace telemetry-system --kube-context $(CENTRAL_CONTEXT) || true; \
	fi

helm-template: ## Generate Kubernetes manifests from Helm chart
	@echo "Generating Kubernetes manifests..."
	helm template telemetry-pipeline ./deployments/helm/telemetry-pipeline \
		--values ./deployments/helm/telemetry-pipeline/values-same-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		> kubernetes-manifests-same-cluster.yaml

helm-template-edge: ## Generate edge cluster manifests
	@echo "Generating edge cluster manifests..."
	helm template telemetry-edge ./deployments/helm/telemetry-pipeline \
		--values ./deployments/helm/telemetry-pipeline/values-edge-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		--set externalRedis.host=$(REDIS_HOST) \
		> kubernetes-manifests-edge.yaml

helm-template-central: ## Generate central cluster manifests
	@echo "Generating central cluster manifests..."
	helm template telemetry-central ./deployments/helm/telemetry-pipeline \
		--values ./deployments/helm/telemetry-pipeline/values-central-cluster.yaml \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		--set externalRedis.host=$(REDIS_HOST) \
		> kubernetes-manifests-central.yaml

# Development helpers
dev-setup: deps ## Setup development environment
	@echo "Setting up development environment..."
	@echo "Installing pre-commit hooks..."
	@echo "Development environment ready!"

dev-run: ## Run all services locally for development
	@echo "Starting all services for development..."
	make -j3 run-streamer run-collector run-api-gateway

# Database helpers
db-setup: ## Setup local PostgreSQL database
	@echo "Setting up local database..."
	@echo "Cleaning up any existing database container..."
	@docker stop telemetry-postgres 2>/dev/null || true
	@docker rm telemetry-postgres 2>/dev/null || true
	@echo "Starting PostgreSQL container..."
	docker run --name telemetry-postgres \
		-e POSTGRES_PASSWORD=postgres \
		-e POSTGRES_USER=postgres \
		-e POSTGRES_DB=telemetry \
		-p 5433:5432 \
		-d postgres:15
	@echo "Waiting for database to start..."
	@for i in {1..30}; do \
		if docker exec telemetry-postgres pg_isready -U postgres -d telemetry >/dev/null 2>&1; then \
			echo "Database is ready!"; \
			break; \
		fi; \
		if [ $$i -eq 30 ]; then \
			echo "Database failed to start after 30 attempts"; \
			exit 1; \
		fi; \
		echo "Waiting for database... ($$i/30)"; \
		sleep 1; \
	done
	@echo "Database ready at localhost:5433"
	@echo "Database: telemetry, User: postgres, Password: postgres, Port: 5433"

db-status: ## Check database status
	@echo "Checking database status..."
	@if docker ps | grep -q telemetry-postgres; then \
		echo "✅ PostgreSQL container is running"; \
		if docker exec telemetry-postgres pg_isready -U postgres -d telemetry >/dev/null 2>&1; then \
			echo "✅ Database 'telemetry' is ready"; \
		else \
			echo "❌ Database is not ready"; \
		fi; \
	else \
		echo "❌ PostgreSQL container is not running"; \
		echo "Run 'make db-setup' to start the database"; \
	fi

db-connect: ## Connect to the database (requires psql)
	@echo "Connecting to database..."
	@if ! command -v psql >/dev/null 2>&1; then \
		echo "❌ psql not found. Install PostgreSQL client or use Docker:"; \
		echo "   docker exec -it telemetry-postgres psql -U postgres -d telemetry"; \
	else \
		psql -h localhost -p 5433 -U postgres -d telemetry; \
	fi

db-logs: ## Show database logs
	@echo "Database logs:"
	@docker logs telemetry-postgres

db-cleanup: ## Stop and remove local database
	@echo "Cleaning up local database..."
	docker stop telemetry-postgres || true
	docker rm telemetry-postgres || true

# Protobuf generation (if needed)
generate-protos: ## Generate Go code from protobuf files
	@echo "Generating protobuf code..."
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		protos/telemetry/*.proto protos/messagequeue/*.proto

# Clean up
clean: ## Clean build artifacts and etcd data
	@echo "Cleaning up..."
	@echo "Flushing etcd data..."
	@if command -v etcdctl >/dev/null 2>&1; then \
		etcdctl --endpoints=localhost:2379 del "" --from-key || echo "No etcd data to flush or etcd not running"; \
	else \
		echo "etcdctl not found, skipping etcd flush"; \
	fi
	@echo "Stopping etcd server..."
	@pkill -f etcd || echo "No etcd process found"
	@rm -rf /tmp/etcd-data || true
	rm -rf $(BINARY_DIR)	
	rm -f coverage.out coverage.html
	rm -f kubernetes-manifests.yaml
	go clean -cache
	go clean -testcache
	@echo "✅ Cleanup complete (including etcd)"

# Release
release: clean deps lint test generate-swagger build ## Prepare for release
	@echo "Release preparation complete!"
	@echo "Version: $(VERSION)"
	@echo "Binaries available in $(BINARY_DIR)/"
	@echo "OpenAPI spec available in docs/"

# Monitoring and observability
logs-streamer: ## Show streamer logs (assumes running in Docker)
	docker logs -f telemetry-streamer

logs-collector: ## Show collector logs (assumes running in Docker)
	docker logs -f telemetry-collector

logs-api: ## Show API gateway logs (assumes running in Docker)
	docker logs -f telemetry-api-gateway

# Performance testing
benchmark: ## Run benchmark tests
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Security scanning
security-scan: ## Run security scan
	@echo "Running security scan..."
	go list -json -m all | nancy sleuth

# Documentation
docs: generate-swagger ## Generate all documentation
	@echo "Generating documentation..."
	@echo "OpenAPI documentation: ./docs/"
	@echo "Coverage report: coverage.html"

# Kind cluster management
kind-setup: ## Setup Kind clusters for cross-cluster testing
	@echo "Setting up Kind clusters for testing..."
	./scripts/setup-kind-clusters.sh create-all

# Removed duplicate targets - see consolidated commands section

kind-load-images: docker-build ## Load Docker images into Kind clusters
	@echo "Loading images into Kind clusters..."
	./scripts/setup-kind-clusters.sh load-images

# Deployment status and information
deploy-status: ## Check deployment status across clusters
	@echo "Checking deployment status..."
	@if [ -n "$(EDGE_CONTEXT)" ] || [ -n "$(CENTRAL_CONTEXT)" ] || [ -n "$(SHARED_CONTEXT)" ]; then \
		./scripts/deploy-cross-cluster.sh \
			$(if $(EDGE_CONTEXT),--edge-context $(EDGE_CONTEXT)) \
			$(if $(CENTRAL_CONTEXT),--central-context $(CENTRAL_CONTEXT)) \
			$(if $(SHARED_CONTEXT),--shared-context $(SHARED_CONTEXT)) \
			status; \
	else \
		echo "Checking same-cluster deployment..."; \
		kubectl get pods,svc,hpa -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline 2>/dev/null || echo "No deployment found in namespace $(NAMESPACE)"; \
	fi

deployment-info: ## Show deployment configuration information
	@echo "=== Deployment Configuration ==="
	@echo "Project: $(PROJECT_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Registry: $(DOCKER_REGISTRY)"
	@echo "Namespace: $(NAMESPACE)"
	@echo ""
	@echo "=== Cross-Cluster Configuration ==="
	@echo "Edge Context: $(EDGE_CONTEXT)"
	@echo "Central Context: $(CENTRAL_CONTEXT)"
	@echo "Shared Context: $(SHARED_CONTEXT)"
	@echo "Redis Host: $(REDIS_HOST)"
	@echo ""
	@echo "=== Available Deployment Modes ==="
	@echo "1. Same Cluster: make helm-install-same-cluster"
	@echo "2. Cross Cluster Edge: make helm-install-cross-cluster-edge EDGE_CONTEXT=<context> REDIS_HOST=<host> REDIS_PASSWORD=<pass>"
	@echo "3. Cross Cluster Central: make helm-install-cross-cluster-central CENTRAL_CONTEXT=<context> REDIS_HOST=<host> REDIS_PASSWORD=<pass> DB_PASSWORD=<pass>"
	@echo "4. Cross Cluster All: make helm-install-cross-cluster-all EDGE_CONTEXT=<edge> CENTRAL_CONTEXT=<central> REDIS_HOST=<host> REDIS_PASSWORD=<pass> DB_PASSWORD=<pass>"

validate-cross-cluster-env: ## Validate cross-cluster environment variables
	@echo "Validating cross-cluster environment..."
	@if [ -z "$(REDIS_HOST)" ]; then echo "Error: REDIS_HOST not set"; exit 1; fi
	@if [ -z "$(REDIS_PASSWORD)" ]; then echo "Error: REDIS_PASSWORD not set"; exit 1; fi
	@echo "Cross-cluster environment validation passed"

validate-same-cluster: ## Validate same cluster deployment
	@echo "Validating same cluster deployment..."
	./scripts/validate-deployment.sh validate-same-cluster

validate-cross-cluster: ## Validate cross-cluster deployment
	@echo "Validating cross-cluster deployment..."
	@if [ -z "$(EDGE_CONTEXT)" ] || [ -z "$(CENTRAL_CONTEXT)" ]; then echo "Error: EDGE_CONTEXT and CENTRAL_CONTEXT required"; exit 1; fi
	./scripts/validate-deployment.sh --edge-context $(EDGE_CONTEXT) --central-context $(CENTRAL_CONTEXT) validate-cross-cluster

validate-connectivity: ## Test connectivity between components
	@echo "Testing component connectivity..."
	./scripts/validate-deployment.sh \
		$(if $(EDGE_CONTEXT),--edge-context $(EDGE_CONTEXT)) \
		$(if $(CENTRAL_CONTEXT),--central-context $(CENTRAL_CONTEXT)) \
		validate-connectivity

validate-health: ## Check health endpoints
	@echo "Checking health endpoints..."
	./scripts/validate-deployment.sh \
		$(if $(CENTRAL_CONTEXT),--central-context $(CENTRAL_CONTEXT)) \
		validate-health

validate-scaling: ## Test auto-scaling functionality
	@echo "Testing scaling functionality..."
	./scripts/validate-deployment.sh \
		$(if $(EDGE_CONTEXT),--edge-context $(EDGE_CONTEXT)) \
		validate-scaling

# CSV Data Management
csv-help: ## Show CSV data management help
	@./scripts/manage-csv-data.sh help

csv-create-configmap: ## Create ConfigMap from CSV file (usage: make csv-create-configmap CSV_FILE=path/to/file.csv)
	@if [ -z "$(CSV_FILE)" ]; then echo "Error: CSV_FILE not set. Usage: make csv-create-configmap CSV_FILE=path/to/file.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh create-configmap "$(CSV_FILE)"

csv-create-pvc: ## Create PVC for CSV storage (usage: make csv-create-pvc [SIZE=1Gi])
	@./scripts/manage-csv-data.sh create-pvc $(SIZE)

csv-upload-pvc: ## Upload CSV to PVC (usage: make csv-upload-pvc CSV_FILE=path/to/file.csv)
	@if [ -z "$(CSV_FILE)" ]; then echo "Error: CSV_FILE not set. Usage: make csv-upload-pvc CSV_FILE=path/to/file.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh upload-to-pvc "$(CSV_FILE)"

csv-list: ## List all CSV data sources in Kubernetes
	@./scripts/manage-csv-data.sh list-csv-sources

csv-validate: ## Validate CSV file format (usage: make csv-validate CSV_FILE=path/to/file.csv)
	@if [ -z "$(CSV_FILE)" ]; then echo "Error: CSV_FILE not set. Usage: make csv-validate CSV_FILE=path/to/file.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh validate-csv "$(CSV_FILE)"

csv-deploy-configmap: ## Complete workflow: create ConfigMap and deploy (usage: make csv-deploy-configmap CSV_FILE=path/to/file.csv)
	@if [ -z "$(CSV_FILE)" ]; then echo "Error: CSV_FILE not set. Usage: make csv-deploy-configmap CSV_FILE=path/to/file.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh deploy-with-configmap "$(CSV_FILE)"

csv-deploy-pvc: ## Complete workflow: create PVC, upload CSV, and deploy (usage: make csv-deploy-pvc CSV_FILE=path/to/file.csv [SIZE=1Gi])
	@if [ -z "$(CSV_FILE)" ]; then echo "Error: CSV_FILE not set. Usage: make csv-deploy-pvc CSV_FILE=path/to/file.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh deploy-with-pvc "$(CSV_FILE)" $(SIZE)

csv-deploy-url: ## Deploy with URL-based CSV (usage: make csv-deploy-url CSV_URL=https://example.com/data.csv)
	@if [ -z "$(CSV_URL)" ]; then echo "Error: CSV_URL not set. Usage: make csv-deploy-url CSV_URL=https://example.com/data.csv"; exit 1; fi
	@./scripts/manage-csv-data.sh deploy-with-url "$(CSV_URL)"

# Nexus Setup Commands
setup-etcd: ## Setup and start etcd for Nexus development
	@echo "🚀 Setting up etcd for Nexus pipeline..."
	@echo "Checking if etcd is installed..."
	@if ! command -v etcd >/dev/null 2>&1; then \
		echo "❌ etcd not found. Installing etcd..."; \
		if [[ "$$(uname)" == "Darwin" ]]; then \
			if command -v brew >/dev/null 2>&1; then \
				brew install etcd; \
			else \
				echo "❌ Please install Homebrew first: https://brew.sh/"; \
				exit 1; \
			fi; \
		elif [[ "$$(uname)" == "Linux" ]]; then \
			echo "Please install etcd manually: https://etcd.io/docs/v3.5/install/"; \
			exit 1; \
		fi; \
	fi
	@echo "✅ etcd found"
	@echo "Starting etcd server..."
	@pkill -f etcd || true
	@sleep 1
	@mkdir -p /tmp/etcd-data
	@etcd --name nexus-etcd \
		--data-dir /tmp/etcd-data \
		--listen-client-urls http://localhost:2379 \
		--advertise-client-urls http://localhost:2379 \
		--listen-peer-urls http://localhost:2380 \
		--initial-advertise-peer-urls http://localhost:2380 \
		--initial-cluster nexus-etcd=http://localhost:2380 \
		--initial-cluster-token nexus-cluster \
		--initial-cluster-state new \
		--log-level info &
	@sleep 3
	@echo "🎉 etcd started successfully!"
	@echo "📊 etcd client URL: http://localhost:2379"
	@echo "🔍 Test connection: etcdctl --endpoints=localhost:2379 endpoint health"

stop-etcd: ## Stop etcd server
	@echo "🛑 Stopping etcd server..."
	@pkill -f etcd || echo "No etcd process found"
	@rm -rf /tmp/etcd-data || true
	@echo "✅ etcd stopped and data cleaned up"

# Consolidated Setup Commands
setup-local-env: ## Setup complete local environment with Docker (usage: make setup-local-env [STREAMER_INSTANCES=2] [COLLECTOR_INSTANCES=2] [API_GW_INSTANCES=2])
	@echo "Setting up local development environment..."
	@echo "Configuration:"
	@echo "  - Streamers: $(STREAMER_INSTANCES)"
	@echo "  - Collectors: $(COLLECTOR_INSTANCES)"
	@echo "  - API Gateways: $(API_GW_INSTANCES)"
	@echo ""
	@echo "Step 1: Building Docker images..."
	@make docker-build-local
	@echo ""
	@echo "Step 2: Creating Docker Compose configuration..."
	@$(MAKE) _create-local-compose-config
	@echo ""
	@echo "Step 3: Starting services with Docker Compose..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) down --remove-orphans || true
	docker-compose -f $(DOCKER_COMPOSE_FILE) up -d --scale streamer=$(STREAMER_INSTANCES) --scale collector=$(COLLECTOR_INSTANCES) --scale api-gateway=$(API_GW_INSTANCES)
	@echo ""
	@echo "Step 4: Waiting for services to be ready..."
	@$(MAKE) _wait-for-local-services
	@echo ""
	@echo "✅ Local environment is ready!"
	@echo "Services:"
	@echo "  - PostgreSQL: localhost:5433 (user: postgres, password: postgres, db: telemetry)"
	@echo "  - Redis: localhost:6379"
	@echo "  - API Gateway: http://localhost:8080"
	@echo "  - Adminer (DB UI): http://localhost:8081"
	@echo "  - Streamers: $(STREAMER_INSTANCES) instances"
	@echo "  - Collectors: $(COLLECTOR_INSTANCES) instances"
	@echo "  - API Gateways: $(API_GW_INSTANCES) instances"
	@echo ""
	@echo "Use 'make local-logs' to see logs, 'make local-status' to check status"

setup-kind-env: ## Setup complete Kind environment with cross-cluster deployment (usage: make setup-kind-env [STREAMER_INSTANCES=2] [COLLECTOR_INSTANCES=2] [API_GW_INSTANCES=2] [KIND_EDGE_NODES=2] [KIND_CENTRAL_NODES=3])
	@echo "Setting up Kind clusters for cross-cluster deployment..."
	@echo "Configuration:"
	@echo "  - Edge cluster nodes: $(KIND_EDGE_NODES)"
	@echo "  - Central cluster nodes: $(KIND_CENTRAL_NODES)"
	@echo "  - Streamers: $(STREAMER_INSTANCES) (1 in edge cluster, rest in central cluster)"
	@echo "  - Collectors: $(COLLECTOR_INSTANCES) (all in central cluster)"
	@echo "  - API Gateways: $(API_GW_INSTANCES) (all in central cluster)"
	@echo ""
	@echo "Step 1: Setting up Kind clusters..."
	@./scripts/setup-kind-clusters.sh --edge-nodes $(KIND_EDGE_NODES) --central-nodes $(KIND_CENTRAL_NODES) create-all
	@echo ""
	@echo "Step 2: Building and loading Docker images..."
	@make docker-build-local
	@./scripts/setup-kind-clusters.sh load-images
	@echo ""
	@echo "Step 3: Creating Helm values for cross-cluster deployment..."
	@$(MAKE) _create-kind-values-files
	@echo ""
	@echo "Step 4: Deploying to central cluster (collectors + API gateway + database)..."
	@helm upgrade --install telemetry-central ./deployments/helm/telemetry-pipeline \
		--namespace telemetry-system --create-namespace \
		--kube-context kind-telemetry-central \
		--values /tmp/kind-central-values.yaml \
		--set image.tag=latest \
		--set image.repository=telemetry-pipeline \
		--set collector.replicaCount=$(COLLECTOR_INSTANCES) \
		--set apiGateway.replicaCount=$(API_GW_INSTANCES) \
		--set externalRedis.host=telemetry-central-redis.telemetry-system.svc.cluster.local \
		--wait --timeout=10m
	@echo ""
	@echo "Step 5: Deploying to edge cluster (1 streamer)..."
	@helm upgrade --install telemetry-edge ./deployments/helm/telemetry-pipeline \
		--namespace telemetry-system --create-namespace \
		--kube-context kind-telemetry-edge \
		--values /tmp/kind-edge-values.yaml \
		--set image.tag=latest \
		--set image.repository=telemetry-pipeline \
		--set streamer.replicaCount=1 \
		--set externalRedis.host=host.docker.internal \
		--set externalRedis.port=6379 \
		--wait --timeout=10m
	@echo ""
	@echo "Step 6: Deploying remaining streamers to central cluster..."
	@if [ $(STREAMER_INSTANCES) -gt 1 ]; then \
		helm upgrade telemetry-central ./deployments/helm/telemetry-pipeline \
			--namespace telemetry-system \
			--kube-context kind-telemetry-central \
			--reuse-values \
			--set streamer.enabled=true \
			--set streamer.replicaCount=$$(($(STREAMER_INSTANCES) - 1)) \
			--wait --timeout=5m; \
	fi
	@echo ""
	@echo "Step 7: Setting up port forwarding..."
	@$(MAKE) _setup-kind-port-forwarding &
	@echo ""
	@echo "✅ Kind cross-cluster environment is ready!"
	@echo "Clusters:"
	@echo "  - Edge cluster: kind-telemetry-edge ($(KIND_EDGE_NODES) nodes, 1 streamer)"
	@echo "  - Central cluster: kind-telemetry-central ($(KIND_CENTRAL_NODES) nodes, $$(($(STREAMER_INSTANCES) - 1)) streamers, $(COLLECTOR_INSTANCES) collectors, $(API_GW_INSTANCES) API gateways)"
	@echo "Services (via port-forward):"
	@echo "  - API Gateway: http://localhost:8080"
	@echo "  - Database: localhost:5432"
	@echo ""
	@echo "Use 'make kind-status' to check status, 'make kind-logs' to see logs"

# Internal helper targets
_create-local-compose-config:
	@echo "Creating Docker Compose configuration..."
	@cp docker-compose.yml docker-compose.yml.bak 2>/dev/null || true
	@cat docker-compose.yml | sed 's/container_name: telemetry-streamer/# container_name: telemetry-streamer/' | sed 's/container_name: telemetry-collector/# container_name: telemetry-collector/' | sed 's/container_name: telemetry-api-gateway/# container_name: telemetry-api-gateway/' > $(DOCKER_COMPOSE_FILE).tmp
	@mv $(DOCKER_COMPOSE_FILE).tmp $(DOCKER_COMPOSE_FILE)

_wait-for-local-services:
	@echo "Waiting for services to be ready..."
	@for i in {1..60}; do \
		if docker-compose -f $(DOCKER_COMPOSE_FILE) ps | grep -q "Up"; then \
			if curl -s http://localhost:8080/health >/dev/null 2>&1; then \
				echo "Services are ready!"; \
				break; \
			fi; \
		fi; \
		if [ $$i -eq 60 ]; then \
			echo "Services failed to start after 5 minutes"; \
			exit 1; \
		fi; \
		echo "Waiting for services... ($$i/60)"; \
		sleep 5; \
	done

_create-kind-values-files:
	@./scripts/create-kind-values.sh $(STREAMER_INSTANCES) $(COLLECTOR_INSTANCES) $(API_GW_INSTANCES)

_setup-kind-port-forwarding:
	@echo "Setting up port forwarding..."
	@kubectl --context=kind-telemetry-central port-forward -n telemetry-system service/telemetry-central-api-gateway 8080:80 >/dev/null 2>&1 &
	@kubectl --context=kind-telemetry-central port-forward -n telemetry-system service/telemetry-central-postgresql 5432:5432 >/dev/null 2>&1 &
	@sleep 2

# Status and monitoring commands
local-status: ## Show status of local Docker environment
	@echo "=== Local Environment Status ==="
	docker-compose -f $(DOCKER_COMPOSE_FILE) ps
	@echo ""
	@echo "=== Service Health ==="
	@curl -s http://localhost:8080/health 2>/dev/null && echo "✅ API Gateway: Healthy" || echo "❌ API Gateway: Unhealthy"
	@docker exec telemetry-postgres pg_isready -U postgres -d telemetry >/dev/null 2>&1 && echo "✅ PostgreSQL: Ready" || echo "❌ PostgreSQL: Not ready"
	@docker exec telemetry-redis redis-cli ping >/dev/null 2>&1 && echo "✅ Redis: Ready" || echo "❌ Redis: Not ready"

local-logs: ## Show logs from local services (usage: make local-logs [SERVICE=all|streamer|collector|api-gateway|postgres|redis])
	@SERVICE=${SERVICE:-all}; \
	if [ "$$SERVICE" = "all" ]; then \
		docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f; \
	else \
		docker-compose -f $(DOCKER_COMPOSE_FILE) logs -f $$SERVICE; \
	fi

local-cleanup: ## Stop and remove local environment
	@echo "Cleaning up local environment..."
	docker-compose -f $(DOCKER_COMPOSE_FILE) down --remove-orphans --volumes
	@if [ -f "docker-compose.yml.bak" ]; then mv docker-compose.yml.bak docker-compose.yml; fi
	@rm -f /tmp/kind-*-values.yaml
	@echo "Local environment cleaned up!"

kind-status: ## Show status of Kind clusters and deployments
	@echo "=== Kind Clusters Status ==="
	@./scripts/setup-kind-clusters.sh status
	@echo ""
	@echo "=== Deployments Status ==="
	@echo "Edge Cluster:"
	@kubectl --context=kind-telemetry-edge get pods,svc -n telemetry-system 2>/dev/null || echo "  No deployments found"
	@echo ""
	@echo "Central Cluster:"
	@kubectl --context=kind-telemetry-central get pods,svc -n telemetry-system 2>/dev/null || echo "  No deployments found"

kind-logs: ## Show logs from Kind deployments (usage: make kind-logs [CLUSTER=edge|central] [COMPONENT=streamer|collector|api-gateway])
	@CLUSTER=${CLUSTER:-central}; \
	COMPONENT=${COMPONENT:-all}; \
	CONTEXT="kind-telemetry-$$CLUSTER"; \
	echo "Showing logs for $$COMPONENT in $$CLUSTER cluster..."; \
	if [ "$$COMPONENT" = "all" ]; then \
		kubectl --context=$$CONTEXT logs -n telemetry-system -l app.kubernetes.io/name=telemetry-pipeline --tail=100 -f; \
	else \
		kubectl --context=$$CONTEXT logs -n telemetry-system -l app.kubernetes.io/component=$$COMPONENT --tail=100 -f; \
	fi

kind-cleanup: ## Clean up Kind clusters and resources
	@echo "Cleaning up Kind environment..."
	@./scripts/setup-kind-clusters.sh delete-all
	@rm -f /tmp/kind-*-values.yaml
	@pkill -f "kubectl.*port-forward" || true
	@echo "Kind environment cleaned up!"

# All-in-one targets
all: clean deps lint test generate-swagger build ## Build everything
	@echo "Build complete!"

ci: deps lint test generate-swagger build ## CI pipeline
	@echo "CI pipeline complete!"
