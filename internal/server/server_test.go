package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db"
	testCollectionName = "servers"
)

func setupTestDB(t *testing.T) (*mongo.Client, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Cleanup function to drop the test database after tests
	cleanup := func() {
		err := client.Database(testDBName).Drop(context.Background())
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
		err = client.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("Failed to disconnect from MongoDB: %v", err)
		}
	}

	return client, cleanup
}

func setupServer(t *testing.T) *Server {
	// Set the template path for testing
	os.Setenv("NANNY_TEMPLATE_PATH", "../../static/index.html")

	// Mock Gemini Client
	mockGeminiClient := &api.GeminiClient{}

	// Mock GitHub Auth
	mockGitHubAuth := &auth.GitHubAuth{}

	// Connect to test database
	client, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a new User Repository
	userRepository := user.NewUserRepository(client.Database(testDBName))
	authTokenRepository := user.NewAuthTokenRepository(client.Database(testDBName))

	// Mock User Service
	mockUserService := user.NewUserService(userRepository, authTokenRepository)

	// Create a new server instance
	server := NewServer(mockGeminiClient, mockGitHubAuth, mockUserService)

	return server
}

func TestHandleStatus(t *testing.T) {
	server := setupServer(t)

	req, err := http.NewRequest("GET", "/status", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	expected := `{"status":"ok"}`
	actual := strings.TrimSpace(recorder.Body.String())
	if actual != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", actual, expected)
	}
}

func TestHandleGetAuthTokens_NoAuth(t *testing.T) {
	server := setupServer(t)

	req, err := http.NewRequest("GET", "/api/auth-tokens", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

func TestHandleDeleteAuthToken_NoAuth(t *testing.T) {
	server := setupServer(t)

	tokenID := bson.NewObjectID().Hex()
	req, err := http.NewRequest("DELETE", fmt.Sprintf("/api/auth-tokens/%s", tokenID), nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

func TestHandleAuthTokensPage(t *testing.T) {
	server := setupServer(t)

	req, err := http.NewRequest("GET", "/auth-tokens", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusUnauthorized)
	}
}

func TestHandleCreateAuthToken_NoAuth(t *testing.T) {
	server := setupServer(t)

	req, err := http.NewRequest("POST", "/create-auth-token", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusSeeOther {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusSeeOther)
	}
}

func TestHandleIndex(t *testing.T) {
	server := setupServer(t)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, req)

	if status := recorder.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

// FIX-ME, NOT WORKING, panic
// func TestAuthMiddleware_ValidToken(t *testing.T) {
// 	// Set up the server
// 	server := setupServer(t)

// 	// Set up a test user
// 	testUser := &user.User{
// 		Email: "test@example.com",
// 	}

// 	// Set up a test auth token
// 	encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
// 	if encryptionKey == "" {
// 		t.Fatal("NANNY_ENCRYPTION_KEY not set")
// 	}

// 	authToken, err := server.userService.CreateAuthToken(context.Background(), testUser.Email, encryptionKey)
// 	if err != nil {
// 		t.Fatalf("Failed to create auth token: %v", err)
// 	}

// 	// Create a test request
// 	req, err := http.NewRequest("GET", "/api/auth-tokens", nil)
// 	if err != nil {
// 		t.Fatalf("Could not create request: %v", err)
// 	}

// 	// Set the Authorization header with the valid token
// 	req.Header.Set("Authorization", "Bearer "+authToken.Token)

// 	// Create a test recorder
// 	recorder := httptest.NewRecorder()

// 	// Serve the request
// 	server.ServeHTTP(recorder, req)

// 	// Check the response status code
// 	if recorder.Code != http.StatusOK {
// 		t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
// 	}

// 	// Check the response body
// 	body, err := io.ReadAll(recorder.Body)
// 	if err != nil {
// 		t.Fatalf("Could not read response body: %v", err)
// 	}

// 	// Unmarshal the response body
// 	var authTokens []AuthTokenData
// 	err = json.Unmarshal(body, &authTokens)
// 	if err != nil {
// 		t.Fatalf("Could not unmarshal response body: %v", err)
// 	}

// 	// Check that the response contains the expected data
// 	if len(authTokens) == 0 {
// 		t.Errorf("Expected at least one auth token, but got none")
// 	}
// }

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	// Set up the server
	server := setupServer(t)

	// Create a test request with an invalid token
	req, err := http.NewRequest("GET", "/api/auth-tokens", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer invalid-token")

	// Create a test recorder
	recorder := httptest.NewRecorder()

	// Serve the request
	server.ServeHTTP(recorder, req)

	// Check the response status code
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, but got %d", http.StatusUnauthorized, recorder.Code)
	}

	// Check the response body
	body, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatalf("Could not read response body: %v", err)
	}
	expected := "Invalid auth token\n"
	if string(body) != expected {
		t.Errorf("Expected body %q, but got %q", expected, string(body))
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Set up the server
	server := setupServer(t)

	// Create a test request without a token
	req, err := http.NewRequest("GET", "/api/auth-tokens", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	// Create a test recorder
	recorder := httptest.NewRecorder()

	// Serve the request
	server.ServeHTTP(recorder, req)

	// Check the response status code
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, but got %d", http.StatusUnauthorized, recorder.Code)
	}

	// Check the response body
	body, err := io.ReadAll(recorder.Body)
	if err != nil {
		t.Fatalf("Could not read response body: %v", err)
	}
	expected := "Authorization header is required\n"
	if string(body) != expected {
		t.Errorf("Expected body %q, but got %q", expected, string(body))
	}
}
