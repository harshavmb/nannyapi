# nannyapi
This repo is an API endpoint service that receives prompts from nannyagents, does some preprocessing, and interacts with remote/self-hosted AI APIs to help answer prompts issued by nannyagents.

## Project Structure

```
nannyapi
├── cmd
│   └── main.go        # Entry point of the application
├── internal
│   ├── server
│   │   └── server.go  # Implementation of the server
│   ├── user
│   │   ├── repository.go         # User and AuthToken repository implementations
│   │   ├── service.go            # User service implementation
│   │   ├── helper.go             # Helper functions for encryption and decryption
│   │   ├── repository_test.go    # Tests for the repository
│   │   ├── service_test.go       # Tests for the service
│   │   ├── helper_test.go        # Tests for the helper functions
├── go.mod             # Module definition file
└── README.md          # Documentation for the project
```

## Getting Started

To run the server, navigate to the project directory and execute the following command:

```
go run main.go
```

## Dependencies

This project uses Go modules for dependency management. Ensure you have Go installed and set up properly.

## Running Tests

To run the tests, navigate to the project directory and execute the following command:

```
go test ./...
```

## Environment Variables

The following environment variables are required for the application to run:

- `NANNY_ENCRYPTION_KEY`: Base64 encoded 32-byte key used for encryption and decryption.

## User Service

The `UserService` handles business logic related to users and authentication tokens. It interacts with the `UserRepository` and `AuthTokenRepository` to perform database operations.

### Methods

- `SaveUser(ctx context.Context, userInfo map[string]interface{}) error`: Saves or updates a user in the database.
- `CreateAuthToken(ctx context.Context, userEmail string) (*AuthToken, error)`: Creates a new authentication token for a user.
- `GetAuthToken(ctx context.Context, userEmail string) (*AuthToken, error)`: Retrieves an authentication token for a user.

## User Repository

The `UserRepository` handles database interactions related to users.

### Methods

- `UpsertUser(ctx context.Context, user *User) (*mongo.UpdateResult, error)`: Inserts or updates a user in the database.
- `FindUserByEmail(ctx context.Context, email string) (*User, error)`: Finds a user by email.

## Auth Token Repository

The `AuthTokenRepository` handles database interactions related to authentication tokens.

### Methods

- `CreateAuthToken(ctx context.Context, userEmail string) (*AuthToken, error)`: Creates a new authentication token for a user.
- `GetAuthTokenByEmail(ctx context.Context, userEmail string) (*AuthToken, error)`: Finds an authentication token by email.
- `UpdateAuthToken(ctx context.Context, authToken *AuthToken) error`: Updates an authentication token in the database.

## Helper Functions

The `helper.go` file contains utility functions for encryption and decryption.

### Functions

- `encrypt(stringToEncrypt, encryptionKey string) (string, error)`: Encrypts a string using the provided encryption key.
- `decrypt(encryptedString, encryptionKey string) (string, error)`: Decrypts an encrypted string using the provided encryption key.