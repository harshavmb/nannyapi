# Quick Start Guide

This guide will help you get started with NannyAPI development quickly.

## Prerequisites

- Go 1.24 or higher
- Docker and Docker Compose
- Git
- Make

## Initial Setup

1. Clone the repository:
```bash
git clone https://github.com/harshavmb/nannyapi.git
cd nannyapi
```

2. Run the development setup script:
```bash
./scripts/setup-dev.sh
```

This script will:
- Install required development tools
- Set up your environment configuration
- Generate API documentation
- Start required services (MongoDB)
- Run initial code quality checks

## Configuration

After running the setup script, you need to:

1. Edit `.env` file with your configuration:
   - Set `NANNY_ENCRYPTION_KEY` (32 bytes)
   - Configure GitHub OAuth (`GH_CLIENT_ID`, `GH_CLIENT_SECRET`)
   - Set AI service API keys if needed

2. Verify MongoDB connection:
```bash
make run
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

## Troubleshooting

### Common Issues

1. **MongoDB Connection Issues**
   ```bash
   docker-compose ps
   docker-compose logs mongo
   ```

2. **Swagger Generation Errors**
   ```bash
   make swag
   ```

3. **Test Failures**
   ```bash
   make test
   go test -v ./... -run TestSpecificFunction
   ```

### Getting Help

- Check existing issues on GitHub
- Review documentation in `/docs`
- Ask in team chat
- Create a new issue

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

See [DEPLOYMENT.md](./DEPLOYMENT.md) for detailed deployment instructions.

## Contributing

See [Contributors.md](../Contributors.md) for contribution guidelines.
