package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/diagnostic"
	"github.com/harshavmb/nannyapi/internal/token"
	"github.com/harshavmb/nannyapi/internal/user"
)

const (
	testDBName         = "test_db"
	testCollectionName = "servers"
	jwtSecret          = "d2a8b6aad8fb7d736508a520e2d53460054d21b14c1a8be86ec61e654ee807e6d47e167628bdeb59d7da25ac4de4ab1cbc161b2a335924b89e22fdac3bc44511e9fa896031b3154fd7365fe01c539ef5681ba70a65619eae8c7c14b832ea989d779d828a4e95e63181ae70ad0d855a40477144cc892097e0b0c0abfd5a26ce5f8bc0159bf44171a6dcd295aa810c4759ae0a0bc0f13b9f5872fd048ab9daa94c64d5e999dc7ea928f5a87731b468c25f2a67a6180f8f99bd9d38c706f9ca77f74e0929b5abec65c3b26d641f57a6c683a0770880748ebc5804ada5179a0252228b1a328898cae4a0d987767889251eda344cb45fd4725099de8f0947328a6166" // just for testing not used anywhere
	encryptionKey      = "T3byOVRJGt/25v6c6GC3wWkNKtL1WPuW5yVjCEnaHA8="                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     // Base64 encoded 32-byte key
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
		err := client.Database(testDBName).Collection(testCollectionName).Drop(context.Background())
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

func setupServer(t *testing.T) (*Server, func(), token.Token, string) {
	// Mock GitHub Auth
	mockGitHubAuth := &auth.GitHubAuth{}

	// Connect to test database
	client, cleanup := setupTestDB(t)

	// Create a new Repository objects
	userRepository := user.NewUserRepository(client.Database(testDBName))
	tokenRepository := token.NewTokenRepository(client.Database(testDBName))
	refreshTokenRepository := token.NewRefreshTokenRepository(client.Database(testDBName))
	agentInfoRepository := agent.NewAgentInfoRepository(client.Database(testDBName))
	diagnosticRepository := diagnostic.NewDiagnosticRepository(client.Database(testDBName))

	// Mock Services
	mockUserService := user.NewUserService(userRepository)
	agentInfoservice := agent.NewAgentInfoService(agentInfoRepository)
	mockTokenService := token.NewTokenService(tokenRepository)
	mockRefreshTokenService := token.NewRefreshTokenService(refreshTokenRepository)
	diagnosticService := diagnostic.NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"), diagnosticRepository, agentInfoservice)

	// Create a new server instance
	server := NewServer(mockGitHubAuth, mockUserService, agentInfoservice, mockTokenService, mockRefreshTokenService, diagnosticService, jwtSecret, encryptionKey)

	// Create a valid auth token for the test user
	testUser := &user.User{
		Email:        "test@example.com",
		Name:         "Find Me",
		AvatarURL:    "http://example.com/avatar.png",
		HTMLURL:      "http://example.com",
		LastLoggedIn: time.Now(),
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

	user, err := server.userService.GetUserByEmail(context.Background(), testUser.Email)
	if err != nil {
		log.Fatalf("error while fetching user: %v", err)
	}

	if user == nil {
		log.Fatalf("user not found")
	}

	staticToken := token.Token{
		UserID: user.ID.Hex(),
		Token:  token.GenerateRandomString(10),
	}

	_, err = mockTokenService.CreateToken(context.Background(), staticToken, encryptionKey)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}

	accessToken, err := generateAccessToken(staticToken.UserID, jwtSecret)
	if err != nil {
		t.Fatalf("Failed to create access token: %v", err)
	}

	return server, cleanup, staticToken, accessToken
}

func TestHandleDeleteAuthToken(t *testing.T) {
	server, cleanup, _, accessToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// create the token
		testTokenObj := token.Token{
			UserID: "123456",
			Token:  "adfadsfdsfdsfadsf",
		}

		tokenCreated, err := server.tokenService.CreateToken(context.Background(), testTokenObj, server.nannyEncryptionKey)
		if err != nil {
			log.Fatalf("error while creating token: %v", err)
		}

		// Delete the token
		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", "/api/auth-token", tokenCreated.ID.Hex()), nil)
		if err != nil {
			t.Fatalf("Could not delete token: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}

		// Check the response body
		expected := `{"message":"Auth token deleted successfully"}`
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}

	})

	t.Run("InValidRequest", func(t *testing.T) {
		// create the token
		testTokenObj := token.Token{
			UserID: "123456",
			Token:  "adfadsfdsfdsfadsf",
		}

		_, err := server.tokenService.CreateToken(context.Background(), testTokenObj, server.nannyEncryptionKey)
		if err != nil {
			log.Fatalf("error while creating token: %v", err)
		}

		// Delete the token that doesn't exist
		req, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", "/api/auth-token", bson.NewObjectID().Hex()), nil) // sending an incorrect one
		if err != nil {
			t.Fatalf("Could not delete token: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusNotFound {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}

	})

	t.Run("UserNotAuthenticated", func(t *testing.T) {
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
	})
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	// Set up the server
	server, cleanup, _, _ := setupServer(t)
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
	expected := "Invalid access token\n"
	if string(body) != expected {
		t.Errorf("Expected body %q, but got %q", expected, string(body))
	}
}

func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Set up the server
	server, cleanup, _, _ := setupServer(t)
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
	expected := "One of Authorization/X-NANNYAPI-Key headers is required\n"
	if string(body) != expected {
		t.Errorf("Expected body %q, but got %q", expected, string(body))
	}
}

func TestAuthMiddleware_BothStaticAndAccessTokens(t *testing.T) {
	// Set up the server
	server, cleanup, apiKey, accesstoken := setupServer(t)
	defer cleanup()

	// Create a test request without a token
	req, err := http.NewRequest("GET", "/api/auth-tokens", nil)
	if err != nil {
		t.Fatalf("Could not create request: %v", err)
	}

	// set both Access token and API keys
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accesstoken))
	req.Header.Set("X-NANNYAPI-Key", apiKey.Token)

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
	expected := "Only one of Authorization/X-NANNYAPI-Key headers is required\n"
	if string(body) != expected {
		t.Errorf("Expected body %q, but got %q", expected, string(body))
	}
}

