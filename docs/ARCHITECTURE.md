# NannyAPI Architecture

## System Overview

NannyAPI is a service that handles prompts from nanny agents, processes them, and interacts with various AI APIs to provide responses. The system is designed with a clean, modular architecture following Go best practices.

## Architecture Diagram

```mermaid
graph TD
    A[Nanny Agent] -->|HTTP Request| B(API Gateway)
    B --> C{Authentication Middleware}
    C -->|Authenticated| D[Chat Service]
    C -->|Authenticated| E[Agent Service]
    D --> F[Diagnostic Service]
    F -->|AI Requests| G[DeepSeek Client]
    D --> I[(MongoDB)]
    E --> I

    style A fill:#f9f,stroke:#333,stroke-width:2px
    style B fill:#bbf,stroke:#333,stroke-width:2px
    style C fill:#dfd,stroke:#333,stroke-width:2px
```

## Core Components

### 1. Server Layer (`internal/server/`)
- Handles HTTP routing and request processing
- Implements middleware for authentication and logging
- Manages API endpoint versioning
- Provides Swagger documentation

### 2. Authentication (`internal/auth/`)
- GitHub OAuth integration
- Token-based authentication
- Session management

### 3. Chat Service (`internal/chat/`)
- Manages chat sessions
- Processes user prompts
- Coordinates with diagnostic service

### 4. Diagnostic Service (`internal/diagnostic/`)
- Integrates with AI providers (DeepSeek)
- Handles prompt preprocessing
- Manages AI response processing

### 5. Agent Service (`internal/agent/`)
- Manages agent information
- Handles agent registration and updates
- Tracks agent statistics

## Data Flow

1. Client sends request → API Gateway
2. Authentication middleware validates token
3. Request routed to appropriate service
4. Service processes request
5. Response returned to client

## Database Schema

```mermaid
erDiagram
    User ||--o{ AuthToken : has
    User {
        ObjectID id
        string email
        string name
        datetime created_at
    }
    AuthToken {
        ObjectID id
        string token
        string user_id
        datetime created_at
        datetime expires_at
    }
    Chat ||--o{ Message : contains
    Chat {
        ObjectID id
        string user_id
        datetime created_at
    }
    Message {
        ObjectID id
        string chat_id
        string content
        string role
        datetime created_at
    }
```

## Security Considerations

1. **Authentication**
   - Token-based authentication
   - GitHub OAuth integration
   - Secure session management

2. **Data Protection**
   - Encryption at rest
   - Secure communication channels
   - Token encryption

3. **API Security**
   - Rate limiting
   - Input validation
   - CORS configuration

## Configuration Management

The system uses environment variables for configuration:
- Database connections
- API keys
- Service endpoints
- Feature flags

## Monitoring and Logging

- Structured logging
- Request/Response tracking
- Error monitoring
- Performance metrics

## Testing Strategy

1. **Unit Tests**
   - Service layer tests
   - Repository tests
   - Helper function tests

2. **Integration Tests**
   - API endpoint tests
   - Database integration tests
   - External service mocks

3. **Performance Tests**
   - Load testing
   - Concurrency testing
   - Resource usage monitoring

## Deployment

The service is designed to be deployed as a containerized application, supporting:
- Docker containers
- Kubernetes orchestration
- Cloud-native scaling
- Zero-downtime deployments
