package stripe

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/dbx"
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
//	GET  /api/stripe/invoices                 – paginated invoice list (auth required)
//	GET  /api/stripe/invoices/{id}/pdf        – redirect to Stripe invoice PDF (auth required)
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

		// Sync subscription state from Stripe (handles missing webhooks)
		if syncErr := mgr.SyncSubscription(userID); syncErr != nil {
			// Non-fatal: log and continue with whatever local state exists
			log.Printf("[stripe] sync on subscription check failed for user %s: %v", userID, syncErr)
		}

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

	// ------------------------------------------------------------------ //
	// GET /api/stripe/invoices – paginated invoice list (auth required)
	// Superusers may pass ?user_id=<id> to view invoices on behalf of a user.
	// ------------------------------------------------------------------ //
	e.Router.GET("/api/stripe/invoices", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		userID := resolveTargetUserID(app, c)

		// Sync invoices from Stripe (lazy; non-fatal on failure)
		if syncErr := mgr.SyncInvoices(userID); syncErr != nil {
			log.Printf("[stripe] invoice sync failed for user %s: %v", userID, syncErr)
		}

		// Parse pagination params
		page, _ := strconv.Atoi(c.Request.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(c.Request.URL.Query().Get("per_page"))
		if page < 1 {
			page = 1
		}
		if perPage < 1 || perPage > 100 {
			perPage = 10
		}

		result, err := mgr.GetInvoices(userID, page, perPage)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{
				Error: "failed to retrieve invoices",
			})
		}
		return c.JSON(http.StatusOK, result)
	}))

	// ------------------------------------------------------------------ //
	// GET /api/stripe/invoices/{id}/pdf – redirect to Stripe PDF (auth required)
	// Superusers may pass ?user_id=<id> to download on behalf of a user.
	// ------------------------------------------------------------------ //
	e.Router.GET("/api/stripe/invoices/{id}/pdf", withAuth(func(c *core.RequestEvent) error {
		if !mgr.IsConfigured() {
			return notConfiguredResponse(c)
		}

		userID := resolveTargetUserID(app, c)
		invoiceID := c.Request.PathValue("id")
		if invoiceID == "" {
			return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invoice id is required"})
		}

		pdfURL, err := mgr.GetInvoicePDFURL(userID, invoiceID)
		if err != nil {
			if err.Error() == "stripe: invoice not found" {
				return c.JSON(http.StatusNotFound, types.ErrorResponse{Error: "invoice not found"})
			}
			return c.JSON(http.StatusInternalServerError, types.ErrorResponse{
				Error: "failed to retrieve invoice PDF",
			})
		}

		// Redirect to the Stripe-hosted PDF
		c.Response.Header().Set("Location", pdfURL)
		c.Response.WriteHeader(http.StatusTemporaryRedirect)
		return nil
	}))

	// ------------------------------------------------------------------ //
	// GET /api/billing/transactions – user's billing history
	// ------------------------------------------------------------------ //
	e.Router.GET("/api/billing/transactions", withAuth(func(c *core.RequestEvent) error {
		userID := c.Auth.Id
		records, err := app.FindAllRecords("billing_transactions",
			dbx.NewExp("user_id = {:uid}", dbx.Params{"uid": userID}),
		)
		if err != nil {
			return c.JSON(http.StatusOK, []any{})
		}

		txs := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			txs = append(txs, map[string]any{
				"id":                       rec.Id,
				"type":                     rec.GetString("type"),
				"status":                   rec.GetString("status"),
				"currency":                 rec.GetString("currency"),
				"amount":                   rec.GetInt("amount"),
				"description":              rec.GetString("description"),
				"quantity":                 rec.GetInt("quantity"),
				"product_slug":             rec.GetString("product_slug"),
				"provider":                 rec.GetString("provider"),
				"provider_transaction_id":  rec.GetString("provider_transaction_id"),
				"provider_subscription_id": rec.GetString("provider_subscription_id"),
				"period_start":             rec.GetString("period_start"),
				"period_end":               rec.GetString("period_end"),
				"created":                  rec.GetString("created"),
			})
		}
		return c.JSON(http.StatusOK, txs)
	}))

	// ------------------------------------------------------------------ //
	// GET /api/billing/products – public product catalog
	// ------------------------------------------------------------------ //
	e.Router.GET("/api/billing/products", func(c *core.RequestEvent) error {
		records, err := app.FindAllRecords("product_catalog",
			dbx.NewExp("active = {:a}", dbx.Params{"a": true}),
		)
		if err != nil {
			return c.JSON(http.StatusOK, []any{})
		}

		products := make([]map[string]any, 0, len(records))
		for _, rec := range records {
			products = append(products, map[string]any{
				"id":              rec.Id,
				"slug":            rec.GetString("slug"),
				"name":            rec.GetString("name"),
				"description":     rec.GetString("description"),
				"type":            rec.GetString("type"),
				"currency":        rec.GetString("currency"),
				"amount":          rec.GetInt("amount"),
				"interval":        rec.GetString("interval"),
				"tokens_per_unit": rec.GetInt("tokens_per_unit"),
			})
		}
		return c.JSON(http.StatusOK, products)
	})
}

// -----------------------------------------------------------------------
// Error helpers
// -----------------------------------------------------------------------

func notConfiguredResponse(c *core.RequestEvent) error {
	return c.JSON(http.StatusServiceUnavailable, types.ErrorResponse{
		Error: "payment integration is not configured on this instance",
	})
}

// resolveTargetUserID returns the user ID to operate on.  If the caller is a
// superuser and a ?user_id=<id> query param is provided, that value is used
// (admin acting on behalf of a user).  Otherwise returns c.Auth.Id.
func resolveTargetUserID(app core.App, c *core.RequestEvent) string {
	if c.Auth != nil && c.Auth.Collection().Name == "_superusers" {
		if uid := c.Request.URL.Query().Get("user_id"); uid != "" {
			return uid
		}
	}
	return c.Auth.Id
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
