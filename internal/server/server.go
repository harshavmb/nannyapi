package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"encoding/json"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/chat"
	"github.com/harshavmb/nannyapi/internal/diagnostic"
	"github.com/harshavmb/nannyapi/internal/token"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	"github.com/rs/cors"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Server represents the HTTP server
type Server struct {
	mux                 *http.ServeMux
	geminiClient        *api.GeminiClient
	githubAuth          *auth.GitHubAuth
	userService         *user.UserService
	agentInfoService    *agent.AgentInfoService
	chatService         *chat.ChatService
	tokenService        *token.TokenService
	refreshTokenservice *token.RefreshTokenService
	diagnosticService   *diagnostic.DiagnosticService
	nannyAPIPort        string
	nannySwaggerURL     string
	gitHubRedirectURL   string
	jwtSecret           string
	nannyEncryptionKey  string
}

// StartDiagnosticRequest represents a request to start a diagnostic session
type StartDiagnosticRequest struct {
	ChatID     string            `json:"chat_id"`
	Issue      string            `json:"issue"`
	SystemInfo map[string]string `json:"system_info"`
}

// ContinueDiagnosticRequest represents a request to continue a diagnostic session
type ContinueDiagnosticRequest struct {
	Results []string `json:"results"`
}

// startChat starts a chat session with the model using the given history.
func (s *Server) startChat(hist []content) *genai.ChatSession {
	model := s.geminiClient.Model()
	cs := model.StartChat()
	cs.History = transform(hist)
	return cs
}

// NewServer creates a new Server instance
func NewServer(geminiClient *api.GeminiClient, githubAuth *auth.GitHubAuth, userService *user.UserService, agentInfoService *agent.AgentInfoService, chatService *chat.ChatService, tokenService *token.TokenService, refreshTokenService *token.RefreshTokenService, diagnosticService *diagnostic.DiagnosticService, jwtSecret, nannyEncryptionKey string) *Server {
	mux := http.NewServeMux()

	// override default nanny API port if NANNY_API_PORT is set
	nannyAPIPort := os.Getenv("NANNY_API_PORT")
	if nannyAPIPort == "" {
		nannyAPIPort = "8080" // Default port
	}

	// override default nanny Swagger URL if NANNY_SWAGGER_URL is set
	nannySwaggerURL := os.Getenv("NANNY_SWAGGER_URL")
	if nannySwaggerURL == "" {
		nannySwaggerURL = fmt.Sprintf("http://localhost:%s/swagger/doc.json", nannyAPIPort) // Default Swagger URL
	}

	// Fix-me
	// this is duplicate I know, not sure how to do it elegantly
	gitHubRedirectURL := os.Getenv("GH_REDIRECT_URL")
	if gitHubRedirectURL == "" {
		gitHubRedirectURL = fmt.Sprintf("http://localhost:%s/github/callback", nannyAPIPort) // Default GitHubCallback URL
	}

	server := &Server{mux: mux, geminiClient: geminiClient, githubAuth: githubAuth, userService: userService, agentInfoService: agentInfoService, chatService: chatService, tokenService: tokenService, refreshTokenservice: refreshTokenService, diagnosticService: diagnosticService, nannyAPIPort: nannyAPIPort, nannySwaggerURL: nannySwaggerURL, gitHubRedirectURL: gitHubRedirectURL, jwtSecret: jwtSecret, nannyEncryptionKey: nannyEncryptionKey}
	server.routes()
	return server
}

