.PHONY: all build build-static test coverage lint fmt clean run deps deps-check sec-check reset-start build-all

# Build the application
all: lint test build

# Build the binary (development - CGO disabled, uses pure Go SQLite)
build:
	CGO_ENABLED=0 go build -o bin/nannyapi ./main.go

# Build static binary (production - no CGO, cross-platform compatible)
build-static:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/nannyapi ./main.go

# Build for all platforms (Linux only)
build-all:
	@echo "Building for Linux amd64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$$(cat VERSION)" -o bin/nannyapi-linux-amd64 ./main.go
	@echo "Building for Linux arm64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w -X main.Version=$$(cat VERSION)" -o bin/nannyapi-linux-arm64 ./main.go
	@echo "All builds complete!"
	@ls -la bin/

# Run all tests
#
# We pass -tags no_ui to avoid a PocketBase v0.37.x regression where
# apis.NewRouter() re-binds the /_/extensions.js route each time it is called
# (once per ApiScenario), which panics on duplicate registration. The "no_ui"
# build tag excludes the superuser UI assets and skips that binding entirely.
test:
	go test -tags no_ui -v -race ./...

# Run tests with coverage
coverage:
	go test -tags no_ui -v -race -coverprofile=coverage.txt -covermode=atomic ./...
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

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.txt coverage.html
	rm -rf pb_data/

# Run the application (simple run)
run:
	go run ./main.go serve --http="0.0.0.0:8090"

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

# Reset database and start server (Development only)
# WARNING: This deletes pb_data!
reset-start: build
	@echo "Running reset-and-start.sh script..."
	@./scripts/reset-and-start.sh
# Docker targets
DOCKER_IMAGE := docker.io/nannyagent/nannyapi
DOCKER_TAG := $(shell cat VERSION 2>/dev/null || echo "latest")

# Build Docker image locally
docker-build:
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) -t $(DOCKER_IMAGE):latest .

# Run Docker container locally
docker-run: docker-build
	@mkdir -p pb_data
	docker run -d --name nannyapi \
		-p 8090:8090 \
		-v $(PWD)/pb_data:/app/pb_data \
		-e PB_AUTOMIGRATE=true \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# Stop and remove Docker container
docker-stop:
	docker stop nannyapi || true
	docker rm nannyapi || true

# View Docker container logs
docker-logs:
	docker logs -f nannyapi

# Push Docker image (requires docker login)
docker-push: docker-build
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest

# Docker compose up
compose-up:
	docker compose up -d

# Docker compose down
compose-down:
	docker compose down

# Docker compose logs
compose-logs:
	docker compose logs -f
