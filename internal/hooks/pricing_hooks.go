package hooks

import (
	"net/http"

	"github.com/nannyagent/nannyapi/internal/pricing"
	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterPricingHooks registers pricing and rate-limiting hooks
func RegisterPricingHooks(app core.App, mgr *pricing.Manager) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// Auth middleware wrapper
		withAuth := func(handler func(*core.RequestEvent) error) func(*core.RequestEvent) error {
			return LoadAuthContext(app)(RequireAuth()(handler))
		}

		// Register pricing routes
		pricing.RegisterRoutes(app, e, mgr, withAuth)

		return e.Next()
	})

	// Set default tier to "free" on new user creation
	app.OnRecordCreate("users").BindFunc(func(e *core.RecordEvent) error {
		if e.Record.GetString("tier") == "" {
			e.Record.Set("tier", string(types.TierFree))
		}
		return e.Next()
	})

	// Intercept agent registration to enforce agent limits
	app.OnRecordCreate("agents").BindFunc(func(e *core.RecordEvent) error {
		if !mgr.IsEnabled() {
			return e.Next()
		}

		userID := e.Record.GetString("user_id")
		if userID == "" {
			return e.Next()
		}

		if limitErr := mgr.CheckAgentLimit(userID); limitErr != nil {
			return apis.NewApiError(http.StatusTooManyRequests, limitErr.Message, map[string]any{
				"code":      limitErr.Code,
				"limit":     limitErr.Limit,
				"used":      limitErr.Used,
				"resets_at": limitErr.ResetsAt,
			})
		}

		return e.Next()
	})

	// Intercept investigation creation to enforce investigation limits
	app.OnRecordCreate("investigations").BindFunc(func(e *core.RecordEvent) error {
		if !mgr.IsEnabled() {
			return e.Next()
		}

		userID := e.Record.GetString("user_id")
		if userID == "" {
			return e.Next()
		}

		if limitErr := mgr.CheckInvestigationLimit(userID); limitErr != nil {
			return apis.NewApiError(http.StatusTooManyRequests, limitErr.Message, map[string]any{
				"code":      limitErr.Code,
				"limit":     limitErr.Limit,
				"used":      limitErr.Used,
				"resets_at": limitErr.ResetsAt,
			})
		}

		// Record usage after successful save
		if err := e.Next(); err != nil {
			return err
		}

		_ = mgr.RecordInvestigationUsage(userID)
		return nil
	})
}

// CheckTokenLimitMiddleware is a helper for handlers that consume tokens.
// Call before making AI calls. Returns error response if limit breached.
func CheckTokenLimitMiddleware(mgr *pricing.Manager, c *core.RequestEvent, estimatedTokens int64) *types.UsageLimitError {
	if !mgr.IsEnabled() {
		return nil
	}

	if c.Auth == nil {
		return nil
	}

	userID := c.Auth.Id
	// If auth is agent, get user_id from agent record
	if c.Auth.Collection().Name == "agents" {
		userID = c.Auth.GetString("user_id")
	}

	return mgr.CheckTokenLimit(userID, estimatedTokens)
}
