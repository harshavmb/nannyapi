#!/bin/bash

echo "Setting up NannyAPI development environment..."

# Check for required tools
command -v go >/dev/null 2>&1 || { echo "Go is required but not installed. Aborting." >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "Docker is required but not installed. Aborting." >&2; exit 1; }
command -v docker-compose >/dev/null 2>&1 || { echo "Docker Compose is required but not installed. Aborting." >&2; exit 1; }

# Create .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file from example..."
    cp .env.example .env
    echo "Please update .env with your actual configuration values"
fi

# Install development tools
echo "Installing development tools..."
go install github.com/swaggo/swag/cmd/swag@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install github.com/sonatype-nexus-community/nancy@latest

# Download dependencies
echo "Downloading Go dependencies..."
go mod download
go mod tidy

# Generate Swagger docs
echo "Generating Swagger documentation..."
swag init -g cmd/main.go

# Run initial code quality checks
echo "Running code quality checks..."
make lint
make test

# Start development environment
echo "Starting development environment..."
docker-compose up -d mongo

echo "Development environment setup complete!"
echo "To start the application, run: make run"
echo "To run tests, run: make test"
echo "To check code quality, run: make pre-commit"