// routes defines the routes for the server
func (s *Server) routes() {

	s.mux.HandleFunc("POST /chat", s.chatHandler)
	s.mux.HandleFunc("/status", s.handleStatus())
	s.mux.HandleFunc("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL(s.nannySwaggerURL),
	))

	// GitHub Auth Endpoints
	s.mux.HandleFunc("/github/login", s.githubAuth.HandleGitHubLogin())
	s.mux.HandleFunc("/github/callback", s.githubAuth.HandleGitHubCallback())
	s.mux.HandleFunc("/github/profile", s.githubAuth.HandleGitHubProfile())

	// Token Endpoints
	s.mux.HandleFunc("POST /api/refresh-token", s.handleRefreshToken())

	// API endoints with token authentication
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("POST /api/auth-token", s.handleCreateAuthToken())
	apiMux.HandleFunc("/api/auth-tokens", s.handleGetAuthTokens())
	apiMux.Handle("/api/user-auth-token", s.handleFetchUserInfoFromToken())
	apiMux.Handle("GET /api/user/{param}", s.handleFetchUserInfo())
	apiMux.Handle("DELETE /api/auth-token/{id}", s.handleDeleteAuthToken())
	apiMux.HandleFunc("POST /api/agent-info", s.handleAgentInfo())
	apiMux.HandleFunc("GET /api/agent-info/", s.handleGetAgentInfoByID())
	apiMux.HandleFunc("GET /api/agent-info/{id}", s.handleGetAgentInfoByID())
	apiMux.HandleFunc("GET /api/agents", s.handleAgentInfos())
	apiMux.HandleFunc("POST /api/chat", s.handleStartChat())
	apiMux.HandleFunc("PUT /api/chat/{id}", s.handleAddPromptResponse())
	apiMux.HandleFunc("GET /api/chat/", s.handleGetChatByID())
	apiMux.HandleFunc("GET /api/chat/{id}", s.handleGetChatByID())

	// Diagnostic Endpoints
	apiMux.HandleFunc("POST /api/diagnostic", s.handleStartDiagnostic())
	apiMux.HandleFunc("POST /api/diagnostic/{id}/continue", s.handleContinueDiagnostic())
	apiMux.HandleFunc("GET /api/diagnostic/{id}", s.handleGetDiagnostic())
	apiMux.HandleFunc("GET /api/diagnostic/{id}/summary", s.handleGetDiagnosticSummary())
	apiMux.HandleFunc("DELETE /api/diagnostic/{id}", s.handleDeleteDiagnostic())
	apiMux.HandleFunc("GET /api/diagnostics", s.handleListDiagnostics())

	// Create a new CORS handler
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:8081", "https://nannyai.dev", "https://nannyui.pages.dev"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	// Create a CORS middleware that applies to specific paths
	corsMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the request path matches the desired pattern
			if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/github/login" || r.URL.Path == "/github/callback" || r.URL.Path == "/github/profile" {
				c.Handler(next).ServeHTTP(w, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}

	// Apply the CORS middleware to the main mux
	s.mux.Handle("/index", corsMiddleware(s.mux))

	// Wrap the API mux with the CORS handler
	s.mux.Handle("/api/", c.Handler(s.AuthMiddleware(apiMux)))
	//s.mux.Handle("/", c.Handler(s.AuthMiddleware(http.HandlerFunc(s.handleIndex()))))

}

