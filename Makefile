.PHONY: help build test clean run-streamer run-collector run-api-gateway docker-build docker-push helm-install helm-uninstall generate-swagger deps lint

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

docker-push: docker-build ## Push Docker images to registry
	@echo "Pushing Docker images..."
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-streamer:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-collector:$(VERSION)
	docker push $(DOCKER_REGISTRY)/$(PROJECT_NAME)-api-gateway:$(VERSION)

# Kubernetes/Helm targets
helm-install: ## Install using Helm
	@echo "Installing with Helm..."
	helm upgrade --install telemetry-pipeline ./deployments/helm/telemetry-pipeline \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME)

helm-uninstall: ## Uninstall Helm release
	@echo "Uninstalling Helm release..."
	helm uninstall telemetry-pipeline

helm-template: ## Generate Kubernetes manifests from Helm chart
	@echo "Generating Kubernetes manifests..."
	helm template telemetry-pipeline ./deployments/helm/telemetry-pipeline \
		--set image.tag=$(VERSION) \
		--set image.repository=$(DOCKER_REGISTRY)/$(PROJECT_NAME) \
		> kubernetes-manifests.yaml

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

# All-in-one targets
all: clean deps lint test generate-swagger build ## Build everything
	@echo "Build complete!"

ci: deps lint test generate-swagger build ## CI pipeline
	@echo "CI pipeline complete!"
