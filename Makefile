.PHONY: help deps build clean test lint \
	build-nexus build-nexus-streamer build-nexus-collector build-nexus-gateway \
	run-nexus-streamer run-nexus-collector run-nexus-gateway setup-etcd stop-etcd \
	docker-build-nexus k8s-deploy-nexus k8s-undeploy-nexus k8s-status-nexus k8s-logs-nexus k8s-port-forward \
	test test-unit test-integration test-e2e test-coverage test-clean \
	generate-swagger release all

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
NAMESPACE ?= telemetry-system
STREAMER_INSTANCES ?= 2
COLLECTOR_INSTANCES ?= 2
GATEWAY_INSTANCES ?= 2

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
build: build-nexus ## Build all components

build-nexus: build-nexus-collector build-nexus-streamer build-nexus-gateway ## Build all telemetry pipeline components

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
	@mkdir -p ./docs/generated
	$(shell go env GOPATH)/bin/swag init -g cmd/nexus-gateway/main.go -o ./docs/generated --parseInternal --parseDependency
	@echo "OpenAPI specification generated in ./docs/generated/"

# Testing
test: ## Run all tests and collect unified coverage
	@echo "🚀 Running all tests with unified coverage collection..."
	@echo ""
	@echo "Step 1: Cleaning previous coverage data..."
	@rm -f *.out coverage.html
	@rm -f coverage-*.html 2>/dev/null || true
	@mkdir -p coverage-reports
	@echo ""
	@echo "Step 2: Running unit tests with coverage..."
	go test -v -coverprofile=unit-coverage.out -coverpkg=./... ./...
	@echo ""
	@echo "Step 3: Running E2E tests with coverage..."
	@if command -v etcd >/dev/null 2>&1; then \
		go test -v -coverprofile=e2e-coverage.out -coverpkg=./... -timeout=10m ./test/e2e/...; \
	else \
		echo "⚠️  etcd not available, skipping E2E tests"; \
	fi
	@echo ""
	@echo "Step 4: Merging all coverage into single coverage.out..."
	@$(MAKE) _merge-coverage-profiles
	@echo ""
	@echo "Step 5: Generating coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo ""
	@echo "Step 6: Analyzing coverage..."
	@COVERAGE=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	echo "Overall coverage: $$COVERAGE%"; \
	echo "✅ Coverage analysis completed"
	@echo ""
	@echo "Step 7: Cleaning up intermediate files..."
	@rm -f unit-coverage.out integration-coverage.out e2e-coverage.out
	@echo ""
	@echo "✅ All tests completed!"
	@echo "📊 Coverage report: coverage.html"
	@echo "📈 Coverage summary:"
	@go tool cover -func=coverage.out | tail -1
	@echo ""

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	go test -v -race -coverprofile=unit-coverage.out ./...

test-integration: ## Run integration tests only
	@echo "Running integration tests..."
	go test -v -tags=integration -coverprofile=integration-coverage.out -coverpkg=./... ./test/integration/...

test-e2e: ## Run end-to-end tests only
	@echo "Running E2E tests..."
	go test -v -coverprofile=e2e-coverage.out -coverpkg=./... ./test/e2e/...

test-comprehensive: test ## Alias for unified test (legacy compatibility)

test-coverage: test ## Alias for unified test (legacy compatibility)

_merge-coverage-profiles:
	@echo "Merging coverage profiles..."
	@echo "mode: set" > coverage.out
	@for file in unit-coverage.out integration-coverage.out e2e-coverage.out; do \
		if [ -f "$$file" ]; then \
			tail -n +2 "$$file" >> coverage.out; \
		fi; \
	done
	@echo "Coverage profiles merged into coverage.out"

_generate-coverage-reports:
	@echo "Generating HTML coverage reports..."
	@go tool cover -html=coverage.out -o coverage.html
	@if [ -f "unit-coverage.out" ]; then \
		go tool cover -html=unit-coverage.out -o coverage-reports/unit-coverage.html; \
	fi
	@if [ -f "integration-coverage.out" ]; then \
		go tool cover -html=integration-coverage.out -o coverage-reports/integration-coverage.html; \
	fi
	@if [ -f "e2e-coverage.out" ]; then \
		go tool cover -html=e2e-coverage.out -o coverage-reports/e2e-coverage.html; \
	fi

_analyze-coverage-thresholds:
	@echo "Analyzing coverage thresholds..."
	@go tool cover -func=coverage.out > coverage-summary.txt
	@COVERAGE=$$(go tool cover -func=coverage.out | tail -1 | awk '{print $$3}' | sed 's/%//'); \
	echo "Overall coverage: $$COVERAGE%"; \
	if [ "$$(echo "$$COVERAGE >= 80" | bc -l 2>/dev/null || echo "0")" = "1" ]; then \
		echo "✅ Coverage threshold met (≥80%)"; \
	else \
		echo "❌ Coverage below threshold (target: ≥80%, actual: $$COVERAGE%)"; \
		echo "Consider adding more tests to improve coverage"; \
	fi

