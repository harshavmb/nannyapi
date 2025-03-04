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
go.mod # Module definition file
└── 
README.md # Documentation for the project
```

## Getting Started

To run the server, navigate to the project directory and execute the following command:

```bash
go run ./cmd/main.go
```

### Prerequisites

*   Go 1.22 or higher
*   MongoDB

### Installation

1.  Clone the repository:

    ```bash
    git clone https://github.com/harshavmb/nannyapi.git
    ```

2.  Navigate to the project directory:

    ```bash
    cd nannyapi
    ```

3.  Install dependencies:

    ```bash
    go mod tidy
    ```

### Configuration

The application relies on environment variables for configuration. You can set these variables in your shell or in a `.env` file.

#### Required Environment Variables

*   `MONGODB_URI`: The URI for your MongoDB instance. Example: `mongodb://localhost:27017`
*   `NANNY_ENCRYPTION_KEY`: The encryption key used for encrypting auth tokens.  Must be 32 bytes long.
*   `GH_CLIENT_ID`: Your GitHub OAuth client ID.
*   `GH_CLIENT_SECRET`: Your GitHub OAuth client secret.

#### Optional Environment Variables

*   `PORT`: The port the server listens on. Defaults to `8080`.
*   `GH_REDIRECT_URL`: The URL to redirect to after GitHub authentication. Defaults to `http://localhost:8080/github/callback`.
*   `SWAGGER_DOC_URL`: The URL to the Swagger documentation. Defaults to `http://localhost:8080/swagger/doc.json`.
*   `NANNY_TEMPLATE_PATH`: Path to the index.html template. Defaults to 
index.html.

### Running the Application

```bash
go run ./cmd/main.go
```

## API Endpoints

The API endpoints are documented using Swagger. You can view the Swagger UI by navigating to:

```
http://localhost:8080/swagger/index.html
```

(Replace `localhost:8080` with your actual host and port if you've overridden the defaults.)

The base path for the API is `/api/v1`.

### Available Endpoints

*   `/status`: Returns the status of the server.
*   `/create-auth-token`: Creates a new auth token for a user.
*   `/api/auth-tokens`: Retrieves all auth tokens for a user.
*   `/api/auth-tokens/{id}`: Deletes an auth token by ID.
*   `/api/chat/{id}`: Retrieves a chat by ID.
*   `/api/agent-info`: Ingests agent information (POST).
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

The `UserRepository` handles database interactions related to users and authentication tokens.

## Helper Functions

The `helper.go` file contains utility functions for encryption, decryption, and token generation.

### Functions

*   `generateRandomToken(length int) (string, error)`: Generates a random token of the specified length using alphanumeric characters.
*   `encrypt(stringToEncrypt string, encryptionKey string) (string, error)`: Encrypts a string using AES-256.
*   `Decrypt(encryptedString string, encryptionKey string) (string, error)`: Decrypts a string using AES-256.
*   `GetUserInfoFromCookie(r *http.Request) (*user.User, error)`: Retrieves user information from the "userinfo" cookie.
*   `IsSessionValid(r *http.Request) bool`: Checks if the user session is valid.
*   `hashToken(token string) string`: Hashes the token using SHA-256.

## Contributing

We welcome contributions! Please see [Contributors](./Contributors.md) for guidelines on how to contribute.

## License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](https://www.gnu.org/licenses/gpl-3.0.html) file for details.