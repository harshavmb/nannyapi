.PHONY: all build test coverage lint fmt swag clean

# Build the application
all: lint test build

# Build the binary
build:
	go build -o bin/nannyapi ./cmd/main.go

# Run all tests
test:
	go test -v -race ./...

# Run tests with coverage
coverage:
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	go tool cover -html=coverage.txt -o coverage.html

# Run linters
lint:
	go vet ./...
	test -z "$$(gofmt -l .)" || (echo "Please run 'make fmt' to format your code." && exit 1)
	which staticcheck > /dev/null || go install honnef.co/go/tools/cmd/staticcheck@latest
	staticcheck ./...

# Format code
fmt:
	gofmt -s -w .

# Generate Swagger documentation
swag:
	which swag > /dev/null || go install github.com/swaggo/swag/cmd/swag@latest
	swag init -g cmd/main.go

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.txt coverage.html

# Run the application
run:
	go run ./cmd/main.go

# Install dependencies
deps:
	go mod download
	go mod tidy

# Check for outdated dependencies
deps-check:
	go list -u -m -json all | go-mod-outdated -update -direct

# Security check
sec-check:
	which gosec > /dev/null || go install github.com/securego/gosec/v2/cmd/gosec@latest
	gosec ./...

# Run all pre-commit checks
pre-commit: fmt lint test swag

.DEFAULT_GOAL := all