func TestHandleAgentInfoWithAccessToken(t *testing.T) {
	server, cleanup, _, accessToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Create a test request with valid agent info
		agentInfo := `{"hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0","os_version":"Ubuntu 24.04"}`
		req, err := http.NewRequest("POST", "/api/agent-info", strings.NewReader(agentInfo))
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
		}

		// Check the response body
		var response map[string]string
		err = json.NewDecoder(recorder.Body).Decode(&response)
		if err != nil {
			t.Fatalf("Could not decode response body: %v", err)
		}

		id, ok := response["id"]
		if !ok {
			t.Errorf("Expected response to contain 'id' field")
		}

		if _, err := bson.ObjectIDFromHex(id); err != nil {
			t.Errorf("Expected 'id' field to be a valid ObjectID, but got %v", id)
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
		req.Header.Set("Authorization", "Bearer "+accessToken)

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

func TestHandleCreateAuthToken(t *testing.T) {
	server, cleanup, _, accessToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		req, err := http.NewRequest("POST", "/api/auth-token", nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+accessToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusCreated {
			t.Errorf("Expected status code %d, but got %d", http.StatusCreated, recorder.Code)
		}

		// Check the response body
		var responseToken token.Token
		err = json.NewDecoder(recorder.Body).Decode(&responseToken)
		if err != nil {
			t.Fatalf("Could not decode response body: %v", err)
		}

		id := responseToken.ID.Hex()

		if _, err := bson.ObjectIDFromHex(id); err != nil {
			t.Errorf("Expected 'id' field to be a valid ObjectID, but got %v", id)
		}

		// Verify the token is same by checking the hash
		tokenHash := token.HashToken(responseToken.Token)
		assert.Equal(t, responseToken.HashedToken, tokenHash)
	})

	t.Run("UserNotAuthenticated", func(t *testing.T) {
		// Create a test request with valid token info
		requestToken := fmt.Sprintf(`{"user_id":"123456","token":"%s"}`, "abcdcadscds") // token model
		req, err := http.NewRequest("POST", "/api/auth-token", strings.NewReader(requestToken))
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
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		if err != nil {
			t.Fatalf("Failed to save agent info: %v", err)
		}

		// Fetch the inserted ID
		agentInfoID := insertResult.InsertedID.(bson.ObjectID).Hex()

		// Create a test request to retrieve agent info by ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/agent-info/%s", agentInfoID), nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}

		// Check the response body
		expected := fmt.Sprintf(`{"id":"%s","user_id":"%s","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"`, agentInfoID, validToken.UserID) // Partial match
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) {
			t.Errorf("Expected body to contain %q, but got %q", expected, actual)
		}
	})

	t.Run("IDNotProvided", func(t *testing.T) {
		// Create a test request without ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/agent-info/%s", ""), nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid Authorization header
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, but got %d", http.StatusBadRequest, recorder.Code)
		}

		// Check the response body
		expected := "Agent ID is required"
		actual := strings.TrimSpace(recorder.Body.String())
		if actual != expected {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})
}

