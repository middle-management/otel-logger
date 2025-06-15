# OpenTelemetry Logger Makefile
# This Makefile provides convenient commands for building, testing, and managing the otel-logger project
# Uses go-arg for clean command-line argument parsing

# Variables
BINARY_NAME=otel-logger
GO_FILES=$(shell find . -name "*.go" -type f)
VERSION?=dev
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}"

# Default platform
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Docker Compose command (try docker compose first, fallback to docker-compose)
DOCKER_COMPOSE_CMD=$(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
BLUE=\033[0;34m
CYAN=\033[0;36m
NC=\033[0m # No Color

.PHONY: help build test clean install run dev lint fmt vet deps check \
        docker-build docker-run docker-clean \
        env-start env-stop env-logs env-restart env-status \
        demo examples release cross-compile \
        bench profile coverage

# Default target
all: clean deps fmt vet test build

# Help target
help: ## Show this help message
	@echo "OpenTelemetry Logger - Available Commands:"
	@echo ""
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  $(CYAN)%-20s$(NC) %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Environment Variables:"
	@echo "  $(YELLOW)VERSION$(NC)    - Version string (default: dev)"
	@echo "  $(YELLOW)GOOS$(NC)       - Target OS for cross-compilation"
	@echo "  $(YELLOW)GOARCH$(NC)     - Target architecture for cross-compilation"

# Build targets
build: ## Build the binary
	@echo "$(BLUE)[BUILD]$(NC) Building $(BINARY_NAME)..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) .
	@echo "$(GREEN)[SUCCESS]$(NC) Binary built: $(BINARY_NAME)"

build-race: ## Build with race detector
	@echo "$(BLUE)[BUILD]$(NC) Building $(BINARY_NAME) with race detector..."
	@go build $(LDFLAGS) -race -o $(BINARY_NAME) .
	@echo "$(GREEN)[SUCCESS]$(NC) Binary built with race detector: $(BINARY_NAME)"

cross-compile: ## Cross-compile for multiple platforms
	@echo "$(BLUE)[BUILD]$(NC) Cross-compiling for multiple platforms..."
	@mkdir -p dist
	@for os in linux darwin windows; do \
		for arch in amd64 arm64; do \
			if [ "$$os" = "windows" ]; then \
				ext=".exe"; \
			else \
				ext=""; \
			fi; \
			echo "Building for $$os/$$arch..."; \
			GOOS=$$os GOARCH=$$arch go build $(LDFLAGS) -o dist/$(BINARY_NAME)-$$os-$$arch$$ext .; \
		done; \
	done
	@echo "$(GREEN)[SUCCESS]$(NC) Cross-compilation completed in dist/"

# Test targets
test: ## Run all tests
	@echo "$(BLUE)[TEST]$(NC) Running Go tests..."
	@go test -v ./...
	@echo "$(GREEN)[SUCCESS]$(NC) All tests passed"

test-race: ## Run tests with race detector
	@echo "$(BLUE)[TEST]$(NC) Running tests with race detector..."
	@go test -race -v ./...
	@echo "$(GREEN)[SUCCESS]$(NC) Race tests passed"

test-short: ## Run short tests only
	@echo "$(BLUE)[TEST]$(NC) Running short tests..."
	@go test -short -v ./...

test-integration: build ## Run integration tests
	@echo "$(BLUE)[TEST]$(NC) Running integration tests..."
	@./test-tool.sh
	@echo "$(GREEN)[SUCCESS]$(NC) Integration tests completed"

bench: ## Run benchmarks
	@echo "$(BLUE)[BENCH]$(NC) Running benchmarks..."
	@go test -bench=. -benchmem ./...

coverage: ## Generate test coverage report
	@echo "$(BLUE)[COVERAGE]$(NC) Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)[SUCCESS]$(NC) Coverage report generated: coverage.html"

# Code quality targets
fmt: ## Format Go code
	@echo "$(BLUE)[FORMAT]$(NC) Formatting Go code..."
	@go fmt ./...
	@echo "$(GREEN)[SUCCESS]$(NC) Code formatted"

vet: ## Run go vet
	@echo "$(BLUE)[VET]$(NC) Running go vet..."
	@go vet ./...
	@echo "$(GREEN)[SUCCESS]$(NC) Go vet passed"

lint: ## Run golangci-lint (requires golangci-lint to be installed)
	@echo "$(BLUE)[LINT]$(NC) Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
		echo "$(GREEN)[SUCCESS]$(NC) Linting completed"; \
	else \
		echo "$(YELLOW)[WARNING]$(NC) golangci-lint not installed. Install with:"; \
		echo "  curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin v1.54.2"; \
	fi

check: fmt vet lint ## Run all code quality checks

# Dependency management
deps: ## Download and tidy dependencies
	@echo "$(BLUE)[DEPS]$(NC) Downloading dependencies..."
	@go mod download
	@go mod tidy
	@echo "$(GREEN)[SUCCESS]$(NC) Dependencies updated"

deps-upgrade: ## Upgrade all dependencies
	@echo "$(BLUE)[DEPS]$(NC) Upgrading dependencies..."
	@go get -u ./...
	@go mod tidy
	@echo "$(GREEN)[SUCCESS]$(NC) Dependencies upgraded"

# Run targets
run: build ## Build and run the application
	@echo "$(BLUE)[RUN]$(NC) Starting $(BINARY_NAME)..."
	@./$(BINARY_NAME) --help

dev: build ## Run in development mode with example logs
	@echo "$(BLUE)[DEV]$(NC) Running development demo..."
	@if [ -f "examples/json-logs.txt" ]; then \
		echo "Sending example logs to http://localhost:4318..."; \
		cat examples/json-logs.txt | ./$(BINARY_NAME) --endpoint http://localhost:4318 --protocol http --service-name dev-demo; \
	else \
		echo '{"timestamp": "'$$(date -Iseconds)'", "level": "info", "message": "Development test message"}' | \
		./$(BINARY_NAME) --endpoint localhost:4317 --service-name dev-demo; \
	fi

# Docker targets
docker-build: ## Build Docker image
	@echo "$(BLUE)[DOCKER]$(NC) Building Docker image..."
	@docker build -t $(BINARY_NAME):$(VERSION) .
	@echo "$(GREEN)[SUCCESS]$(NC) Docker image built: $(BINARY_NAME):$(VERSION)"

docker-run: docker-build ## Run in Docker container
	@echo "$(BLUE)[DOCKER]$(NC) Running in Docker container..."
	@docker run -it --rm $(BINARY_NAME):$(VERSION) --help

docker-clean: ## Clean Docker images
	@echo "$(BLUE)[DOCKER]$(NC) Cleaning Docker images..."
	@docker rmi $(BINARY_NAME):$(VERSION) 2>/dev/null || true
	@docker system prune -f

# Environment management (using Docker Compose)
env-start: ## Start the development environment
	@echo "$(BLUE)[ENV]$(NC) Starting development environment..."
	@$(DOCKER_COMPOSE_CMD) up -d
	@echo "$(GREEN)[SUCCESS]$(NC) Development environment started"
	@echo "Waiting for services to be ready..."
	@sleep 10
	@make env-status

env-stop: ## Stop the development environment
	@echo "$(BLUE)[ENV]$(NC) Stopping development environment..."
	@$(DOCKER_COMPOSE_CMD) down
	@echo "$(GREEN)[SUCCESS]$(NC) Development environment stopped"

env-restart: env-stop env-start ## Restart the development environment

env-logs: ## Show logs from the development environment
	@echo "$(BLUE)[ENV]$(NC) Showing environment logs..."
	@$(DOCKER_COMPOSE_CMD) logs -f otel-collector

env-status: ## Show status of development environment
	@echo "$(BLUE)[ENV]$(NC) Development environment status:"
	@echo ""
	@echo "Services:"
	@$(DOCKER_COMPOSE_CMD) ps
	@echo ""
	@echo "Endpoints:"
	@echo "  - OpenTelemetry Collector (gRPC): localhost:4317"
	@echo "  - OpenTelemetry Collector (HTTP): http://localhost:4318"
	@echo "  - Jaeger UI: http://localhost:16686"
	@echo "  - Prometheus: http://localhost:9090"
	@echo "  - Grafana: http://localhost:3000 (admin/admin)"
	@echo "  - Elasticsearch: http://localhost:9200"
	@echo "  - Kibana: http://localhost:5601"

# Demo and examples
demo: build env-start ## Run full demonstration
	@echo "$(BLUE)[DEMO]$(NC) Running full demonstration..."
	@sleep 5
	@./quickstart.sh demo
	@echo "$(GREEN)[SUCCESS]$(NC) Demonstration completed"

examples: build ## Generate example log files
	@echo "$(BLUE)[EXAMPLES]$(NC) Generating example log files..."
	@mkdir -p examples
	@if [ ! -f "examples/json-logs.txt" ] || [ ! -f "examples/prefixed-logs.txt" ] || [ ! -f "examples/mixed-logs.txt" ]; then \
		echo "$(YELLOW)[INFO]$(NC) Example files already exist or were created during setup"; \
	fi
	@echo "$(GREEN)[SUCCESS]$(NC) Example files ready in examples/"

# Performance and profiling
profile: build ## Run with CPU profiling
	@echo "$(BLUE)[PROFILE]$(NC) Running with CPU profiling..."
	@mkdir -p profiles
	@echo '{"timestamp": "'$$(date -Iseconds)'", "level": "info", "message": "Profile test"}' | \
		./$(BINARY_NAME) -cpuprofile=profiles/cpu.prof --endpoint localhost:4317 || true
	@if [ -f "profiles/cpu.prof" ]; then \
		echo "$(GREEN)[SUCCESS]$(NC) CPU profile saved to profiles/cpu.prof"; \
		echo "View with: go tool pprof profiles/cpu.prof"; \
	fi

# Release management
release: clean deps check test cross-compile ## Build release artifacts
	@echo "$(BLUE)[RELEASE]$(NC) Creating release artifacts..."
	@mkdir -p release
	@cp dist/* release/ 2>/dev/null || true
	@cp README.md release/
	@cp examples/*.txt release/ 2>/dev/null || true
	@echo "$(GREEN)[SUCCESS]$(NC) Release artifacts created in release/"

# Installation
install: build ## Install binary to GOPATH/bin
	@echo "$(BLUE)[INSTALL]$(NC) Installing $(BINARY_NAME)..."
	@go install $(LDFLAGS) .
	@echo "$(GREEN)[SUCCESS]$(NC) $(BINARY_NAME) installed to $$(go env GOPATH)/bin"

install-tools: ## Install development tools
	@echo "$(BLUE)[TOOLS]$(NC) Installing development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/kisielk/errcheck@latest
	@echo "$(GREEN)[SUCCESS]$(NC) Development tools installed"

# Cleanup
clean: ## Clean build artifacts
	@echo "$(BLUE)[CLEAN]$(NC) Cleaning build artifacts..."
	@rm -f $(BINARY_NAME)
	@rm -rf dist/
	@rm -rf release/
	@rm -rf profiles/
	@rm -f coverage.out coverage.html
	@rm -rf test-logs/
	@echo "$(GREEN)[SUCCESS]$(NC) Cleanup completed"

clean-all: clean docker-clean ## Clean everything including Docker
	@echo "$(BLUE)[CLEAN]$(NC) Full cleanup completed"

# Version info
version: ## Show version information
	@echo "Binary: $(BINARY_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $$(go version)"
	@echo "Platform: $(GOOS)/$(GOARCH)"
