package server

import (
	"bytes"
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

	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/chat"
	"github.com/harshavmb/nannyapi/internal/token"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db"
	testCollectionName = "servers"
	jwtSecret          = "d2a8b6aad8fb7d736508a520e2d53460054d21b14c1a8be86ec61e654ee807e6d47e167628bdeb59d7da25ac4de4ab1cbc161b2a335924b89e22fdac3bc44511e9fa896031b3154fd7365fe01c539ef5681ba70a65619eae8c7c14b832ea989d779d828a4e95e63181ae70ad0d855a40477144cc892097e0b0c0abfd5a26ce5f8bc0159bf44171a6dcd295aa810c4759ae0a0bc0f13b9f5872fd048ab9daa94c64d5e999dc7ea928f5a87731b468c25f2a67a6180f8f99bd9d38c706f9ca77f74e0929b5abec65c3b26d641f57a6c683a0770880748ebc5804ada5179a0252228b1a328898cae4a0d987767889251eda344cb45fd4725099de8f0947328a6166" //just for testing not used anywhere
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

func setupServer(t *testing.T) (*Server, func(), string, string) {
	// Mock Gemini Client
	mockGeminiClient := &api.GeminiClient{}

	// Mock GitHub Auth
	mockGitHubAuth := &auth.GitHubAuth{}

	// Connect to test database
	client, cleanup := setupTestDB(t)
	//defer cleanup()

	// Create a new Repository objects
	userRepository := user.NewUserRepository(client.Database(testDBName))
	tokenRepository := token.NewTokenRepository(client.Database(testDBName))
	refreshTokenRepository := token.NewRefreshTokenRepository(client.Database(testDBName))
	agentInfoRepository := agent.NewAgentInfoRepository(client.Database(testDBName))
	ChatRepository := chat.NewChatRepository(client.Database(testDBName))

	// Mock Services
	mockUserService := user.NewUserService(userRepository)
	agentInfoservice := agent.NewAgentInfoService(agentInfoRepository)
	chatService := chat.NewChatService(ChatRepository, agentInfoservice)
	mockTokenService := token.NewTokenService(tokenRepository)
	mockRefreshTokenService := token.NewRefreshTokenService(refreshTokenRepository)

	// Create a new server instance
	server := NewServer(mockGeminiClient, mockGitHubAuth, mockUserService, agentInfoservice, chatService, mockTokenService, mockRefreshTokenService, jwtSecret, encryptionKey)

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

	staticToken := token.Token{
		UserID: token.GenerateRandomString(6),
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

	return server, cleanup, staticToken.Token, accessToken
}

func TestHandleDeleteAuthToken_NoAuth(t *testing.T) {
	server, cleanup, _, _ := setupServer(t)
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
	req.Header.Set("X-NANNYAPI-Key", apiKey)

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

func TestHandleGetAgentInfoByID(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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
		req.Header.Set("X-NANNYAPI-Key", validToken)

		// Create a test recorder
		recorder := httptest.NewRecorder()

		// Serve the request
		server.ServeHTTP(recorder, req)

		// Check the response status code
		if recorder.Code != http.StatusOK {
			t.Errorf("Expected status code %d, but got %d", http.StatusOK, recorder.Code)
		}

		// Check the response body
		expected := fmt.Sprintf(`{"id":"%s","user_id":"123456","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"`, agentInfoID) // Partial match
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
		req.Header.Set("X-NANNYAPI-Key", validToken)

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

func TestHandleStartChat(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		chat := fmt.Sprintf(`{"agent_id":"%s"}`, agentInfoID)
		req, err := http.NewRequest("POST", "/api/chat", strings.NewReader(chat))

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusCreated, recorder.Code)

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

	t.Run("InvalidAgentID", func(t *testing.T) {
		chat := fmt.Sprintf(`{"agent_id":"%s"}`, "agent1")
		req, err := http.NewRequest("POST", "/api/chat", strings.NewReader(chat))

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		// Check the response body
		expected := "Invalid agent_id passed"
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) { // partial match
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})

	t.Run("NonExistentAgent", func(t *testing.T) {
		chat := fmt.Sprintf(`{"agent_id":"%s"}`, bson.NewObjectID().Hex())
		req, err := http.NewRequest("POST", "/api/chat", strings.NewReader(chat))

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		// Check the response body
		expected := "agent_id doesn't exist"
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) { // partial match
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})

	t.Run("InvalidRequestPayload", func(t *testing.T) {
		requestBody := `{"invalid_field":"value"}`
		req, err := http.NewRequest("POST", "/api/chat", bytes.NewBufferString(requestBody))

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestChatService_AddPromptResponse(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequestText", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		// Insert a chat to update
		initialChat := &chat.Chat{
			AgentID: agentInfoID,
			History: generateHistory(
				[]string{"Initial prompt"},
				[]string{"Initial response"},
				[]string{"text"},
			),
		}
		intialChatResult, err := server.chatService.StartChat(context.Background(), initialChat)
		assert.NoError(t, err)

		chatID := intialChatResult.InsertedID.(bson.ObjectID).Hex()

		// Update the chat with a new prompt-response pair
		reqBody := `{"prompt":"Hello","response":"Hi there!","type":"text"}`
		req, err := http.NewRequest("PUT", fmt.Sprintf("/api/chat/%s", chatID), strings.NewReader(reqBody))
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedChat chat.Chat
		err = json.NewDecoder(recorder.Body).Decode(&updatedChat)
		assert.NoError(t, err)
		assert.Len(t, updatedChat.History, 2)
		assert.Equal(t, "Initial prompt", updatedChat.History[0].Prompt)
		//assert.Equal(t, "Initial response", updatedChat.History[0].Response) ## won't work as response is randomized now
		assert.Equal(t, "Hello", updatedChat.History[1].Prompt)
		//assert.Equal(t, "Hi there!", updatedChat.History[1].Response) ## won't work as response is randomized now
		assert.Equal(t, "text", updatedChat.History[1].Type)
	})

	t.Run("ValidRequestCommand", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		// Insert a chat to update
		initialChat := &chat.Chat{
			AgentID: agentInfoID,
			History: generateHistory(
				[]string{"perform health checks"},
				[]string{""},
				[]string{"commands"},
			),
		}
		intialChatResult, err := server.chatService.StartChat(context.Background(), initialChat)
		assert.NoError(t, err)

		chatID := intialChatResult.InsertedID.(bson.ObjectID).Hex()

		// Update the chat with a new prompt-response pair
		reqBody := `{"prompt":"11:42:27 up 36 days,  2:48,  3 users,  load average: 0.02, 0.03, 0.00","response":"","type":"text"}`
		req, err := http.NewRequest("PUT", fmt.Sprintf("/api/chat/%s", chatID), strings.NewReader(reqBody))
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedChat chat.Chat
		err = json.NewDecoder(recorder.Body).Decode(&updatedChat)
		assert.NoError(t, err)
		assert.Len(t, updatedChat.History, 2)
		assert.Equal(t, "perform health checks", updatedChat.History[0].Prompt)
		assert.Contains(t, updatedChat.History[0].Response, "uptime") // partial match to check uptime is in response
		assert.Equal(t, "commands", updatedChat.History[0].Type)
		assert.Contains(t, updatedChat.History[1].Prompt, "load average") // partial match to check load average is in prompt
		assert.Equal(t, "text", updatedChat.History[1].Type)
	})

	t.Run("InValidRequestPayload", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		// Insert a chat to update
		initialChat := &chat.Chat{
			AgentID: agentInfoID,
			History: generateHistory(
				[]string{"Initial prompt"},
				[]string{"Initial response"},
				[]string{"text"},
			),
		}
		intialChatResult, err := server.chatService.StartChat(context.Background(), initialChat)
		assert.NoError(t, err)

		chatID := intialChatResult.InsertedID.(bson.ObjectID).Hex()

		// Update the chat with a new prompt-response pair
		reqBody := `{"response":"Hi there!"}`
		req, err := http.NewRequest("PUT", fmt.Sprintf("/api/chat/%s", chatID), strings.NewReader(reqBody))
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("NonExistentChat", func(t *testing.T) {
		// Update the chat with a new prompt-response pair
		reqBody := `{"prompt":"Hello","response":"Hi there!","type":"text"}`
		req, err := http.NewRequest("PUT", fmt.Sprintf("/api/chat/%s", bson.NewObjectID().Hex()), strings.NewReader(reqBody))
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestChatService_GetChatByID(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		// Insert a chat to update
		initialChat := &chat.Chat{
			AgentID: agentInfoID,
			History: generateHistory(
				[]string{"Initial prompt"},
				[]string{"Initial response"},
				[]string{"text"},
			),
		}
		intialChatResult, err := server.chatService.StartChat(context.Background(), initialChat)
		assert.NoError(t, err)

		chatID := intialChatResult.InsertedID.(bson.ObjectID).Hex()
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/chat/%s", chatID), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var chat chat.Chat
		err = json.NewDecoder(recorder.Body).Decode(&chat)
		assert.NoError(t, err)
		assert.NotNil(t, chat)
		assert.Equal(t, agentInfoID, chat.AgentID)
		assert.Equal(t, chatID, chat.ID.Hex())
	})

	t.Run("NonExistentChat", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        "123456",
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

		// Insert a chat to update
		initialChat := &chat.Chat{
			AgentID: agentInfoID,
			History: generateHistory(
				[]string{"Initial prompt"},
				[]string{"Initial response"},
				[]string{"text"},
			),
		}
		_, err = server.chatService.StartChat(context.Background(), initialChat)
		assert.NoError(t, err)

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/chat/%s", bson.NewObjectID().Hex()), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("NoIDPassed", func(t *testing.T) {

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/chat/%s", ""), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
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

	// Get the userID
	// TO-DO until all the relationships between collections are sorted
	claims, err := server.tokenService.GetTokenByHashedToken(context.Background(), token.HashToken(validToken))
	if err != nil {
		log.Fatalf("error while fetching claims: %v", err)
	}

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			UserID:        claims.UserID,
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

		// Create a test request to retrieve agents
		req, err := http.NewRequest("GET", "/api/agents", nil)
		if err != nil {
			t.Fatalf("Could not create request: %v", err)
		}

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		// Check the response body
		expected := fmt.Sprintf(`[{"id":"%s","user_id":"%s","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"`, agentInfoID, claims.UserID) // Partial match
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) {
			t.Errorf("Expected body to contain %q, but got %q", expected, actual)
		}
	})

}

func TestHandleFetchUserInfoFromID(t *testing.T) {
	server, cleanup, validToken, _ := setupServer(t)
	defer cleanup()

	t.Run("ValidEmail", func(t *testing.T) {
		// Get the User via email
		user, err := server.userService.GetUserByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)

		// Get the user via ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/user/%s", user.ID.Hex()), nil)
		assert.NoError(t, err)

		// Set a valid X-NANNYAPI-Key header
		req.Header.Set("X-NANNYAPI-Key", validToken)

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
		req.Header.Set("X-NANNYAPI-Key", validToken)

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
