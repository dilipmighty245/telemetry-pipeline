.PHONY: help build test clean run-streamer run-collector run-api-gateway docker-build docker-push helm-install helm-uninstall generate-swagger deps lint \
	helm-install-same-cluster helm-install-cross-cluster-edge helm-install-cross-cluster-central helm-install-cross-cluster-all \
	helm-uninstall-cross-cluster helm-template-edge helm-template-central docker-build-local docker-build-all \
	kind-setup kind-cleanup kind-status kind-load-images \
	deploy-status deployment-info validate-cross-cluster-env \
	validate-same-cluster validate-cross-cluster validate-connectivity validate-health validate-scaling

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

# Build targets
build: build-streamer build-collector build-api-gateway ## Build all services

build-streamer: ## Build streamer service
	@echo "Building streamer..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/streamer ./cmd/streamer

build-collector: ## Build collector service
	@echo "Building collector..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/collector ./cmd/collector

build-api-gateway: ## Build API gateway service
	@echo "Building API gateway..."
	@mkdir -p $(BINARY_DIR)
	go build $(GO_BUILD_FLAGS) -o $(BINARY_DIR)/api-gateway ./cmd/api-gateway

# Generate OpenAPI specification
generate-swagger: ## Generate OpenAPI/Swagger specification
	@echo "Generating OpenAPI specification..."
	swag init -g cmd/api-gateway/main.go -o ./docs --parseInternal --parseDependency
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

# Run services locally
run-streamer: build-streamer ## Run streamer service locally
	@echo "Starting streamer service..."
	./$(BINARY_DIR)/streamer -csv=dcgm_metrics_20250718_134233.csv -loop=true -log-level=debug

run-collector: build-collector ## Run collector service locally
	@echo "Starting collector service..."
	./$(BINARY_DIR)/collector -log-level=debug

run-api-gateway: build-api-gateway generate-swagger ## Run API gateway service locally
	@echo "Starting API gateway service..."
	./$(BINARY_DIR)/api-gateway -port=8080 -log-level=debug

# Docker targets
docker-build: ## Build Docker images for all services
	@echo "Building Docker images..."
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
	docker run --name telemetry-postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=telemetry -p 5432:5432 -d postgres:15
	@echo "Waiting for database to start..."
	sleep 10
	@echo "Database ready at localhost:5432"

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
clean: ## Clean build artifacts
	@echo "Cleaning up..."
	rm -rf $(BINARY_DIR)
	rm -rf docs/
	rm -f coverage.out coverage.html
	rm -f kubernetes-manifests.yaml
	go clean -cache
	go clean -testcache

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

kind-cleanup: ## Clean up Kind clusters
	@echo "Cleaning up Kind clusters..."
	./scripts/setup-kind-clusters.sh delete-all

kind-status: ## Show Kind clusters status
	@echo "Checking Kind clusters status..."
	./scripts/setup-kind-clusters.sh status

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

# All-in-one targets
all: clean deps lint test generate-swagger build ## Build everything
	@echo "Build complete!"

ci: deps lint test generate-swagger build ## CI pipeline
	@echo "CI pipeline complete!"