// HandleRefreshToken handles refresh token requests
// @Summary Handle refresh token validation, creation and creation of accessTokens too
// @Description Handle refresh token validation, creation and creation of accessTokens too
// @Tags refresh-token
// @Accept json
// @Produce json
// @Param refreshToken body string true "Refresh Token"
// @Success 200 {object} map[string]string "refreshToken and accessToken"
// @Failure 400 {string} string "Invalid request payload"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to create refresh token"
// @Router /api/refresh-token [post]
func (s *Server) handleRefreshToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Retrieve refresh token from cookies
		cookie, err := r.Cookie("refresh_token")
		if err != nil {
			http.Error(w, "Refresh token is required", http.StatusBadRequest)
			return
		}
		refreshToken := cookie.Value

		// Validate the refresh token
		var tokenExpired bool
		_, claims, err := s.validateRefreshToken(context.Background(), refreshToken, s.jwtSecret)
		if err != nil {
			// check for revoked or expired tokens
			if err.Error() == "refresh token expired" || err.Error() == "refresh token revoked" {
				tokenExpired = true
			} else {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		}

		// final refresh token
		var finalRefreshToken string

		if tokenExpired && claims != nil {
			// Generate a new refresh token
			finalRefreshToken, err = generateRefreshToken(claims.UserID, s.jwtSecret)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			ipAddress := strings.Split(r.RemoteAddr, ":")

			// Save the new refresh token
			refreshTokenData := &token.RefreshToken{
				Token:     finalRefreshToken,
				UserID:    claims.UserID,
				UserAgent: r.UserAgent(),
				IPAddress: ipAddress[0],
			}
			_, err = s.refreshTokenservice.CreateRefreshToken(context.Background(), *refreshTokenData, s.nannyEncryptionKey)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// just use the same refreshToken
		if !tokenExpired && claims != nil {
			finalRefreshToken = refreshToken
		}

		// Generate the new access token
		accessToken, err := generateAccessToken(claims.UserID, s.jwtSecret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Prepare response
		response := map[string]string{
			"access_token":  accessToken,
			"refresh_token": finalRefreshToken,
		}

		log.Printf("Refresh and access tokens are created for user %s", claims.UserID)

		// Write response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

// handleFetchUserInfo handles the fetching of user info from the id
// the request with the following format:
// Sends a JSOn payload containing the user info to the client with the following format.
// Response:
//   - email: string
//   - name: string
//   - avatar: string
//
// handleFetchUserInfo godoc
//
//	@Summary		Fetch user info from id
//	@Description	Fetch user info from id
//	@Tags			user-from-email
//
// @Param param path string true "ID of the user"
//
//	@Produce		json
//	@Success		200		{object}	[]string
//	@Failure		400		{string}    "Bad Request"
//	@Failure		404		{string}    "Not Found"
//	@Failure		500		{string}    "Internal Server Error"
//	@Router			/api/user/{param} [get]
func (s *Server) handleFetchUserInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var user *user.User
		var err error

		// Assume it's an ID and fetch user by ID
		// Extract user ID from the URL path
		id := r.PathValue("param")
		if id == "" {
			http.Error(w, "Chat ID is required", http.StatusBadRequest)
			return
		}
		userID, err := bson.ObjectIDFromHex(id)
		if err != nil {
			http.Error(w, "Invalid user ID format", http.StatusBadRequest)
			return
		}
		user, err = s.userService.GetUserByID(r.Context(), userID)

		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
				return
			}
			log.Printf("Failed to retrieve user info: %v", err)
			http.Error(w, "Failed to retrieve user info", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
			return
		}

		log.Printf("User info retreived for: %s", user.Email)

		json.NewEncoder(w).Encode(user)
	}
}

// handleFetchUserInfoFromToken handles the fetching of user info from the auth token
// the request with the following format:
// Sends a JSOn payload containing the user info to the client with the following format.
// Response:
//   - email: string
//   - name: string
//   - avatar: string
//
// handleFetchUserInfoFromToken godoc
//
//	@Summary		Fetch user info from auth token
//	@Description	Fetch user info from auth token
//	@Tags			auth-tokens
//	@Produce		json
//	@Success		200		{object}	[]string
//	@Failure		400		{string}    "Bad Request"
//	@Failure		404		{string}    "Not Found"
//	@Failure		500		{string}    "Internal Server Error"
//	@Router			/api/user-auth-token [get]
func (s *Server) handleFetchUserInfoFromToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, ok := GetUserFromContext(r)
		if !ok {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		json.NewEncoder(w).Encode(userID)
	}
}