func TestNannyAPIPortOverride(t *testing.T) {
	// Set the environment variable
	os.Setenv("NANNY_API_PORT", "9090")
	defer os.Unsetenv("NANNYAPI_PORT")

	server, cleanup, _, _ := setupServer(t)
	defer cleanup()

	// Check if the server is running on the correct port
	assert.Equal(t, "9090", server.nannyAPIPort)

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

func TestSwaggerURL(t *testing.T) {
	// Set the environment variable
	os.Setenv("NANNY_SWAGGER_URL", "http://localhost:9090/swagger/doc.json")
	defer os.Unsetenv("NANNY_SWAGGER_URL")

	server, cleanup, _, _ := setupServer(t)
	defer cleanup()

	// Check if the server is running on the correct github callback url
	assert.Equal(t, os.Getenv("NANNY_SWAGGER_URL"), server.nannySwaggerURL)
}

func TestGitHubRedirectURL(t *testing.T) {
	// Set the environment variable
	os.Setenv("GH_REDIRECT_URL", "http://example.net/swagger/doc.json")
	defer os.Unsetenv("GH_REDIRECT_URL")

	server, cleanup, _, _ := setupServer(t)
	defer cleanup()

	// Check if the server is running on the correct github callback url
	assert.Equal(t, os.Getenv("GH_REDIRECT_URL"), server.gitHubRedirectURL)
}

func TestHandleAgentInfosWithAPIKey(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		if err != nil {
			t.Fatalf("Failed to save agent info: %v", err)
		}

		// Fetch the inserted ID
		agentInfo.ID = insertResult.InsertedID.(bson.ObjectID)
		agentInfoID := agentInfo.ID.Hex()

		// Create a test request to retrieve agents
		req, err := http.NewRequest("GET", "/api/agents", nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		// Unmarshal the response body into a slice of AgentInfo
		var actualAgentInfos []agent.AgentInfo
		err = json.Unmarshal(recorder.Body.Bytes(), &actualAgentInfos)
		if err != nil {
			t.Fatalf("Failed to unmarshal response body: %v", err)
		}

		// Check if the expected agent info is present in the actual response
		found := false
		for _, actualAgent := range actualAgentInfos {
			if actualAgent.ID.Hex() == agentInfoID &&
				actualAgent.UserID == agentInfo.UserID &&
				actualAgent.Hostname == agentInfo.Hostname &&
				actualAgent.IPAddress == agentInfo.IPAddress &&
				actualAgent.KernelVersion == agentInfo.KernelVersion {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected to find the inserted agent info in the response, but it was not found.\nExpected ID: %s\nActual Response: %s", agentInfoID, recorder.Body.String())
		}
	})

}

func TestHandleFetchUserInfoFromID(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidEmail", func(t *testing.T) {
		// Get the user via ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/user/%s", validToken.UserID), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]string
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Equal(t, "test@example.com", response["email"])
		assert.Equal(t, "Find Me", response["name"])
		assert.Equal(t, "http://example.com/avatar.png", response["avatar_url"])
	})

	t.Run("UserNotFound", func(t *testing.T) {
		// Create a request with an email that does not exist in the database
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/user/%s", bson.NewObjectID().Hex()), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		expected := `{"error":"User not found"}`
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})

	t.Run("UnauthorizedRequest", func(t *testing.T) {
		// Create a request with a valid email
		req, err := http.NewRequest("GET", "/api/user/asdfadsfsf", nil)
		assert.NoError(t, err)

		// Do not set an Authorization header
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)

		expected := `One of Authorization/X-NANNYAPI-Key headers is required`
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})
}