test-watch: ## Run tests in watch mode (requires entr)
	@echo "Running tests in watch mode (Ctrl+C to stop)..."
	@if command -v entr >/dev/null 2>&1; then \
		find . -name "*.go" | entr -c make test-unit; \
	else \
		echo "❌ 'entr' not found. Install with: brew install entr (macOS) or apt-get install entr (Linux)"; \
	fi

test-specific: ## Run specific test (usage: make test-specific TEST=TestName)
	@if [ -z "$(TEST)" ]; then echo "Usage: make test-specific TEST=TestName"; exit 1; fi
	@echo "Running specific test: $(TEST)"
	go test -v -run $(TEST) ./...

test-package: ## Run tests for specific package (usage: make test-package PKG=./pkg/messagequeue)
	@if [ -z "$(PKG)" ]; then echo "Usage: make test-package PKG=./pkg/messagequeue"; exit 1; fi
	@echo "Running tests for package: $(PKG)"
	go test -v -coverprofile=package-coverage.out $(PKG)
	@go tool cover -html=package-coverage.out -o package-coverage.html
	@echo "Package coverage report: package-coverage.html"

test-race: ## Run tests with race detection
	@echo "Running tests with race detection..."
	go test -race -short ./...

test-clean: ## Clean test artifacts and coverage reports
	@echo "Cleaning test artifacts..."
	@rm -f *.out *.html benchmark-results.txt coverage-summary.txt
	@rm -rf coverage-reports
	@go clean -testcache
	@echo "Test artifacts cleaned"

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
	LOG_LEVEL=info \
	./$(BINARY_DIR)/nexus-collector



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



# Kubernetes deployment targets
k8s-deploy-nexus: ## Deploy to Kubernetes using Helm
	@echo "🚀 Deploying telemetry pipeline to Kubernetes..."
	helm upgrade --install telemetry-pipeline deployments/helm/telemetry-pipeline \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--values deployments/helm/telemetry-pipeline/values-nexus.yaml \
		--set image.tag=$(VERSION) \
		--set global.imageRegistry=$(DOCKER_REGISTRY) \
		--set nexusStreamer.replicaCount=$(STREAMER_INSTANCES) \
		--set nexusCollector.replicaCount=$(COLLECTOR_INSTANCES) \
		--set nexusGateway.replicaCount=$(GATEWAY_INSTANCES) \
		--wait --timeout=10m
	@echo "✅ Deployment successful"

k8s-undeploy-nexus: ## Undeploy from Kubernetes
	@echo "🗑️  Undeploying telemetry pipeline from Kubernetes..."
	helm uninstall telemetry-pipeline --namespace $(NAMESPACE) || true
	@echo "✅ Undeployment complete"

k8s-status-nexus: ## Show Kubernetes deployment status
	@echo "📊 Deployment Status:"
	@echo "Helm Release:"
	helm status telemetry-pipeline --namespace $(NAMESPACE) || echo "Not deployed"
	@echo "Pods:"
	kubectl get pods -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline || echo "No pods found"
	@echo "Services:"
	kubectl get svc -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline || echo "No services found"

k8s-logs-nexus: ## Follow logs from Kubernetes deployment
	@echo "📋 Following logs from components..."
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/name=telemetry-pipeline --all-containers=true -f

k8s-port-forward: ## Port forward to Gateway
	@echo "🔗 Port forwarding to Gateway (8080 -> 8080)..."
	kubectl port-forward -n $(NAMESPACE) svc/telemetry-pipeline-nexus-gateway 8080:8080



# Docker targets
docker-build: docker-build-nexus ## Build Docker images (Nexus components)

docker-build-nexus: ## Build Nexus Docker images
	@echo "Building Nexus Docker images..."
	docker build -f deployments/docker/Dockerfile.nexus-streamer -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-streamer:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.nexus-collector -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-collector:$(VERSION) .
	docker build -f deployments/docker/Dockerfile.nexus-gateway -t $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-gateway:$(VERSION) .
	@echo "✅ Nexus Docker images built successfully"

docker-push: docker-build-nexus ## Push Docker images to registry
	@echo "Pushing Docker images..."
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-streamer:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-collector:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-nexus-gateway:$(VERSION)



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
	rm -f *.out *.html benchmark-results.txt coverage-summary.txt
	rm -rf coverage-reports
	rm -f kubernetes-manifests*.yaml
	go clean -cache
	go clean -testcache
	@echo "✅ Cleanup complete (including etcd and test artifacts)"

# Release
release: clean deps lint test generate-swagger build ## Prepare for release
	@echo "Release preparation complete!"
	@echo "Version: $(VERSION)"
	@echo "Binaries available in $(BINARY_DIR)/"
	@echo "OpenAPI spec available in docs/"

# Performance testing
benchmark: ## Run benchmark tests
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

# Documentation
docs: generate-swagger ## Generate all documentation
	@echo "Generating documentation..."
	@echo "OpenAPI documentation: ./docs/generated/"
	@echo "Coverage report: coverage.html"



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



# All-in-one targets
all: clean deps lint test generate-swagger build ## Build everything
	@echo "Build complete!"

ci: deps lint test generate-swagger build ## CI pipeline
	@echo "CI pipeline complete!"
