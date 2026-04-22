package hooks

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/nannyagent/nannyapi/internal/agents"
	"github.com/pocketbase/pocketbase/core"
)

// LoadAuthContext returns a middleware that loads the auth record from the
// Authorization header. It supports:
//
//   - Standard PocketBase auth tokens (users or agents collections).
//   - "Static" API tokens (prefix: "nsk_") created by a user via the
//     "create-static-token" action. When a static token is presented:
//   - authRecord is set to the owning user record, AND
//   - if the request includes an `X-Agent-ID` header that refers to an
//     agent belonging to the same user, authRecord is instead set to
//     that agent record so per-agent handlers (e.g. ingest-metrics)
//     work transparently with a shared static token.
func LoadAuthContext(app core.App) func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
	return func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
		return func(e *core.RequestEvent) error {
			token := e.Request.Header.Get("Authorization")
			if token == "" {
				return next(e)
			}

			// Remove "Bearer " prefix if present (case-insensitive).
			if len(token) > 7 && strings.EqualFold(token[:7], "bearer ") {
				token = token[7:]
			}

			// Static API token path.
			if agents.IsStaticToken(token) {
				user, err := agents.ResolveStaticToken(app, token)
				if err == nil && user != nil {
					// Optional agent impersonation: allow a shared token to
					// authenticate as a specific agent under the same user.
					if agentID := e.Request.Header.Get("X-Agent-ID"); agentID != "" {
						if agentsColl, cerr := app.FindCollectionByNameOrId("agents"); cerr == nil {
							if agent, aerr := app.FindRecordById(agentsColl, agentID); aerr == nil {
								if agent.GetString("user_id") == user.Id && agent.GetString("status") != "revoked" {
									e.Set("authRecord", agent)
									e.Set("authViaStaticToken", true)
									return next(e)
								}
							}
						}
						// X-Agent-ID specified but invalid: reject explicitly.
						return e.JSON(http.StatusForbidden, map[string]string{"error": "invalid X-Agent-ID for static token"})
					}
					e.Set("authRecord", user)
					e.Set("authViaStaticToken", true)
					return next(e)
				}
				// Fall through to return unauthenticated request; handlers that
				// require auth will respond with 401.
				return next(e)
			}

			record, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth)
			if err != nil {
				fmt.Println("FindAuthRecordByToken error:", err)
			}
			if record != nil {
				e.Set("authRecord", record)
			}

			return next(e)
		}
	}
}

// RequireAuth returns a middleware that requires authentication
func RequireAuth() func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
	return func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
		return func(e *core.RequestEvent) error {
			if e.Auth == nil {
				return e.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			}
			return next(e)
		}
	}
}

// RequireAuthCollection returns a middleware that requires authentication for a specific collection
func RequireAuthCollection(collectionName string) func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
	return func(next func(*core.RequestEvent) error) func(*core.RequestEvent) error {
		return func(e *core.RequestEvent) error {
			if e.Auth == nil || e.Auth.Collection().Name != collectionName {
				return e.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			}
			return next(e)
		}
	}
}
