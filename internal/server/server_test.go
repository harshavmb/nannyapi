package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

func setupServer(t *testing.T) (*Server, func(), string) {
	// Set the template path for testing
	os.Setenv("NANNY_TEMPLATE_PATH", "../../static/index.html")

	// Mock Gemini Client
	mockGeminiClient := &api.GeminiClient{}

	// Mock GitHub Auth
	mockGitHubAuth := &auth.GitHubAuth{}

	// Connect to test database
	client, cleanup := setupTestDB(t)
	//defer cleanup()

	// Create a new User Repository
	userRepository := user.NewUserRepository(client.Database(testDBName))
	authTokenRepository := user.NewAuthTokenRepository(client.Database(testDBName))
	agentInfoRepository := user.NewAgentInfoRepository(client.Database(testDBName))

	// Mock User Service
	mockUserService := user.NewUserService(userRepository, authTokenRepository)
	agentInfoservice := user.NewAgentInfoService(agentInfoRepository)

	// Create a new server instance
	server := NewServer(mockGeminiClient, mockGitHubAuth, mockUserService, agentInfoservice)

	// Create a valid auth token for the test user
	testUser := &user.User{
		Email:        "test@example.com",
		Name:         "Find Me",
		AvatarURL:    "http://example.com/avatar.png",
		HTMLURL:      "http://example.com",
		LastLoggedIn: time.Now(),
	}
	encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		t.Fatal("NANNY_ENCRYPTION_KEY not set")
	}
	err := mockUserService.SaveUser(context.Background(), map[string]interface{}{
		"email":          testUser.Email,
		"name":           testUser.Name,
		"avatar_url":     testUser.AvatarURL,
		"html_url":       testUser.HTMLURL,
		"last_logged_in": testUser.LastLoggedIn,
	})
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	authToken, err := mockUserService.CreateAuthToken(context.Background(), testUser.Email, encryptionKey)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}

	decryptedToken, err := user.Decrypt(authToken.Token, encryptionKey)
	if err != nil {
		log.Fatalf("Failed to decrypt token: %v", err)
	}

	return server, cleanup, decryptedToken
}

func TestHandleStatus(t *testing.T) {
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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
	server, cleanup, _ := setupServer(t)
	defer cleanup()

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

func TestHandleAgentInfo(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Create a test request with valid agent info
		agentInfo := `{"hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"}`
		req, err := http.NewRequest("POST", "/api/agent-info", strings.NewReader(agentInfo))
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
		}

		// Check the response body
		expected := `{"message":"Agent info saved successfully"}`
		actual := strings.TrimSpace(recorder.Body.String())
		if actual != expected {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})

	t.Run("InvalidRequestPayload", func(t *testing.T) {
		// Create a test request with invalid agent info
		agentInfo := `{"hostname":"test-host","ip_address":"192.168.1.1"}`
		req, err := http.NewRequest("POST", "/api/agent-info", strings.NewReader(agentInfo))
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, but got %d", http.StatusBadRequest, recorder.Code)
		}

		// Check the response body
		expected := "All fields (hostname, ip_address, kernel_version) are required"
		actual := strings.TrimSpace(recorder.Body.String())
		if actual != expected {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})

	t.Run("UserNotAuthenticated", func(t *testing.T) {
		// Create a test request with valid agent info
		agentInfo := `{"hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"}`
		req, err := http.NewRequest("POST", "/api/agent-info", strings.NewReader(agentInfo))
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
	})
}

func TestHandleGetAgentInfoByID(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &user.AgentInfo{
			Email:         "test@example.com",
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		if err != nil {
			t.Fatalf("Failed to save agent info: %v", err)
		}

		// Fetch the inserted ID
		agentInfoID := insertResult.ID.Hex()

		// Create a test request to retrieve agent info by ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/agent-info?id=%s", agentInfoID), nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}

		// Check the response body
		expected := fmt.Sprintf(`[{"_id":"%s","email":"test@example.com","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0","created_at":"`, agentInfoID) // Partial match
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) {
			t.Errorf("Expected body to contain %q, but got %q", expected, actual)
		}
	})

	t.Run("IDNotProvided", func(t *testing.T) {
		// Create a test request without ID
		req, err := http.NewRequest("GET", "/api/agent-info", nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, but got %d", http.StatusBadRequest, recorder.Code)
		}

		// Check the response body
		expected := "ID is required\n"
		actual := strings.TrimSpace(recorder.Body.String())
		if actual != expected {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})
}
