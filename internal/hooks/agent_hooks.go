package hooks

import (
	"net/http"
	"os"
	"time"

	"github.com/nannyagent/nannyapi/internal/agents"
	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterAgentHooks registers all agent-related functionality
func RegisterAgentHooks(app core.App) {
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:8080"
	}

	// Single agent management endpoint handling all operations
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// POST /api/agent - Handles all agent operations
		e.Router.POST("/api/agent", LoadAuthContext(app)(func(c *core.RequestEvent) error {
			var baseReq struct {
				Action string `json:"action"`
			}
			if err := c.BindBody(&baseReq); err != nil {
				return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
			}

			switch baseReq.Action {
			case "device-auth-start":
				return agents.HandleDeviceAuthStart(app, c, frontendURL)
			case "authorize":
				return agents.HandleAuthorize(app, c)
			case "register":
				return agents.HandleRegister(app, c)
			case "refresh":
				return agents.HandleRefreshToken(app, c)
			case "renew-refresh-token":
				return agents.HandleRenewRefreshToken(app, c)
			case "ingest-metrics":
				return agents.HandleIngestMetrics(app, c)
			case "list":
				return agents.HandleListAgents(app, c)
			case "revoke":
				return agents.HandleRevokeAgent(app, c)
			case "health":
				return agents.HandleAgentHealth(app, c)
			case "create-static-token":
				return agents.HandleCreateStaticToken(app, c)
			case "list-static-tokens":
				return agents.HandleListStaticTokens(app, c)
			case "revoke-static-token":
				return agents.HandleRevokeStaticToken(app, c)
			default:
				return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "unknown action"})
			}
		}))

		// Cleanup cron job
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				agents.CleanupExpiredDeviceCodes(app)
			}
		}()

		return e.Next()
	})
}