// chatHandler returns the complete response of the model to the client. Expects a JSON payload in
// the request with the following format:
// Request:
//   - chat: string
//   - history: []
//
// Sends a JSON payload containing the model response to the client with the following format.
// Response:
//   - text: string
//
// chatHandler godoc
//
//	@Summary		Chat with the model
//	@Description	Chat with the model
//	@Tags			chat
//	@Accept			json
//	@Produce		json
//	@Param			chat	body		chatRequest	true	"Chat request"
//	@Success		200		{object}	[]string
//	@Failure		400		{string}    "Bad Request"
//	@Failure		404		{string}    "Not Found"
//	@Failure		500		{string}    "Internal Server Error"
//	@Router			/chat [post]
func (s *Server) chatHandler(w http.ResponseWriter, r *http.Request) {
	cr := &chatRequest{}
	if err := parseRequestJSON(r, cr); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	cs := s.startChat(cr.History)

	if len(cr.History) == 0 {
		// 1. Initial Prompt (Request from User/Agent):
		initialPrompt := fmt.Sprintf("Run a list investigative Linux commands to diagnose %s on a server. If binaries are from sysstat, collect metrics for 5 seconds every 1 sec interval (only if required by the input prompt)", cr.Chat)
		res, err := cs.SendMessage(s.geminiClient.Ctx, genai.Text(initialPrompt))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return // Return early to avoid sending a response
		}

		// ... (Extract commands from Gemini's response as before) ...
		commands, err := extractCommands(res) // Helper function to extract commands
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 2. Send commands to agent (and return to the agent)
		sendCommandsToAgent(w, commands) // Helper to send commands to agent
	} else {
		// 3. Agent's Response (Commands Output):
		//agentResponse := getAgentResponse(r) // Helper to read JSON from agent

		// 4. Construct a *new* prompt for Gemini *including* the agent's output
		feedbackPrompt := fmt.Sprintf("Here are the results of the commands I ran: %s", cr.Chat)

		// 5. Send feedback prompt to Gemini
		feedbackRes, err := cs.SendMessage(s.geminiClient.Ctx, genai.Text(feedbackPrompt))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 6. Process Gemini's *feedback* response and send it back to the client
		geminiFeedback := extractGeminiFeedback(feedbackRes) // Helper function
		sendGeminiFeedbackToClient(w, geminiFeedback)        // Helper to send the feedback
	}

}

// handleIndex handles the index and status routes
//
// chatHandler godoc
//
//	@Summary		Status of the API
//	@Description	Status of the API
//	@Tags			status
//	@Accept			json
//	@Produce		json
//	@Success		200		{object}	[]string
//	@Failure		404		{string}    "Not Found"
//	@Failure		500		{string}    "Internal Server Error"
//	@Router			/status [get]
func (s *Server) handleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// ServeHTTP implements the http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleCreateAuthToken creates auth token (aka API key) for the authenticated user.
// @Summary Creates auth token (aka API key) for the authenticated user
// @Description Creates auth token (aka API key) for the authenticated user.
// @Tags auth-token
// @Produce json
// @Success 201 {object} map[string]string "id of the inserted token"
// @Failure 400 {string} string "Invalid request payload"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to create API key"
// @Router /api/auth-token [post]
func (s *Server) handleCreateAuthToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, ok := GetUserFromContext(r)
		if !ok {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// set token.Token model
		var authToken token.Token
		tokenString := token.GenerateRandomString(33) // 33 characters as it was before

		authToken.UserID = userID
		authToken.Token = tokenString

		// Create API key for the user
		responseToken, err := s.tokenService.CreateToken(r.Context(), authToken, s.nannyEncryptionKey)
		if err != nil || responseToken == nil {
			log.Printf("Failed to create API key: %v", err)
			http.Error(w, "Failed to create API key", http.StatusInternalServerError)
			return
		}

		// Decrypt the token before sent to client
		decryptedToken, err := token.Decrypt(responseToken.Token, s.nannyEncryptionKey)
		if err != nil {
			log.Printf("Failed to decrypt Token: %v", err)
			http.Error(w, "Failed to create API key", http.StatusInternalServerError)
			return
		}
		responseToken.Token = decryptedToken

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(responseToken)
	}
}