func TestHandleRefreshToken(t *testing.T) {
	server, cleanup, _, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRefreshToken", func(t *testing.T) {
		userID := "test-user-id"
		// Generate the refresh token
		tokenString, err := generateRefreshToken(userID, jwtSecret)
		if err != nil {
			log.Fatalf("error generating refresh token %v", err)
		}

		// Create a valid refresh token
		refreshToken, err := server.refreshTokenservice.CreateRefreshToken(context.Background(), token.RefreshToken{
			UserID:    userID,
			UserAgent: "test-agent",
			IPAddress: "127.0.0.1",
			Token:     tokenString,
		}, server.nannyEncryptionKey)
		assert.NoError(t, err)

		// Create a request with the valid refresh token
		req, err := http.NewRequest("POST", "/api/refresh-token", nil)
		assert.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:     "refresh_token",
			Value:    tokenString,
			HttpOnly: true,
		})

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusCreated, recorder.Code)

		var response map[string]string
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)
		assert.NotEmpty(t, response["access_token"])
		assert.NotEmpty(t, response["refresh_token"])
		assert.Equal(t, token.HashToken(tokenString), refreshToken.HashedToken)
	})

	t.Run("ExpiredRefreshToken", func(t *testing.T) {
		userID := "test-user-id"
		// Generate the refresh token
		tokenString, err := generateRefreshToken(userID, jwtSecret)
		if err != nil {
			log.Fatalf("error generating refresh token %v", err)
		}

		// Create a valid refresh token
		refreshToken, err := server.refreshTokenservice.CreateRefreshToken(context.Background(), token.RefreshToken{
			UserID:    userID,
			UserAgent: "test-agent",
			IPAddress: "127.0.0.1",
			Token:     tokenString,
		}, server.nannyEncryptionKey)
		assert.NoError(t, err)

		// Simulate an expired refresh token
		updatedToken := token.RefreshToken{
			ID:        refreshToken.ID,
			UserID:    refreshToken.UserID,
			Token:     tokenString,
			CreatedAt: refreshToken.CreatedAt,
			ExpiresAt: time.Now().AddDate(0, 0, -10), // +7-10 == -3 days back
			Revoked:   refreshToken.Revoked,
			UserAgent: refreshToken.UserAgent,
			IPAddress: refreshToken.IPAddress,
		}

		// update the token now
		err = server.refreshTokenservice.UpdateRefreshToken(context.Background(), updatedToken, server.nannyEncryptionKey)
		if err != nil {
			log.Fatalf("error updating refresh token %v", err)
		}

		// Create a request with the expired refresh token
		req, err := http.NewRequest("POST", "/api/refresh-token", nil)
		assert.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:     "refresh_token",
			Value:    tokenString,
			HttpOnly: true,
		})

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusCreated, recorder.Code)

		var response map[string]string
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)
		assert.NotEmpty(t, response["access_token"])
		assert.NotEmpty(t, response["refresh_token"])
	})

	t.Run("InvalidRefreshToken", func(t *testing.T) {
		// Create a request with an invalid refresh token
		req, err := http.NewRequest("POST", "/api/refresh-token", nil)
		assert.NoError(t, err)
		req.AddCookie(&http.Cookie{
			Name:     "refresh_token",
			Value:    "invalid-token",
			HttpOnly: true,
		})

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)

		expected := "invalid token"
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Contains(t, actual, expected)
	})

	t.Run("MissingRefreshToken", func(t *testing.T) {
		// Create a request without a refresh token
		req, err := http.NewRequest("POST", "/api/refresh-token", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		expected := "Refresh token is required"
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})
}

