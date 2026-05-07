package stripe

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/pocketbase/pocketbase/core"
	stripego "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"
)

// maxWebhookPayload is the maximum number of bytes we read from a webhook
// request body (Stripe payloads are well under 64 KiB in practice).
const maxWebhookPayload = 65536

// HandleWebhook is the raw http.HandlerFunc-compatible webhook dispatcher.
// It:
//  1. Reads and size-caps the request body.
//  2. Verifies the Stripe-Signature header (unless dev mode is enabled).
//  3. Dispatches to the appropriate Manager handler.
//
// It is registered as POST /api/stripe/webhook in routes.go.
func HandleWebhook(app core.App, mgr *Manager) func(*core.RequestEvent) error {
	return func(c *core.RequestEvent) error {
		payload, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookPayload))
		if err != nil {
			log.Printf("[stripe] webhook: failed to read body: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "cannot read request body"})
		}

		sigHeader := c.Request.Header.Get("Stripe-Signature")
		secret := WebhookSecret()

		var event stripego.Event

		if secret == "" {
			// Dev / test mode: skip signature verification only when the
			// safety env var is explicitly set.
			if os.Getenv("STRIPE_SKIP_WEBHOOK_VERIFY") != "true" {
				log.Printf("[stripe] webhook: STRIPE_WEBHOOK_SECRET is not set; rejecting event")
				return c.JSON(http.StatusBadRequest, map[string]string{
					"error": "webhook secret not configured; set STRIPE_WEBHOOK_SECRET",
				})
			}
			if jsonErr := json.Unmarshal(payload, &event); jsonErr != nil {
				log.Printf("[stripe] webhook: failed to parse event: %v", jsonErr)
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			}
		} else {
			var verifyErr error
			event, verifyErr = webhook.ConstructEvent(payload, sigHeader, secret)
			if verifyErr != nil {
				// ErrNotSigned or signature mismatch – potential tampering
				log.Printf("[stripe] webhook: signature verification failed: %v", verifyErr)
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid webhook signature"})
			}
		}

		log.Printf("[stripe] webhook event: %s (id=%s)", event.Type, event.ID)

		if err := dispatchEvent(mgr, &event); err != nil {
			// Return 500 so Stripe retries transient processing failures
			// (e.g. temporary DB or network outages).
			log.Printf("[stripe] webhook: error handling %s: %v", event.Type, err)
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to process webhook event",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{"received": "true"})
	}
}

// dispatchEvent routes a verified Stripe event to the right handler.
func dispatchEvent(mgr *Manager, event *stripego.Event) error {
	switch event.Type {
	case "checkout.session.completed":
		var session stripego.CheckoutSession
		if err := unmarshalData(event.Data.Raw, &session); err != nil {
			return err
		}
		return mgr.HandleCheckoutSessionCompleted(&session)

	case "customer.subscription.created", "customer.subscription.updated":
		var sub stripego.Subscription
		if err := unmarshalData(event.Data.Raw, &sub); err != nil {
			return err
		}
		return mgr.HandleSubscriptionUpdated(&sub)

	case "customer.subscription.deleted":
		var sub stripego.Subscription
		if err := unmarshalData(event.Data.Raw, &sub); err != nil {
			return err
		}
		return mgr.HandleSubscriptionDeleted(&sub)

	case "invoice.payment_succeeded":
		var invoice stripego.Invoice
		if err := unmarshalData(event.Data.Raw, &invoice); err != nil {
			return err
		}
		return mgr.HandleInvoicePaymentSucceeded(&invoice)

	case "invoice.payment_failed":
		var invoice stripego.Invoice
		if err := unmarshalData(event.Data.Raw, &invoice); err != nil {
			return err
		}
		return mgr.HandleInvoicePaymentFailed(&invoice)

	default:
		log.Printf("[stripe] webhook: unhandled event type %s – ignoring", event.Type)
		return nil
	}
}

// unmarshalData deserialises event.Data.Raw into the target struct.
func unmarshalData(raw json.RawMessage, target interface{}) error {
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("stripe: unmarshal %T: %w", target, err)
	}
	return nil
}
