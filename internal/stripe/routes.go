package stripe

import (
	"errors"
	"net/http"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/core"
)

// RegisterRoutes attaches all Stripe billing endpoints to the router.
//
// Routes added:
//
//	GET  /api/stripe/subscription             – current subscription status (auth required)
//	POST /api/stripe/subscribe                – create checkout session for Pro (auth required)
//	POST /api/stripe/cancel-subscription      – cancel at period end (auth required)
//	POST /api/stripe/reactivate-subscription  – undo pending cancellation (auth required)
//	POST /api/stripe/buy-credits              – ad-hoc token top-up (auth required, Pro only)
//	POST /api/stripe/webhook                  – Stripe event webhook (public, signed)
//
// When Stripe is not configured every authenticated endpoint returns a
// 503 with a clear message, allowing self-hosted users to ignore the
// billing subsystem entirely.
func RegisterRoutes(
	app core.App,
	e *core.ServeEvent,
	mgr *Manager,
	withAuth func(func(*core.RequestEvent) error) func(*core.RequestEvent) error,
) {
	// ------------------------------------------------------------------ //
	// GET /api/stripe/subscription
	// ------------------------------------------------------------------ //
	e.Router.GET("/api/stripe/subscription", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		userID := c.Auth.Id
		sub, err := mgr.GetSubscription(userID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{
				Error: "failed to retrieve subscription",
			})
		}
		if sub == nil {
			return c.JSON(http.StatusOK, types.SubscriptionResponse{
				HasSubscription: false,
			})
		}

		resp := types.SubscriptionResponse{
			HasSubscription:   true,
			Status:            sub.Status,
			CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
		}
		if !sub.CurrentPeriodEnd.IsZero() {
			resp.CurrentPeriodEnd = &sub.CurrentPeriodEnd
		}
		if sub.IsActive() {
			resp.PlanName = "Pro"
		}
		return c.JSON(http.StatusOK, resp)
	}))

	// ------------------------------------------------------------------ //
	// POST /api/stripe/subscribe
	// ------------------------------------------------------------------ //
	e.Router.POST("/api/stripe/subscribe", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		var req types.SubscribeRequest
		if err := c.BindBody(&req); err != nil {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request body"})
		}
		if req.SuccessURL == "" || req.CancelURL == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{
				Error: "success_url and cancel_url are required",
			})
		}

		userID := c.Auth.Id
		checkoutURL, err := mgr.CreateSubscriptionCheckout(userID, req.SuccessURL, req.CancelURL)
		if err != nil {
			return stripeErrorResponse(c, err)
		}
		return c.JSON(http.StatusOK, types.CheckoutSessionResponse{CheckoutURL: checkoutURL})
	}))

	// ------------------------------------------------------------------ //
	// POST /api/stripe/cancel-subscription
	// ------------------------------------------------------------------ //
	e.Router.POST("/api/stripe/cancel-subscription", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		var req types.CancelSubscriptionRequest
		// reason is optional; ignore bind errors
		_ = c.BindBody(&req)

		userID := c.Auth.Id
		if err := mgr.CancelSubscription(userID, req.Reason); err != nil {
			return stripeErrorResponse(c, err)
		}
		return c.JSON(http.StatusOK, map[string]string{
			"message": "subscription will be cancelled at the end of the current billing period",
		})
	}))

	// ------------------------------------------------------------------ //
	// POST /api/stripe/reactivate-subscription
	// ------------------------------------------------------------------ //
	e.Router.POST("/api/stripe/reactivate-subscription", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		userID := c.Auth.Id
		if err := mgr.ReactivateSubscription(userID); err != nil {
			return stripeErrorResponse(c, err)
		}
		return c.JSON(http.StatusOK, map[string]string{
			"message": "subscription reactivated; billing will continue as normal",
		})
	}))

	// ------------------------------------------------------------------ //
	// POST /api/stripe/buy-credits
	// ------------------------------------------------------------------ //
	e.Router.POST("/api/stripe/buy-credits", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		var req types.BuyCreditsRequest
		if err := c.BindBody(&req); err != nil {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request body"})
		}
		if req.Quantity < 1 {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{
				Error: "quantity must be at least 1",
			})
		}
		if req.SuccessURL == "" || req.CancelURL == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{
				Error: "success_url and cancel_url are required",
			})
		}

		userID := c.Auth.Id
		checkoutURL, err := mgr.BuyCreditsCheckout(userID, req.Quantity, req.SuccessURL, req.CancelURL)
		if err != nil {
			return stripeErrorResponse(c, err)
		}
		return c.JSON(http.StatusOK, types.CheckoutSessionResponse{CheckoutURL: checkoutURL})
	}))

	// ------------------------------------------------------------------ //
	// POST /api/stripe/webhook  (no auth – verified by Stripe signature)
	// ------------------------------------------------------------------ //
	e.Router.POST("/api/stripe/webhook", HandleWebhook(app, mgr))
}

// -----------------------------------------------------------------------
// Error helpers
// -----------------------------------------------------------------------

func notConfiguredResponse(c *core.RequestEvent) error {
	return c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
		Error: "payment integration is not configured on this instance",
	})
}

// stripeErrorResponse maps known domain errors to appropriate HTTP codes.
func stripeErrorResponse(c *core.RequestEvent, err error) error {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return notConfiguredResponse(c)
	case errors.Is(err, ErrActiveSubscriptionExists):
		return c.JSON(http.StatusConflict, types.ErrorResponse{Error: err.Error()})
	default:
		// Surface user-friendly messages for known validation errors
		msg := err.Error()
		// Strip "stripe: " prefix for cleaner client messages
		if len(msg) > 8 && msg[:8] == "stripe: " {
			msg = msg[8:]
		}
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: msg})
	}
}
