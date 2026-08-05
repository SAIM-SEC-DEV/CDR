.PHONY: build run test clean docker-build docker-run help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOCLEAN=$(GOCMD) clean
GOMOD=$(GOCMD) mod

# Binary name
BINARY_NAME=forge

# Main file
MAIN_FILE=main.go

# Build flags
LDFLAGS=-ldflags "-s -w"

# Default target
all: build

# Build the application
build:
	@echo "Building $(BINARY_NAME)..."
	CGO_ENABLED=0 $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_FILE)
	@echo "Build complete!"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GORUN) $(MAIN_FILE)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	@echo "Coverage report:"
	$(GOCMD) tool cover -func=coverage.out

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f coverage.out
	@echo "Clean complete!"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded!"

# Tidy go.mod
tidy:
	@echo "Tidying go.mod..."
	$(GOMOD) tidy
	@echo "go.mod tidied!"

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t forge-api:latest .
	@echo "Docker image built!"

# Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker-compose up forge-api
	@echo "Docker container stopped!"

# Run all services with Docker Compose
docker-up:
	@echo "Starting all services..."
	docker-compose up -d
	@echo "Services started!"

# Stop all services
docker-down:
	@echo "Stopping all services..."
	docker-compose down
	@echo "Services stopped!"

# View logs
docker-logs:
	docker-compose logs -f

# Restart services
docker-restart:
	docker-compose restart

# Help message
help:
	@echo "FORGE API - Makefile Commands"
	@echo ""
	@echo "Usage:"
	@echo "  make build          - Build the application"
	@echo "  make run            - Run the application locally"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Download dependencies"
	@echo "  make tidy           - Tidy go.mod"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-run     - Run Docker container (forge-api only)"
	@echo "  make docker-up      - Start all Docker Compose services"
	@echo "  make docker-down    - Stop all Docker Compose services"
	@echo "  make docker-logs    - View Docker Compose logs"
	@echo "  make docker-restart - Restart Docker Compose services"
	@echo "  make help           - Show this help message"
	@echo ""
