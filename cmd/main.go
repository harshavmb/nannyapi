package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/harshavmb/nannyapi/docs"
	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/harshavmb/nannyapi/internal/auth"
	"github.com/harshavmb/nannyapi/internal/chat"
	"github.com/harshavmb/nannyapi/internal/server"
	"github.com/harshavmb/nannyapi/internal/token"
	"github.com/harshavmb/nannyapi/internal/user"
	"github.com/harshavmb/nannyapi/pkg/api"
	"github.com/harshavmb/nannyapi/pkg/database"
	"github.com/rs/cors"
)

const defaultPort = "8080"

//	@contact.name	API Support
//	@contact.url	https://nannyai.dev/support
//	@contact.email	harsha@harshanu.space

// @license.name	GNU General Public License v3.0
// @license.url	https://www.gnu.org/licenses/gpl-3.0.html
func main() {

	// programmatically set swagger info
	docs.SwaggerInfo.Title = "NannyAPI"
	docs.SwaggerInfo.Description = "This is an API endpoint service that receives prompts from nannyagents, do some preprocessing, interact with remote/self-hosted AI APIs to help answering prompts issued by nannyagents."
	docs.SwaggerInfo.Version = "2.0"
	docs.SwaggerInfo.Host = "nannyai.dev"
	docs.SwaggerInfo.BasePath = "/api/v1"

	ctx := context.Background()

	var geminiClient *api.GeminiClient
	var err error

	// Check if Gemini API key is present
	if os.Getenv("GEMINI_API_KEY") != "" {
		// Initialize Gemini API client
		geminiClient, err = api.NewGeminiClient(ctx)
		if err != nil {
			log.Fatalf("could not create Gemini client %v", err)
		}
		defer geminiClient.Close()
	}

	// Initialize MongoDB client
	mongoDB, err := database.InitDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Check if NANNY_ENCRYPTION_KEY is present in env vars
	if os.Getenv("NANNY_ENCRYPTION_KEY") == "" {
		log.Fatalf("NANNY_ENCRYPTION_KEY not set")
	}
	nannyEncryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")

	// Check if JWT_SECRET is present in env vars
	if os.Getenv("JWT_SECRET") == "" {
		log.Fatalf("JWT_SECRET not set")
	}
	jwtSecret := os.Getenv("JWT_SECRET")

	// Get the frontend Host from the environment variable
	frontendHost := os.Getenv("FRONTEND_HOST")
	if frontendHost == "" {
		frontendHost = "http://localhost:8081" // Default frontend Host
	}

	// Access preferred port the server must listen to as an environment variable if provided.
	port := defaultPort
	if os.Getenv("NANNY_API_PORT") != "" {
		port = os.Getenv("NANNY_API_PORT")
	}

	// Initialize User Repository and Service
	userRepo := user.NewUserRepository(mongoDB)
	agentInfoRepo := agent.NewAgentInfoRepository(mongoDB)
	tokenRepo := token.NewTokenRepository(mongoDB)
	refreshTokenRepo := token.NewRefreshTokenRepository(mongoDB)
	chatRepo := chat.NewChatRepository(mongoDB)
	userService := user.NewUserService(userRepo)
	tokenService := token.NewTokenService(tokenRepo)
	refreshTokenService := token.NewRefreshTokenService(refreshTokenRepo)
	agentService := agent.NewAgentInfoService(agentInfoRepo)
	chatService := chat.NewChatService(chatRepo, agentService)

	// Initialize GitHub OAuth
	githubClientID := os.Getenv("GH_CLIENT_ID")
	githubClientSecret := os.Getenv("GH_CLIENT_SECRET")
	// Get the GitHub redirect URL from the environment variable
	githubRedirectURL := os.Getenv("GH_REDIRECT_URL")
	if githubRedirectURL == "" {
		githubRedirectURL = fmt.Sprintf("http://localhost:%s/github/callback", port)
	}
	githubAuth := auth.NewGitHubAuth(githubClientID, githubClientSecret, githubRedirectURL, userService, refreshTokenService, nannyEncryptionKey, jwtSecret, frontendHost)

	// Create server with Gemini client
	srv := server.NewServer(geminiClient, githubAuth, userService, agentService, chatService, tokenService, refreshTokenService, jwtSecret, nannyEncryptionKey)

	// Add CORS middleware handler.
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:8081", "https://nannyai.dev", "https://test.nannyai.dev"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Access-Control-Allow-Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	})
	handler := c.Handler(srv)

	log.Printf("Starting server on port %s...", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
