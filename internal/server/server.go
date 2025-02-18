package server

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"

	"encoding/json"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	httpSwagger "github.com/swaggo/http-swagger/v2"
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
	User      user.User
	AuthToken *user.AuthToken
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
	tmpl := template.Must(template.ParseFiles("./static/index.html"))
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
	s.mux.HandleFunc("/create-auth-token", s.handleCreateAuthToken())

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
		userCookie, err := r.Cookie("userinfo")
		if err != nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
		if encryptionKey == "" {
			return
		}

		if userCookie.Value != "" {
			decodedValue, err := url.QueryUnescape(userCookie.Value)
			if err != nil {
				log.Printf("Failed to URL unescape user info: %v", err)
				http.Error(w, "Failed to retrieve user info", http.StatusInternalServerError)
				return
			}

			var user user.User
			err = json.Unmarshal([]byte(decodedValue), &user)
			if err != nil {
				log.Printf("Failed to unmarshal user info: %v", err)
				http.Error(w, "Failed to retrieve user info", http.StatusInternalServerError)
				return
			}

			// Create auth token
			log.Printf("Creating auth token for user %s", user.Email)
			authToken, err := s.userService.CreateAuthToken(r.Context(), user.Email, encryptionKey)
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
		}

		// Redirect to index
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		userCookie, err := r.Cookie("userinfo")
		if err == nil && userCookie.Value != "" {
			decodedValue, err := url.QueryUnescape(userCookie.Value)
			if err != nil {
				log.Printf("Failed to URL unescape user info: %v", err)
				http.Error(w, "Failed to retrieve user info", http.StatusInternalServerError)
				return
			}

			var user user.User
			err = json.Unmarshal([]byte(decodedValue), &user)
			if err != nil {
				log.Printf("Failed to unmarshal user info: %v", err)
				http.Error(w, "Failed to retrieve user info", http.StatusInternalServerError)
				return
			}

			// Retrieve auth token
			authToken, err := s.getMaskedAuthToken(r, user.Email, encryptionKey)
			if err != nil {
				log.Printf("Failed to retrieve auth token: %v", err)
				http.Error(w, "Failed to retrieve auth token", http.StatusInternalServerError)
				return
			}

			data := TemplateData{
				User:      user,
				AuthToken: authToken,
			}

			if authToken != nil {
				s.template.Execute(w, data)
				return
			}

		}
		s.template.Execute(w, TemplateData{})
	}
}
