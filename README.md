# nannyapi

[![CI](https://github.com/harshavmb/nannyapi/actions/workflows/ci.yml/badge.svg)](https://github.com/harshavmb/nannyapi/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/harshavmb/nannyapi/branch/main/graph/badge.svg)](https://codecov.io/gh/harshavmb/nannyapi)
[![Go Report Card](https://goreportcard.com/badge/github.com/harshavmb/nannyapi)](https://goreportcard.com/report/github.com/harshavmb/nannyapi)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

This repo is an API endpoint service that receives prompts from nannyagents, does some preprocessing, and interacts with remote/self-hosted AI APIs to help diagnose system issues.

## Getting Started

To run the server, navigate to the project directory and execute the following command:

```bash
go run ./cmd/main.go
```

## Prerequisites
- Go 1.24 or higher
- Docker and Docker Compose
- Git
- Make

## Installation
Follow the [Quick Start Guide](docs/QUICKSTART.md) for detailed setup instructions.

## Configuration
The application uses environment variables for configuration. Required variables:

- `MONGODB_URI` - MongoDB connection string
- `NANNY_ENCRYPTION_KEY` - (32 bytes) Used for encrypting sensitive data
- `JWT_SECRET` - Secret for JWT token signing
- `GH_CLIENT_ID` - GitHub OAuth client ID
- `GH_CLIENT_SECRET` - GitHub OAuth client secret
- `DEEPSEEK_API_KEY` - DeepSeek API key for AI services

## API Endpoints

The API endpoints are documented using Swagger. All API interactions are logged for audit purposes.

### Authentication Endpoints
- `POST /github/login` - GitHub OAuth login
- `GET /github/callback` - GitHub OAuth callback
- `GET /github/profile` - Get GitHub profile

### User Management
- `GET /api/user/{id}` - Get user info by ID
- `GET /api/user-auth-token` - Get user info from auth token

### Auth Token Management
- `POST /api/auth-token` - Create new auth token
- `GET /api/auth-tokens` - List auth tokens
- `DELETE /api/auth-token/{id}` - Delete auth token

### Agent Management
- `POST /api/agent-info` - Register agent information
- `GET /api/agent-info/{id}` - Get agent info by ID
- `GET /api/agents` - List all agents

### Diagnostic Endpoints
- `POST /api/diagnostic` - Start diagnostic session
- `POST /api/diagnostic/{id}/continue` - Continue diagnostic session
- `GET /api/diagnostic/{id}` - Get diagnostic session details
- `GET /api/diagnostic/{id}/summary` - Get diagnostic summary
- `DELETE /api/diagnostic/{id}` - Delete diagnostic session
- `GET /api/diagnostics` - List all diagnostic sessions

### Status
- `GET /status` - Get API service status

> **Security Note**: All API endpoints under `/api/` require authentication using either JWT Bearer token or API key.

## Audit Logging

Every interaction between agents and the API is comprehensively logged for audit purposes, including:
- All diagnostic sessions and their results
- System metric changes
- Authentication attempts
- Token creation/deletion events

Logs are stored in structured JSON format with timestamps and relevant metadata for easy analysis and compliance requirements.

## Documentation
For more detailed documentation, please refer to:
- [API Documentation](https://nannyai.dev/docs)
- [System Architecture](docs/ARCHITECTURE.md)
- [Deployment Guide](docs/DEPLOYMENT.md)
- [Contributing Guidelines](Contributors.md)

## License
This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.