// handleGetAuthTokens retrieves all auth tokens for the authenticated user.
// @Summary Get all auth tokens
// @Description Retrieves all auth tokens for the authenticated user.
// @Tags auth-tokens
// @Produce json
// @Success 200 {array} token.Token "Successfully retrieved auth tokens"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to retrieve auth tokens"
// @Router /api/auth-tokens [get]
func (s *Server) handleGetAuthTokens() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, ok := GetUserFromContext(r)
		if !ok {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Retrieve all auth tokens for the user
		authTokens, err := s.tokenService.GetAllTokens(r.Context(), userID)
		if err != nil {
			log.Printf("Failed to retrieve auth tokens: %v", err)
			http.Error(w, "Failed to retrieve auth tokens", http.StatusInternalServerError)
			return
		}

		// decrypt all token keys
		var unencryptedTokens []*token.Token
		for _, innerToken := range authTokens {
			// Create a copy of the innerToken
			newToken := *innerToken

			// Unencrytping the token now
			newTokenStr, err := token.Decrypt(newToken.Token, s.nannyEncryptionKey)
			if err != nil {
				log.Printf("Failed to retrieve auth tokens: %v", err)
				http.Error(w, "Failed to retrieve auth tokens", http.StatusInternalServerError)
				return
			}
			newToken.Token = newTokenStr

			// Append the modified copy to the new slice
			unencryptedTokens = append(unencryptedTokens, &newToken)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(unencryptedTokens)
	}
}

// handleDeleteAuthToken deletes a specific auth token by ID.
// @Summary Delete an auth token
// @Description Deletes a specific auth token by ID.
// @Tags auth-tokens
// @Param id path string true "Token ID"
// @Produce json
// @Success 200 {object} map[string]string "Auth token deleted successfully"
// @Failure 400 {string} string "Invalid token ID format or Token ID is required"
// @Failure 500 {string} string "Failed to delete auth token"
// @Router /api/auth-token/{id} [delete]
func (s *Server) handleDeleteAuthToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Extract token ID from the URL path
		tokenID := r.PathValue("id")
		if tokenID == "" {
			http.Error(w, "Token ID is required", http.StatusBadRequest)
			return
		}

		// Convert token ID to ObjectID
		objID, err := bson.ObjectIDFromHex(tokenID)
		if err != nil {
			http.Error(w, "Invalid token ID format", http.StatusBadRequest)
			return
		}

		err = s.tokenService.DeleteToken(r.Context(), objID)
		if err != nil {
			// non-existant token passed
			if strings.Contains(err.Error(), "invalid token passed") {
				http.Error(w, "Failed to delete auth token", http.StatusNotFound)
				return
			}
			log.Printf("Failed to delete %s auth token of user %s: %v", tokenID, userID, err)
			http.Error(w, "Failed to delete auth token", http.StatusInternalServerError)
			return
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Auth token deleted successfully"})
	}
}

// handleAgentInfo handles the ingestion of agent information
// @Summary Ingest agent information
// @Description Ingest agent information
// @Tags agent-info
// @Accept json
// @Produce json
// @Param agentInfo body agent.AgentInfo true "Agent Information"
// @Success 201 {object} map[string]string "id of the inserted agent info"
// @Failure 400 {string} string "Invalid request payload"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to create agent"
// @Router /api/agent-infos [post]
func (s *Server) handleAgentInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		var agentInfo agent.AgentInfo
		if err := json.NewDecoder(r.Body).Decode(&agentInfo); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if agentInfo.Hostname == "" || agentInfo.IPAddress == "" || agentInfo.KernelVersion == "" || agentInfo.OsVersion == "" {
			http.Error(w, "All fields (hostname, ip_address, kernel_version) are required", http.StatusBadRequest)
			return
		}

		agentInfo.UserID = userID

		insertOneResult, err := s.agentInfoService.SaveAgentInfo(r.Context(), agentInfo)
		if err != nil {
			http.Error(w, "Failed to save agent info", http.StatusInternalServerError)
			return
		}

		// Return the inserted ID in the response
		response := map[string]string{
			"id": insertOneResult.InsertedID.(bson.ObjectID).Hex(),
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

// handleGetAgentInfos retrieves agents information
// @Summary Ingest agent information
// @Description Ingest agent information
// @Tags agent-info
// @Accept json
// @Produce json
// @Success 200 {object} []agent.AgentInfo "Successfully retrieved agent info"
// @Failure 400 {string} string "Invalid request payload"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to retrieve agents info"
// @Router /api/agents [get]
func (s *Server) handleAgentInfos() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		agents, err := s.agentInfoService.GetAgents(r.Context(), userID)
		if err != nil {
			log.Printf("Failed to retrieve agents info: %v", err)
			http.Error(w, "Failed to retrieve agents info", http.StatusInternalServerError)
			return
		}

		if agents == nil {
			agents = []*agent.AgentInfo{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)
	}
}

