# nannyapi
This repo is an API endpoint service that receives prompts from nannyagents, does some preprocessing, and interacts with remote/self-hosted AI APIs to help answer prompts issued by nannyagents.

## Project Structure

```
nannyapi
├── cmd
│   └── main.go        # Entry point of the application
├── internal
│   ├── server
│   │   ├── server.go  # Implementation of the server
│   │   ├── middleware.go # Authentication middleware
│   ├── user
│   │   ├── repository.go         # User and AuthToken repository implementations
│   │   ├── service.go            # User service implementation
│   │   ├── helper.go             # Helper functions for encryption, decryption, and token generation
│   │   ├── repository_test.go    # Tests for the repository
│   │   ├── service_test.go       # Tests for the service
│   │   ├── helper_test.go        # Tests for the helper functions
│   ├── static
│   │   ├── index.html # index page
│   │   ├── create_auth_token.html # create auth token page
│   │   ├── auth_tokens.html # auth tokens page
├── 

go.mod

             # Module definition file
└── 

README.md

          # Documentation for the project
```

## Getting Started

To run the server, navigate to the project directory and execute the following command:

```bash
go run ./cmd/main.go
```

## Environment Variables

The following environment variables are required to run the server:

*   `NANNY_MONGODB_URI`: The URI for the MongoDB database.
*   `NANNY_ENCRYPTION_KEY`: The encryption key used to encrypt and decrypt auth tokens (Base64 encoded 32-byte key).
*   `NANNY_TEMPLATE_PATH`: The path to the index.html template file (optional, defaults to ./static/index.html).

## API Endpoints

The following API endpoints are available:

*   `/status`: Returns the status of the server.
*   `/`: Serves the index page.
*   `/create-auth-token`: Creates a new auth token for the user.
*   `/auth-tokens`: Serves the auth tokens page.
*   `/api/auth-tokens`: Retrieves all auth tokens for the authenticated user (requires authentication).
*   `/api/auth-tokens/{id}`: Deletes a specific auth token by ID (requires authentication).

## Authentication

The API endpoints under `/api/` require authentication using an auth token. The auth token should be passed in the `Authorization` header as a Bearer token.

## Tests

To run the tests, navigate to the project directory and execute the following command:

```bash
go test ./...
```

## User Service

The `UserService` handles business logic related to users and authentication tokens. It interacts with the `UserRepository` to perform database operations.

### Methods

*   `CreateUser(ctx context.Context, user User) (*User, error)`: Creates a new user.
*   `GetUserByEmail(ctx context.Context, email string) (*User, error)`: Retrieves a user by email.
*   `CreateAuthToken(ctx context.Context, email string, encryptionKey string) (*AuthToken, error)`: Creates a new auth token.
*   `GetAuthToken(ctx context.Context, email string, encryptionKey string) (*AuthToken, error)`: Retrieves an auth token by email.
*   `GetAuthTokenByToken(ctx context.Context, token string, encryptionKey string) (*AuthToken, error)`: Retrieves an auth token by token.
*   `GetAllAuthTokens(ctx context.Context, email string) ([]AuthToken, error)`: Retrieves all auth tokens for a user.
*   `DeleteAuthToken(ctx context.Context, id primitive.ObjectID) error`: Deletes an auth token.

## User Repository

The `UserRepository` handles database interactions related to users and auth tokens.

### Methods

*   `CreateUser(ctx context.Context, user User) (*User, error)`: Creates a new user.
*   `GetUserByEmail(ctx context.Context, email string) (*User, error)`: Retrieves a user by email.
*   `CreateAuthToken(ctx context.Context, authToken AuthToken) (*AuthToken, error)`: Creates a new auth token.
*   `GetAuthToken(ctx context.Context, email string) (*AuthToken, error)`: Retrieves an auth token by email.
*   `GetAuthTokenByHashedToken(ctx context.Context, hashedToken string) (*AuthToken, error)`: Retrieves an auth token by hashed token.
*   `GetAllAuthTokens(ctx context.Context, email string) ([]AuthToken, error)`: Retrieves all auth tokens for a user.
*   `DeleteAuthToken(ctx context.Context, id primitive.ObjectID) error`: Deletes an auth token.

## Helper Functions

The `helper.go` file contains utility functions for encryption, decryption, and token generation.

### Functions

*   `generateRandomToken(length int) (string, error)`: Generates a random token of the specified length using alphanumeric characters.
*   `encrypt(stringToEncrypt string, encryptionKey string) (string, error)`: Encrypts a string using AES-256.
*   `Decrypt(encryptedString string, encryptionKey string) (string, error)`: Decrypts a string using AES-256.
*   `GetUserInfoFromCookie(r *http.Request) (*user.User, error)`: Retrieves user information from the "userinfo" cookie.
*   `IsSessionValid(r *http.Request) bool`: Checks if the user session is valid.
*   `hashToken(token string) string`: Hashes the token using SHA-256.