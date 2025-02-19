package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"encoding/json"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Server represents the HTTP server
type Server struct {
	mux          *http.ServeMux
	geminiClient *api.GeminiClient
	githubAuth   *auth.GitHubAuth
	template     *template.Template
	userService  *user.UserService
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
func NewServer(geminiClient *api.GeminiClient, githubAuth *auth.GitHubAuth, userService *user.UserService) *Server {
	mux := http.NewServeMux()
	templatePath := os.Getenv("NANNY_TEMPLATE_PATH")
	if templatePath == "" {
		templatePath = "./static/index.html" // Default template path
	}
	tmpl := template.Must(template.ParseFiles(templatePath))
	server := &Server{mux: mux, geminiClient: geminiClient, githubAuth: githubAuth, template: tmpl, userService: userService}
	server.routes()
	return server
}

// routes defines the routes for the server
func (s *Server) routes() {
	s.mux.HandleFunc("POST /chat", s.chatHandler)
	s.mux.HandleFunc("/status", s.handleStatus())
	s.mux.HandleFunc("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("http://localhost:8080/swagger/doc.json"),
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
	apiMux.Handle("DELETE /api/auth-token/{id}", s.handleDeleteAuthToken())

	s.mux.Handle("/api/", s.AuthMiddleware(apiMux))

	// Serve static files from the "static" directory
	fs := http.FileServer(http.Dir("./static"))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
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