func TestHandleStartDiagnostic(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
			SystemMetrics: agent.SystemMetrics{
				CPUInfo:     []string{"Intel i7-1165G7"},
				CPUUsage:    45.5,
				MemoryTotal: 16 * 1024 * 1024 * 1024, // 16GB
				MemoryUsed:  8 * 1024 * 1024 * 1024,  // 8GB
				MemoryFree:  8 * 1024 * 1024 * 1024,  // 8GB
				DiskUsage: map[string]int64{
					"/":     250 * 1024 * 1024 * 1024, // 250GB
					"/home": 500 * 1024 * 1024 * 1024, // 500GB
				},
				FSUsage: map[string]string{
					"/":     "45%",
					"/home": "60%",
				},
			},
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		assert.NotEmpty(t, session.ID)
		assert.Equal(t, agentID, session.AgentID)
		assert.Equal(t, "High CPU usage", session.InitialIssue)
		assert.Equal(t, 0, session.CurrentIteration)
		assert.Equal(t, "in_progress", session.Status)
		assert.NotEmpty(t, session.History)

		// Verify system metrics are captured in the diagnostic session
		assert.NotNil(t, session.History[0].SystemSnapshot)
		assert.Equal(t, 45.5, session.History[0].SystemSnapshot.CPUUsage)
		assert.Equal(t, int64(16*1024*1024*1024), session.History[0].SystemSnapshot.MemoryTotal)
	})

	t.Run("InvalidAgentID", func(t *testing.T) {
		diagnosticReq := `{
			"agent_id": "invalid-id",
			"issue": "High CPU usage"
		}`

		req, err := http.NewRequest("POST", "/api/diagnostic", strings.NewReader(diagnosticReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)
		req.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid agent ID format")
	})

	t.Run("NonExistentAgent", func(t *testing.T) {
		diagnosticReq := fmt.Sprintf(`{
			"agent_id": "%s",
			"issue": "High CPU usage"
		}`, bson.NewObjectID().Hex())

		req, err := http.NewRequest("POST", "/api/diagnostic", strings.NewReader(diagnosticReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)
		req.Header.Set("Content-Type", "application/json") // Add Content-Type header

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "agent not found")
	})

	t.Run("MissingRequiredFields", func(t *testing.T) {
		// Create request with missing required fields
		diagnosticReq := `{
			"agent_id": "507f1f77bcf86cd799439011"
		}`

		req, err := http.NewRequest("POST", "/api/diagnostic", strings.NewReader(diagnosticReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)
		req.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Agent ID and Issue are required")
	})

	t.Run("UnauthorizedRequest", func(t *testing.T) {
		diagnosticReq := `{
			"agent_id": "507f1f77bcf86cd799439011",
			"issue": "High CPU usage"
		}`

		req, err := http.NewRequest("POST", "/api/diagnostic", strings.NewReader(diagnosticReq))
		// Add the Content-Type header
		req.Header.Set("Content-Type", "application/json")

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

func TestHandleContinueDiagnostic(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
			SystemMetrics: agent.SystemMetrics{
				CPUInfo:     []string{"Intel i7-1165G7"},
				CPUUsage:    45.5,
				MemoryTotal: 16 * 1024 * 1024 * 1024,
				MemoryUsed:  8 * 1024 * 1024 * 1024,
				MemoryFree:  8 * 1024 * 1024 * 1024,
				DiskUsage: map[string]int64{
					"/":     250 * 1024 * 1024 * 1024,
					"/home": 500 * 1024 * 1024 * 1024,
				},
				FSUsage: map[string]string{
					"/":     "45%",
					"/home": "60%",
				},
			},
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		// Now continue the session
		continueReq := fmt.Sprintf(`{
            "diagnostic_output": [
                "top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
                "Tasks: 180 total, 2 running, 178 sleeping"
            ],
            "system_metrics": {
                "cpu_info": ["Intel i7-1165G7"],
                "cpu_usage": 45.5,
                "memory_total": 17179869184,
                "memory_used": 8589934592,
                "memory_free": 8589934592,
                "disk_usage": {
                    "/": 268435456000,
                    "/home": 536870912000
                },
                "fs_usage": {
                    "/": "45%%",
                    "/home": "60%%"
                }
            }
        }`)

		req, err := http.NewRequest("POST", fmt.Sprintf("/api/diagnostic/%s/continue", session.ID.Hex()), strings.NewReader(continueReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)
		req.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusCreated, recorder.Code)

		var updatedSession diagnostic.DiagnosticSession
		err = json.NewDecoder(recorder.Body).Decode(&updatedSession)
		assert.NoError(t, err)
		assert.Equal(t, session.ID, updatedSession.ID)
		assert.Equal(t, 1, updatedSession.CurrentIteration)
		assert.Greater(t, len(updatedSession.History), 0)
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
			SystemMetrics: agent.SystemMetrics{
				CPUInfo:     []string{"Intel i7-1165G7"},
				CPUUsage:    45.5,
				MemoryTotal: 16 * 1024 * 1024 * 1024,
				MemoryUsed:  8 * 1024 * 1024 * 1024,
				MemoryFree:  8 * 1024 * 1024 * 1024,
				DiskUsage: map[string]int64{
					"/":     250 * 1024 * 1024 * 1024,
					"/home": 500 * 1024 * 1024 * 1024,
				},
				FSUsage: map[string]string{
					"/":     "45%",
					"/home": "60%",
				},
			},
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		_, err = server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		continueReq := fmt.Sprintf(`{
            "diagnostic_output": [
                "top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
                "Tasks: 180 total, 2 running, 178 sleeping"
            ],
            "system_metrics": {
                "cpu_info": ["Intel i7-1165G7"],
                "cpu_usage": 45.5,
                "memory_total": 17179869184,
                "memory_used": 8589934592,
                "memory_free": 8589934592,
                "disk_usage": {
                    "/": 268435456000,
                    "/home": 536870912000
                },
                "fs_usage": {
                    "/": "45%%",
                    "/home": "60%%"
                }
            }
        }`)

		req, err := http.NewRequest("POST", fmt.Sprintf("/api/diagnostic/%s/continue", bson.NewObjectID().Hex()), strings.NewReader(continueReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)
		req.Header.Set("Content-Type", "application/json")

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		continueReq := `{"results": ["test output"]}`

		req, err := http.NewRequest("POST", "/api/diagnostic/invalid-id/continue", strings.NewReader(continueReq))
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid session ID format")
	})
}

func TestHandleGetDiagnosticSession(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/diagnostic/%s", session.ID.Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var fetchedSession diagnostic.DiagnosticSession
		err = json.NewDecoder(recorder.Body).Decode(&fetchedSession)
		assert.NoError(t, err)
		assert.Equal(t, session.ID, fetchedSession.ID)
		assert.Equal(t, session.AgentID, fetchedSession.AgentID)
		assert.Equal(t, session.InitialIssue, fetchedSession.InitialIssue)
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/diagnostic/%s", bson.NewObjectID().Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/diagnostic/invalid-id", nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid session ID format\n")
	})
}

func TestHandleDeleteDiagnostic(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		req, err := http.NewRequest("DELETE", fmt.Sprintf("/api/diagnostic/%s", session.ID.Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		// Verify session was deleted
		_, err = server.diagnosticService.GetDiagnosticSession(context.Background(), session.ID.Hex())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", fmt.Sprintf("/api/diagnostic/%s", bson.NewObjectID().Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", "/api/diagnostic/invalid-id", nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "Invalid session ID format\n")
	})
}

