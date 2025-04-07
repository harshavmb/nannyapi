# Quick Start Guide

This guide will help you get started with NannyAPI development quickly.

## Prerequisites

1. **Go 1.24+**
   - Required for building and running the application
   - [Install Go](https://golang.org/doc/install)

2. **MongoDB**
   - Required for data storage
   - [Install MongoDB](https://docs.mongodb.com/manual/installation/)

3. **Make**
   - Required for running development scripts
   - Usually pre-installed on Linux/Mac

4. **Git**
   - Required for version control
   - [Install Git](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git)

## Initial Setup

1. Clone the repository:
   ```bash
   git clone https://github.com/harshavmb/nannyapi.git
   cd nannyapi
   ```

2. Set up environment variables:
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

3. Install dependencies:
   ```bash
   make deps
   ```

4. Set up development database:
   ```bash
   make setup-dev
   ```

## Configuration

Required environment variables:

```bash
# MongoDB
MONGODB_URI=mongodb://localhost:27017/nannyapi

# Security
NANNY_ENCRYPTION_KEY=your-32-byte-encryption-key
JWT_SECRET=your-jwt-secret

# GitHub OAuth
GH_CLIENT_ID=your-github-client-id
GH_CLIENT_SECRET=your-github-client-secret
GH_REDIRECT_URL=http://localhost:8080/github/callback

# AI Services
DEEPSEEK_API_KEY=your-deepseek-api-key
```

## Development Workflow

1. **Before starting work:**
   ```bash
   git pull
   make deps
   ```

2. **During development:**
   - Format code: `make fmt`
   - Run tests: `make test`
   - Check coverage: `make coverage`
   - Update API docs: `make swag`

3. **Before committing:**
   ```bash
   make pre-commit
   ```

4. **Running the application:**
   ```bash
   make run
   ```

## API Documentation

- Local Swagger UI: http://localhost:8080/swagger/index.html
- Production API docs: https://nannyai.dev/documentation

## Common Tasks

### Adding a New API Endpoint

1. Add route in `internal/server/server.go`
2. Create handler function
3. Add Swagger documentation
4. Write tests
5. Run `make swag` to update docs

### Adding a New Service

1. Create new package in `internal/`
2. Implement interfaces
3. Add to dependency injection in `main.go`
4. Write tests
5. Update documentation

### Database Changes

1. Add models in appropriate package
2. Update repository implementation
3. Write migration if needed
4. Update tests
5. Document changes

## Testing

Run all tests:
```bash
make test
```

Run specific tests:
```bash
go test ./internal/diagnostic/...
```

## Troubleshooting

### Common Issues

1. **MongoDB Connection Issues**
   - Check MongoDB is running
   - Verify connection string
   - Check network connectivity

2. **Authentication Errors**
   - Verify environment variables
   - Check token expiration
   - Validate API key format

3. **Build Errors**
   - Run `go mod tidy`
   - Clear Go cache
   - Check Go version

## Best Practices

1. **Code Quality**
   - Follow Go style guide
   - Use meaningful variable names
   - Add comments for complex logic
   - Keep functions small and focused

2. **Testing**
   - Write unit tests for new code
   - Include integration tests
   - Maintain >80% coverage

3. **Documentation**
   - Update API documentation
   - Add code comments
   - Update README if needed

4. **Security**
   - Never commit secrets
   - Validate all inputs
   - Follow security guidelines
   - Use proper error handling

## Deployment

See [DEPLOYMENT.md](DEPLOYMENT.md) for detailed deployment instructions.

## Contributing

See [Contributors.md](../Contributors.md) for contribution guidelines.