// handleGetAgentInfo retrieves agent information by id
// @Summary Get agent info by ID
// @Description Retrieves agent information by ID
// @Tags agent-info
// @Param id path string true "Agent ID"
// @Produce json
// @Success 200 {object} agent.AgentInfo "Successfully retrieved agent info"
// @Failure 400 {string} string "Invalid ID format"
// @Failure 401 {string} string "User not authenticated"
// @Failure 404 {string} string "Agent info not found"
// @Failure 500 {string} string "Failed to retrieve agent info"
// @Router /api/agent-info/{id} [get]
func (s *Server) handleGetAgentInfoByID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Extract the ID from the URL path
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Agent ID is required", http.StatusBadRequest)
			return
		}

		// Convert the ID to an ObjectID
		objectID, err := bson.ObjectIDFromHex(id)
		if err != nil {
			http.Error(w, "Invalid ID format", http.StatusBadRequest)
			return
		}

		agentInfo, err := s.agentInfoService.GetAgentInfoByID(r.Context(), objectID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "Agent info not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to retrieve agent info", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(agentInfo)
	}
}

// handleStartChat starts a new chat session
// @Summary Start a new chat session
// @Description Starts a new chat session
// @Tags chat
// @Accept json
// @Produce json
// @Param agentID body string true "Agent ID"
// @Success 201 {object} chat.Chat "Chat session started successfully"
// @Failure 500 {string} string "Failed to start chat session"
// @Router /api/chat [post]
func (s *Server) handleStartChat() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		var chat chat.Chat
		if err := json.NewDecoder(r.Body).Decode(&chat); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if chat.AgentID == "" {
			http.Error(w, "Agent ID is required", http.StatusBadRequest)
			return
		}

		// validate whether agentId exists and is in the correct format
		_, err := bson.ObjectIDFromHex(chat.AgentID)
		if err != nil {
			http.Error(w, "Invalid agent_id passed", http.StatusBadRequest)
			return
		}

		insertResult, err := s.chatService.StartChat(r.Context(), &chat)
		if err != nil {
			http.Error(w, "Failed to start chat session", http.StatusInternalServerError)
			return
		}

		if insertResult == nil {
			http.Error(w, "agent_id doesn't exist", http.StatusBadRequest)
			return
		}

		// Return the inserted ID in the response
		response := map[string]string{
			"id": insertResult.InsertedID.(bson.ObjectID).Hex(),
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(response)
	}
}

// handleAddPromptResponse adds a prompt-response pair to an existing chat session
// @Summary Add a prompt-response pair to a chat session
// @Description Adds a prompt-response pair to an existing chat session
// @Tags chat
// @Accept json
// @Produce json
// @Param chatID path string true "Chat ID"
// @Param promptResponse body chat.PromptResponse true "Prompt and Response"
// @Success 200 {object} chat.Chat "Prompt-response pair added successfully"
// @Failure 400 {string} string "Invalid request payload"
// @Failure 404 {string} string "Chat session not found"
// @Failure 500 {string} string "Failed to add prompt-response pair"
// @Router /api/chat/{id} [put]
func (s *Server) handleAddPromptResponse() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		id := r.PathValue("id")
		chatID, err := bson.ObjectIDFromHex(id)
		if err != nil {
			http.Error(w, "Invalid chat ID format", http.StatusBadRequest)
			return
		}

		var promptResponse chat.PromptResponse
		if err := json.NewDecoder(r.Body).Decode(&promptResponse); err != nil {
			http.Error(w, "Invalid request payload", http.StatusBadRequest)
			return
		}

		if promptResponse.Prompt == "" || promptResponse.Type == "" {
			http.Error(w, "Prompt and Type are required", http.StatusBadRequest)
			return
		}

		chat, err := s.chatService.AddPromptResponse(r.Context(), chatID, promptResponse)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "Chat session not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to add prompt-response pair", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(chat)
	}
}

