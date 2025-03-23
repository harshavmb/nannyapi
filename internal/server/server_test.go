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
	server := NewServer(mockGeminiClient, mockGitHubAuth, mockUserService, agentInfoservice, chatService, mockTokenService, mockRefreshTokenService, "")

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
	tokenString := token.Token{} // just to supress local builds, will be removed
	authToken, err := mockTokenService.CreateToken(context.Background(), tokenString, encryptionKey)
	if err != nil {
		t.Fatalf("Failed to create auth token: %v", err)
	}

	decryptedToken, err := token.Decrypt(authToken.Token, encryptionKey)
	if err != nil {
		log.Fatalf("Failed to decrypt token: %v", err)
	}

	return server, cleanup, decryptedToken
}

func generateHistory(prompts, responses, types []string) []chat.PromptResponse {
	history := make([]chat.PromptResponse, len(prompts))
	for i := range prompts {
		history[i] = chat.PromptResponse{
			Prompt:   prompts[i],
			Response: responses[i],
			Type:     types[i],
		}
	}
	return history
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
		agentInfo := `{"hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0","os_version":"Ubuntu 24.04"}`
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
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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
		expected := fmt.Sprintf(`{"id":"%s","email":"test@example.com","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"`, agentInfoID) // Partial match
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
		expected := "Agent ID is required"
		actual := strings.TrimSpace(recorder.Body.String())
		if actual != expected {
			t.Errorf("Expected body %q, but got %q", expected, actual)
		}
	})
}

func TestHandleStartChat(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		assert.NoError(t, err)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestChatService_AddPromptResponse(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequestText", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var updatedChat chat.Chat
		err = json.NewDecoder(recorder.Body).Decode(&updatedChat)
		assert.NoError(t, err)
		assert.Len(t, updatedChat.History, 2)
		assert.Equal(t, "Initial prompt", updatedChat.History[0].Prompt)
		assert.Equal(t, "Initial response", updatedChat.History[0].Response)
		assert.Equal(t, "Hello", updatedChat.History[1].Prompt)
		assert.Equal(t, "Hi there!", updatedChat.History[1].Response)
		assert.Equal(t, "text", updatedChat.History[1].Type)
	})

	t.Run("ValidRequestCommand", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})

	t.Run("NonExistentChat", func(t *testing.T) {
		// Update the chat with a new prompt-response pair
		reqBody := `{"prompt":"Hello","response":"Hi there!","type":"text"}`
		req, err := http.NewRequest("PUT", fmt.Sprintf("/api/chat/%s", bson.NewObjectID().Hex()), strings.NewReader(reqBody))
		assert.NoError(t, err)

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})
}

func TestChatService_GetChatByID(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
	})

	t.Run("NoIDPassed", func(t *testing.T) {

		req, err := http.NewRequest("GET", fmt.Sprintf("/api/chat/%s", ""), nil)
		assert.NoError(t, err)

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestNannyAPIPortOverride(t *testing.T) {
	// Set the environment variable
	os.Setenv("NANNY_API_PORT", "9090")
	defer os.Unsetenv("NANNYAPI_PORT")

	server, cleanup, _ := setupServer(t)
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

	server, cleanup, _ := setupServer(t)
	defer cleanup()

	// Check if the server is running on the correct github callback url
	assert.Equal(t, os.Getenv("NANNY_SWAGGER_URL"), server.nannySwaggerURL)
}

func TestGitHubRedirectURL(t *testing.T) {
	// Set the environment variable
	os.Setenv("GH_REDIRECT_URL", "http://example.net/swagger/doc.json")
	defer os.Unsetenv("GH_REDIRECT_URL")

	server, cleanup, _ := setupServer(t)
	defer cleanup()

	// Check if the server is running on the correct github callback url
	assert.Equal(t, os.Getenv("GH_REDIRECT_URL"), server.gitHubRedirectURL)
}

func TestHandleAgentInfos(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidRequest", func(t *testing.T) {
		// Insert test agent info into the database
		agentInfo := &agent.AgentInfo{
			Email:         "test@example.com",
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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		// Check the response body
		expected := fmt.Sprintf(`[{"id":"%s","email":"test@example.com","hostname":"test-host","ip_address":"192.168.1.1","kernel_version":"5.10.0"`, agentInfoID) // Partial match
		actual := strings.TrimSpace(recorder.Body.String())
		if !strings.Contains(actual, expected) {
			t.Errorf("Expected body to contain %q, but got %q", expected, actual)
		}
	})

}

func TestHandleFetchUserInfoFromEmail(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidEmail", func(t *testing.T) {
		// Create a request with a valid email
		req, err := http.NewRequest("GET", "/api/user/test@example.com", nil)
		assert.NoError(t, err)

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

	// Uncomment this test once the email validation is implemented
	// t.Run("InvalidEmailFormat", func(t *testing.T) {
	// 	// Create a request with an invalid email format
	// 	req, err := http.NewRequest("GET", "/api/user/invalid-email", nil)
	// 	assert.NoError(t, err)

	// 	// Set a valid Authorization header
	// 	req.Header.Set("Authorization", "Bearer "+validToken)

	// 	recorder := httptest.NewRecorder()
	// 	server.ServeHTTP(recorder, req)

	// 	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	// 	expected := `{"error":"Invalid email format"}`
	// 	actual := strings.TrimSpace(recorder.Body.String())
	// 	assert.Equal(t, expected, actual)
	// })

	t.Run("UserNotFound", func(t *testing.T) {
		// Create a request with an email that does not exist in the database
		req, err := http.NewRequest("GET", "/api/user/nonexistent@example.com", nil)
		assert.NoError(t, err)

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusNotFound, recorder.Code)
		expected := `{"error":"User not found"}`
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})

	t.Run("UnauthorizedRequest", func(t *testing.T) {
		// Create a request with a valid email
		req, err := http.NewRequest("GET", "/api/user/test@example.com", nil)
		assert.NoError(t, err)

		// Do not set an Authorization header
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)

		expected := `Authorization header is required`
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})
}

func TestHandleFetchUserInfoFromID(t *testing.T) {
	server, cleanup, validToken := setupServer(t)
	defer cleanup()

	t.Run("ValidEmail", func(t *testing.T) {
		// Get the User via email
		user, err := server.userService.GetUserByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)

		// Get the user via ID
		req, err := http.NewRequest("GET", fmt.Sprintf("/api/user/%s", user.ID.Hex()), nil)
		assert.NoError(t, err)

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

		// Set a valid Authorization header
		req.Header.Set("Authorization", "Bearer "+validToken)

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

		expected := `Authorization header is required`
		actual := strings.TrimSpace(recorder.Body.String())
		assert.Equal(t, expected, actual)
	})
}