func TestHandleListDiagnostics(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		req, err := http.NewRequest("GET", "/api/diagnostics", nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var sessions []*diagnostic.DiagnosticSession
		err = json.NewDecoder(recorder.Body).Decode(&sessions)
		assert.NoError(t, err)
		assert.NotEmpty(t, sessions)
		assert.Equal(t, session.ID, sessions[0].ID)
		assert.Equal(t, session.AgentID, sessions[0].AgentID)
	})

	t.Run("UnauthorizedRequest", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/diagnostics", nil)
		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

func TestHandleGetDiagnosticSummary(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// First create a diagnostic session
		agentInfo := &agent.AgentInfo{
			UserID:        validToken.UserID,
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := server.agentInfoService.SaveAgentInfo(context.Background(), *agentInfo)
		assert.NoError(t, err)

		agentID := insertResult.InsertedID.(bson.ObjectID).Hex()
		session, err := server.diagnosticService.StartDiagnosticSession(
			context.Background(),
			agentID,
			validToken.UserID,
			"High CPU usage",
		)
		assert.NoError(t, err)

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/diagnostic/%s/summary", session.ID.Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]string
		err = json.NewDecoder(recorder.Body).Decode(&response)
		assert.NoError(t, err)
		assert.Contains(t, response["summary"], "High CPU usage")
		assert.Contains(t, response["summary"], "Diagnostic Summary")
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/diagnostic/%s/summary", bson.NewObjectID().Hex()), nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		req, err := http.NewRequest("GET", "/api/diagnostic/invalid-id/summary", nil)
		assert.NoError(t, err)
		req.Header.Set("X-NANNYAPI-Key", validToken.Token)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "invalid session ID format")
	})
}