// handleGetChatByID retrieves a chat session by ID
// @Summary Get a chat session by ID
// @Description Retrieves a chat session by ID
// @Tags chat
// @Produce json
// @Param chatID path string true "Chat ID"
// @Success 200 {object} chat.Chat "Successfully retrieved chat session"
// @Failure 400 {string} string "Invalid chat ID format"
// @Failure 404 {string} string "Chat session not found"
// @Failure 500 {string} string "Failed to retrieve chat session"
// @Router /api/chat/{id} [get]
func (s *Server) handleGetChatByID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Extract the ID from the URL path
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Chat ID is required", http.StatusBadRequest)
			return
		}

		chatID, err := bson.ObjectIDFromHex(id)
		if err != nil {
			http.Error(w, "Invalid chat ID format", http.StatusBadRequest)
			return
		}

		chat, err := s.chatService.GetChatByID(r.Context(), chatID)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				http.Error(w, "Chat session not found", http.StatusNotFound)
				return
			}
			http.Error(w, "Failed to retrieve chat session", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(chat)
	}
}

// handleStartDiagnostic starts a new diagnostic session
// @Summary Start diagnostic session
// @Description Start a new diagnostic session for an agent
// @Tags diagnostic
// @Accept json
// @Produce json
// @Param request body StartDiagnosticRequest true "Start diagnostic request"
// @Success 201 {object} diagnostic.DiagnosticSession
// @Failure 400 {string} string "Invalid request format"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostic [post]
func (s *Server) handleStartDiagnostic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user is authenticated
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "User not authenticated"})
			return
		}

		var req struct {
			AgentID string `json:"agent_id"`
			Issue   string `json:"issue"`
		}

		if err := parseRequestJSON(r, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		if req.AgentID == "" || req.Issue == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "Agent ID and Issue are required"})
			return
		}

		session, err := s.diagnosticService.StartDiagnosticSession(r.Context(), req.AgentID, userID, req.Issue)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "invalid agent ID format") {
				statusCode = http.StatusBadRequest
			} else if strings.Contains(err.Error(), "agent not found") {
				statusCode = http.StatusBadRequest
			} else if strings.Contains(err.Error(), "agent does not belong to user") {
				statusCode = http.StatusForbidden
			}
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(session)
	}
}

// handleContinueDiagnostic continues an existing diagnostic session
// @Summary Continue a diagnostic session
// @Description Continue an existing Linux system diagnostic session
// @Tags diagnostic
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param diagnostic_output body []string true "Command results"
// @Success 200 {object} diagnostic.DiagnosticSession
// @Failure 400 {string} string "Invalid request"
// @Failure 404 {string} string "Session not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostic/{id}/continue [post]
func (s *Server) handleContinueDiagnostic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, "Session ID is required", http.StatusBadRequest)
			return
		}

		// Validate session ID format
		if _, err := bson.ObjectIDFromHex(sessionID); err != nil {
			http.Error(w, "invalid session ID format", http.StatusBadRequest)
			return
		}

		var req struct {
			DiagnosticOutput []string             `json:"diagnostic_output"`
			SystemMetrics    *agent.SystemMetrics `json:"system_metrics,omitempty"`
		}

		if err := parseRequestJSON(r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		session, err := s.diagnosticService.ContinueDiagnosticSession(r.Context(), sessionID, req.DiagnosticOutput)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "invalid session ID format") {
				statusCode = http.StatusBadRequest
			} else if strings.Contains(err.Error(), "session not found") {
				statusCode = http.StatusNotFound
			}
			http.Error(w, err.Error(), statusCode)
			return
		}

		if session == nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}

		json.NewEncoder(w).Encode(session)
	}
}

