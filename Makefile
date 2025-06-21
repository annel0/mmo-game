.PHONY: all build run test lint clean help docker proto

# Variables
BINARY_NAME=mmo-server
BINARY_PATH=bin/$(BINARY_NAME)
MAIN_PATH=./cmd/server
DOCKER_IMAGE=mmo-server
DOCKER_TAG=latest

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Build flags
LDFLAGS=-ldflags "-s -w"
BUILD_FLAGS=-v $(LDFLAGS)

# Default target
all: lint test build

# Build the binary
build:
	@echo "🔨 Building binary..."
	@mkdir -p bin
	$(GOBUILD) $(BUILD_FLAGS) -o $(BINARY_PATH) $(MAIN_PATH)
	@echo "✅ Build complete: $(BINARY_PATH)"

# Run the application
run: build
	@echo "🚀 Starting server..."
	$(BINARY_PATH)

# Run with hot reload (requires air)
dev:
	@echo "🔄 Starting with hot reload..."
	@which air > /dev/null || go install github.com/air-verse/air@latest
	air

# Run tests
test:
	@echo "🧪 Running tests..."
	$(GOTEST) -v -race -cover ./...

# Run tests with coverage
test-coverage:
	@echo "📊 Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "✅ Coverage report: coverage.html"

# Run benchmarks
bench:
	@echo "⚡ Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Lint the code
lint:
	@echo "🔍 Running linters..."
	@which golangci-lint > /dev/null || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin
	$(GOLINT) run --timeout=5m ./...

# Format code
fmt:
	@echo "🎨 Formatting code..."
	$(GOFMT) -s -w .
	$(GOCMD) fmt ./...

# Clean up
clean:
	@echo "🧹 Cleaning up..."
	$(GOCLEAN)
	rm -rf bin/ coverage.out coverage.html

# Update dependencies
deps:
	@echo "📦 Updating dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	$(GOMOD) verify

# Security check
security:
	@echo "🔒 Running security checks..."
	@which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec -fmt=json -out=security-report.json ./...
	@which govulncheck > /dev/null || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

# Generate protobuf files
proto:
	@echo "📝 Generating protobuf files..."
	./scripts/generate_proto.sh

# Docker build
docker-build:
	@echo "🐳 Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

# Docker run
docker-run:
	@echo "🐳 Running Docker container..."
	docker run -p 8080:8080 -p 8088:8088 -p 9090:9090 $(DOCKER_IMAGE):$(DOCKER_TAG)

# Database setup
db-setup:
	@echo "🗄️ Setting up database..."
	mysql -u root -p < scripts/setup_mariadb.sql

# Database migration (requires migrate tool)
db-migrate:
	@echo "🗄️ Running database migrations..."
	@which migrate > /dev/null || go install -tags 'mysql' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	migrate -path migrations -database "mysql://root:password@tcp(localhost:3306)/mmo_game" up

# Start all dependencies (requires docker-compose)
deps-up:
	@echo "🚀 Starting dependencies..."
	docker-compose up -d mariadb redis

# Stop all dependencies
deps-down:
	@echo "🛑 Stopping dependencies..."
	docker-compose down

# Check code quality
check: lint test security
	@echo "✅ All checks passed!"

# Install development tools
tools:
	@echo "🛠️ Installing development tools..."
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	@echo "✅ Tools installed!"

# Help
help:
	@echo "MMO Game Server - Available targets:"
	@echo "  make build          - Build the binary"
	@echo "  make run            - Build and run the server"
	@echo "  make dev            - Run with hot reload"
	@echo "  make test           - Run tests with race detector"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make bench          - Run benchmarks"
	@echo "  make lint           - Run linters"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Update dependencies"
	@echo "  make security       - Run security checks"
	@echo "  make proto          - Generate protobuf files"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run Docker container"
	@echo "  make db-setup       - Setup database"
	@echo "  make db-migrate     - Run database migrations"
	@echo "  make deps-up        - Start dependencies (MariaDB, Redis)"
	@echo "  make deps-down      - Stop dependencies"
	@echo "  make check          - Run all checks (lint, test, security)"
	@echo "  make tools          - Install development tools"
	@echo "  make help           - Show this help"

# Default help
.DEFAULT_GOAL := help 