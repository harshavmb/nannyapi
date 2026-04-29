package pricing

import (
	"net/http"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterRoutes registers all pricing-related API endpoints
func RegisterRoutes(app core.App, e *core.ServeEvent, mgr *Manager, withAuth func(func(*core.RequestEvent) error) func(*core.RequestEvent) error) {
	// Public: GET /api/pricing - returns public tier info
	e.Router.GET("/api/pricing", func(c *core.RequestEvent) error {
		if !mgr.IsEnabled() {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"enabled": false,
				"message": "Pricing is not enabled on this instance",
			})
		}

		info := mgr.GetPublicInfo()
		return c.JSON(http.StatusOK, map[string]interface{}{
			"enabled": true,
			"tiers":   info.Tiers,
		})
	})

	// Authenticated: GET /api/pricing/usage - returns user's usage info
	e.Router.GET("/api/pricing/usage", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsEnabled() {
			return c.JSON(http.StatusOK, map[string]interface{}{
				"enabled": false,
				"message": "No usage limits on this instance",
			})
		}

		userID := c.Auth.Id
		info := mgr.GetUserUsageInfo(userID)
		return c.JSON(http.StatusOK, info)
	}))

	// Admin: POST /api/admin/pricing/promote - promote user tier
	e.Router.POST("/api/admin/pricing/promote", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		var req types.AdminTierPromotionRequest
		if err := c.BindBody(&req); err != nil {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
		}

		if req.UserID == "" || req.Tier == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "user_id and tier are required"})
		}

		tier := types.TierType(req.Tier)
		if tier != types.TierFree && tier != types.TierPro {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid tier, must be 'free' or 'pro'"})
		}

		adminID := c.Auth.Id
		if err := mgr.PromoteUserTier(adminID, req.UserID, tier, req.DurationDays, req.Reason); err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"message": "user tier updated successfully",
		})
	}))

	// Admin: POST /api/admin/pricing/limits - update user limits
	e.Router.POST("/api/admin/pricing/limits", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		var req types.AdminUpdateLimitsRequest
		if err := c.BindBody(&req); err != nil {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
		}

		if req.UserID == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "user_id is required"})
		}

		if err := mgr.UpdateUserLimits(req.UserID, req); err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"message": "user limits updated successfully",
		})
	}))

	// Admin: POST /api/admin/pricing/revoke - revoke tier override
	e.Router.POST("/api/admin/pricing/revoke", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		var req struct {
			UserID string `json:"user_id"`
		}
		if err := c.BindBody(&req); err != nil || req.UserID == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "user_id is required"})
		}

		if err := mgr.RevokeUserTierOverride(req.UserID); err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"message": "tier override revoked",
		})
	}))

	// Admin: GET /api/admin/pricing/config - get pricing config
	e.Router.GET("/api/admin/pricing/config", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		config := mgr.GetConfig()
		return c.JSON(http.StatusOK, config)
	}))

	// Admin: POST /api/admin/pricing/config - update pricing config
	e.Router.POST("/api/admin/pricing/config", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		var config types.PricingConfig
		if err := c.BindBody(&config); err != nil {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid config"})
		}

		if err := mgr.SaveConfigToDB(&config); err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: err.Error()})
		}

		return c.JSON(http.StatusOK, map[string]string{
			"message": "pricing config updated",
		})
	}))

	// Admin: GET /api/admin/pricing/user/:userId - get user pricing details
	e.Router.GET("/api/admin/pricing/user/{userId}", withAuth(func(c *core.RequestEvent) error {
		if !isSuperuser(app, c) {
			return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "admin access required"})
		}

		userID := c.Request.PathValue("userId")
		if userID == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "userId required"})
		}

		info := mgr.GetUserUsageInfo(userID)
		limits := mgr.GetUserLimits(userID)

		return c.JSON(http.StatusOK, map[string]interface{}{
			"usage":  info,
			"limits": limits,
			"tier":   mgr.GetUserTier(userID),
		})
	}))
}

// isSuperuser checks if the authenticated user is a PocketBase superuser
func isSuperuser(app core.App, c *core.RequestEvent) bool {
	if c.Auth == nil {
		return false
	}

	// Check if auth record is from _superusers collection
	if c.Auth.Collection().Name == "_superusers" {
		return true
	}

	// Also check if user has admin role field
	if c.Auth.Collection().Name == "users" {
		role := c.Auth.GetString("role")
		return role == "admin"
	}

	return false
}