// handleGetDiagnostic retrieves a diagnostic session
// @Summary Get diagnostic session
// @Description Get details of a diagnostic session
// @Tags diagnostic
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} diagnostic.DiagnosticSession
// @Failure 400 {string} string "Invalid session ID format"
// @Failure 404 {string} string "Session not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostic/{id} [get]
func (s *Server) handleGetDiagnostic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, "Session ID is required", http.StatusBadRequest)
			return
		}

		// Validate session ID format
		if _, err := bson.ObjectIDFromHex(sessionID); err != nil {
			http.Error(w, "invalid session ID format", http.StatusBadRequest)
			return
		}

		session, err := s.diagnosticService.GetDiagnosticSession(r.Context(), sessionID)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "session not found") {
				statusCode = http.StatusNotFound
			}
			http.Error(w, err.Error(), statusCode)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(session)
	}
}

// handleGetDiagnosticSummary retrieves a diagnostic session summary
// @Summary Get diagnostic summary
// @Description Get a summary of the diagnostic session
// @Tags diagnostic
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {string} string "Diagnostic summary"
// @Failure 404 {string} string "Session not found"
// @Failure 400 {string} string "Invalid session ID format"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostic/{id}/summary [get]
func (s *Server) handleGetDiagnosticSummary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, "Session ID is required", http.StatusBadRequest)
			return
		}

		// Validate session ID format
		if _, err := bson.ObjectIDFromHex(sessionID); err != nil {
			http.Error(w, "invalid session ID format", http.StatusBadRequest)
			return
		}

		summary, err := s.diagnosticService.GetDiagnosticSummary(r.Context(), sessionID)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if strings.Contains(err.Error(), "session not found") {
				statusCode = http.StatusNotFound
			}
			http.Error(w, err.Error(), statusCode)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(summary))
	}
}

// handleDeleteDiagnostic deletes a diagnostic session
// @Summary Delete a diagnostic session
// @Description Delete a diagnostic session and its associated data
// @Tags diagnostic
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {string} string "Session deleted successfully"
// @Failure 400 {string} string "Invalid session ID"
// @Failure 401 {string} string "User not authenticated"
// @Failure 403 {string} string "User not authorized"
// @Failure 404 {string} string "Session not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostic/{id} [delete]
func (s *Server) handleDeleteDiagnostic() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if user is authenticated
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		sessionID := r.PathValue("id")
		if sessionID == "" {
			http.Error(w, "Session ID is required", http.StatusBadRequest)
			return
		}

		// Validate session ID format
		if _, err := bson.ObjectIDFromHex(sessionID); err != nil {
			http.Error(w, "Invalid session ID format", http.StatusBadRequest)
			return
		}

		err := s.diagnosticService.DeleteSession(r.Context(), sessionID, userID)
		if err != nil {
			statusCode := http.StatusInternalServerError
			if err.Error() == "session not found" {
				statusCode = http.StatusNotFound
			} else if err.Error() == "user does not own this session" {
				statusCode = http.StatusForbidden
			}
			http.Error(w, err.Error(), statusCode)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Session deleted successfully"})
	}
}

// handleListDiagnostics lists diagnostic sessions for the authenticated user
// @Summary List diagnostic sessions
// @Description List all diagnostic sessions for the authenticated user
// @Tags diagnostic
// @Produce json
// @Success 200 {array} diagnostic.DiagnosticSession
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Internal server error"
// @Router /api/diagnostics [get]
func (s *Server) handleListDiagnostics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user is authenticated
		userID, _ := GetUserFromContext(r)
		if userID == "" {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		sessions, err := s.diagnosticService.ListUserSessions(r.Context(), userID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to list sessions: %v", err), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(sessions)
	}
}
