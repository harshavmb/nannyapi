package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"encoding/json"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/chat"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Server represents the HTTP server
type Server struct {
	mux               *http.ServeMux
	geminiClient      *api.GeminiClient
	githubAuth        *auth.GitHubAuth
	template          *template.Template
	userService       *user.UserService
	agentInfoService  *agent.AgentInfoService
	chatService       *chat.ChatService
	nannyAPIPort      string
	nannySwaggerURL   string
	gitHubRedirectURL string
}

// TemplateData struct
type TemplateData struct {
	User       user.User
	AuthToken  *user.AuthToken
	AuthTokens []AuthTokenData
}

type AuthTokenData struct {
	ID          string `json:"id"`
	MaskedToken string `json:"maskedToken"`
	CreatedAt   string `json:"createdAt"`
}

// startChat starts a chat session with the model using the given history.
func (s *Server) startChat(hist []content) *genai.ChatSession {
	model := s.geminiClient.Model()
	cs := model.StartChat()
	cs.History = transform(hist)
	return cs
}

// NewServer creates a new Server instance
func NewServer(geminiClient *api.GeminiClient, githubAuth *auth.GitHubAuth, userService *user.UserService, agentInfoService *agent.AgentInfoService, chatService *chat.ChatService) *Server {
	mux := http.NewServeMux()

	// override default template path if NANNY_TEMPLATE_PATH is set
	templatePath := os.Getenv("NANNY_TEMPLATE_PATH")
	if templatePath == "" {
		templatePath = "./static/index.html" // Default template path
	}

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

	tmpl := template.Must(template.ParseFiles(templatePath))
	server := &Server{mux: mux, geminiClient: geminiClient, githubAuth: githubAuth, template: tmpl, userService: userService, agentInfoService: agentInfoService, chatService: chatService, nannyAPIPort: nannyAPIPort, nannySwaggerURL: nannySwaggerURL, gitHubRedirectURL: gitHubRedirectURL}
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
	s.mux.HandleFunc("/github/login", s.githubAuth.HandleGitHubLogin())
	s.mux.HandleFunc("/github/callback", s.githubAuth.HandleGitHubCallback())
	s.mux.HandleFunc("/github/profile", s.githubAuth.HandleGitHubProfile())
	s.mux.HandleFunc("/", s.handleIndex())
	s.mux.HandleFunc("POST /create-auth-token", s.handleCreateAuthToken())
	s.mux.Handle("DELETE /auth-token/{id}", s.handleDeleteAuthToken())
	//s.mux.HandleFunc("/api/auth-tokens", s.handleGetAuthTokens())
	//s.mux.HandleFunc("DELETE /api/auth-tokens/{id}", s.handleDeleteAuthToken())
	s.mux.HandleFunc("/auth-tokens", s.handleAuthTokensPage())

	// API endoints with token authentication
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/auth-tokens", s.handleGetAuthTokens())
	apiMux.Handle("/api/user-auth-token", s.handleFetchUserInfoFromToken())
	apiMux.Handle("DELETE /api/auth-token/{id}", s.handleDeleteAuthToken())
	apiMux.HandleFunc("POST /api/agent-info", s.handleAgentInfo())
	apiMux.HandleFunc("GET /api/agent-info/", s.handleGetAgentInfoByID())
	apiMux.HandleFunc("GET /api/agent-info/{id}", s.handleGetAgentInfoByID())
	apiMux.HandleFunc("POST /api/chat", s.handleStartChat())
	apiMux.HandleFunc("PUT /api/chat/{id}", s.handleAddPromptResponse())
	apiMux.HandleFunc("GET /api/chat/", s.handleGetChatByID())
	apiMux.HandleFunc("GET /api/chat/{id}", s.handleGetChatByID())

	s.mux.Handle("/api/", s.AuthMiddleware(apiMux))

	// Serve static files from the "static" directory
	fs := http.FileServer(http.Dir("./static"))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
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
//	@Router			/user-auth-token [get]
func (s *Server) handleFetchUserInfoFromToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userInfo, ok := GetUserFromContext(r)
		if !ok {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		log.Printf("User info found in context: %s", userInfo.Email)

		json.NewEncoder(w).Encode(userInfo)
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

// handleCreateAuthToken handles the creation of auth tokens
func (s *Server) handleCreateAuthToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if user info is already in the cookie
		userInfo, err := GetUserInfoFromCookie(r)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
		if encryptionKey == "" {
			return
		}

		// Create auth token
		log.Printf("Creating auth token for user %s", userInfo.Email)
		authToken, err := s.userService.CreateAuthToken(r.Context(), userInfo.Email, encryptionKey)
		if err != nil {
			log.Printf("Failed to create auth token: %v", err)
			http.Error(w, "Failed to create auth token", http.StatusInternalServerError)
			return
		}

		if authToken == nil {
			log.Printf("Failed to create auth token")
			http.Error(w, "Failed to create auth token", http.StatusInternalServerError)
			return
		}

		// Check if the auth token is already retrieved
		if !authToken.Retrieved {
			decryptedAuthToken, err := s.userService.GetAuthTokenByToken(r.Context(), authToken.Token)
			if err != nil {
				log.Printf("Failed to retrieve auth token: %v", err)
				http.Error(w, "Failed to retrieve auth token", http.StatusInternalServerError)
				return
			}
			authToken.Token = decryptedAuthToken.Token
		}

		// Render the create-auth-token.html page
		tmpl, err := template.ParseFiles("./static/create_auth_token.html")
		if err != nil {
			log.Printf("Failed to parse create_auth_token.html: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		data := TemplateData{
			AuthToken: authToken,
		}

		err = tmpl.Execute(w, data)
		if err != nil {
			log.Printf("Failed to execute template: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	}
}

// handleIndex handles the index route
func (s *Server) handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
		if encryptionKey == "" {
			return
		}

		// Check if user info is already in the cookie
		userInfo, err := GetUserInfoFromCookie(r)
		if err == nil {
			data := TemplateData{
				User: *userInfo,
			}

			s.template.Execute(w, data)

		}
		s.template.Execute(w, TemplateData{})
	}
}

// handleGetAuthTokens retrieves all auth tokens for the authenticated user.
// @Summary Get all auth tokens
// @Description Retrieves all auth tokens for the authenticated user.
// @Tags auth-tokens
// @Produce json
// @Success 200 {array} AuthTokenData "Successfully retrieved auth tokens"
// @Failure 401 {string} string "User not authenticated"
// @Failure 500 {string} string "Failed to retrieve auth tokens"
// @Router /api/auth-tokens [get]
func (s *Server) handleGetAuthTokens() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userInfo, ok := GetUserFromContext(r)
		if !ok {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Retrieve all auth tokens for the user
		authTokens, err := s.userService.GetAllAuthTokens(r.Context(), userInfo.Email)
		if err != nil {
			log.Printf("Failed to retrieve auth tokens: %v", err)
			http.Error(w, "Failed to retrieve auth tokens", http.StatusInternalServerError)
			return
		}

		var authTokenDataList []AuthTokenData
		for _, token := range authTokens {

			authTokenDataList = append(authTokenDataList, AuthTokenData{
				ID:          token.ID.Hex(),
				MaskedToken: token.Token,
				CreatedAt:   token.CreatedAt.String(),
			})
		}
		json.NewEncoder(w).Encode(authTokenDataList)
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
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
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

		// Delete the auth token
		err = s.userService.DeleteAuthToken(r.Context(), objID)
		if err != nil {
			log.Printf("Failed to delete auth token of user %s: %v", userInfo.Email, err)
			http.Error(w, "Failed to delete auth token", http.StatusInternalServerError)
			return
		}

		// Return success response
		json.NewEncoder(w).Encode(map[string]string{"message": "Auth token deleted successfully"})
	}
}

// handleAuthTokensPage serves the auth tokens page.
// @Summary Get auth tokens page
// @Description Serves the auth tokens page.
// @Tags auth-tokens
// @Produce html
// @Success 200 {string} string "Successfully served auth tokens page"
// @Failure 500 {string} string "Internal Server Error"
// @Router /auth-tokens [get]
func (s *Server) handleAuthTokensPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if user info is already in the cookie
		userInfo, err := GetUserInfoFromCookie(r)
		if err != nil {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Retrieve all auth tokens for the user
		authTokens, err := s.userService.GetAllAuthTokens(r.Context(), userInfo.Email)
		if err != nil {
			log.Printf("Failed to retrieve auth tokens: %v", err)
			http.Error(w, "Failed to retrieve auth tokens", http.StatusInternalServerError)
			return
		}

		var authTokenDataList []AuthTokenData
		for _, token := range authTokens {

			authTokenDataList = append(authTokenDataList, AuthTokenData{
				ID:          token.ID.Hex(),
				MaskedToken: token.Token,
				CreatedAt:   token.CreatedAt.String(),
			})
		}

		data := TemplateData{
			AuthTokens: authTokenDataList,
		}

		tmpl, err := template.ParseFiles("./static/auth_tokens.html")
		if err != nil {
			log.Printf("Failed to parse auth_tokens.html: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, data)
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
// @Failure 500 {string} string "Failed to save agent info"
// @Router /api/agent-info [post]
func (s *Server) handleAgentInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Check if user info is already in the cookie
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
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

		agentInfo.Email = userInfo.Email

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
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
			http.Error(w, "User not authenticated", http.StatusUnauthorized)
			return
		}

		// Extract the ID from the URL path
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "Agent ID is required", http.StatusBadRequest)
			return
		}
		fmt.Println("ID: ", id)

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
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
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
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
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
		userInfo, _ := GetUserFromContext(r)
		if userInfo == nil {
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
