// Package stripe implements billing via the Stripe payment platform.
//
// The package is optional: when STRIPE_SECRET_KEY is not set the Manager
// returns ErrNotConfigured for all operations so callers can surface a
// friendly "payment not available on this instance" message without crashing.
//
// Rate limiting (HTTP 429) is handled transparently with exponential
// back-off + jitter.  See retryDo for details.
package stripe

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	stripego "github.com/stripe/stripe-go/v85"
)

// ErrNotConfigured is returned when the Stripe secret key has not been set.
var ErrNotConfigured = errors.New("stripe: payment integration is not configured on this instance")

// ErrActiveSubscriptionExists is returned when a user tries to subscribe
// while already having an active subscription.
var ErrActiveSubscriptionExists = errors.New("stripe: an active subscription already exists; cancel it first or buy extra credits instead")

// retryConfig holds exponential-backoff parameters.
type retryConfig struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
}

var defaultRetry = retryConfig{
	maxAttempts: 4,
	baseDelay:   500 * time.Millisecond,
	maxDelay:    30 * time.Second,
}

// retryDo executes fn with exponential back-off + full jitter whenever
// Stripe returns a 429 (rate-limited). Other errors are returned immediately.
func retryDo(cfg retryConfig, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < cfg.maxAttempts; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		var stripeErr *stripego.Error
		if errors.As(err, &stripeErr) {
			if stripeErr.HTTPStatusCode == http.StatusTooManyRequests {
				delay := backoffDelay(cfg, attempt)
				log.Printf("[stripe] rate limited (attempt %d/%d): %s - retrying in %s",
					attempt+1, cfg.maxAttempts, stripeErr.Msg, delay)
				time.Sleep(delay)
				lastErr = err
				continue
			}
		}
		return err
	}
	return fmt.Errorf("stripe: max retries exceeded: %w", lastErr)
}

// backoffDelay computes exponential delay with full jitter.
func backoffDelay(cfg retryConfig, attempt int) time.Duration {
	ceiling := cfg.baseDelay * (1 << attempt)
	if ceiling > cfg.maxDelay {
		ceiling = cfg.maxDelay
	}
	// #nosec G404
	jitter := time.Duration(rand.Int63n(int64(ceiling)))
	if jitter < cfg.baseDelay/4 {
		jitter = cfg.baseDelay / 4
	}
	return jitter
}

// newStripeClient builds a new stripe.Client from STRIPE_SECRET_KEY env var.
// Returns nil, ErrNotConfigured when the variable is absent.
func newStripeClient() (*stripego.Client, error) {
	key := os.Getenv("STRIPE_SECRET_KEY")
	if key == "" {
		return nil, ErrNotConfigured
	}
	// Use a custom backend config: disable SDK-level retries because retryDo
	// handles back-off with full jitter, and disable telemetry.
	backends := stripego.NewBackendsWithConfig(&stripego.BackendConfig{
		MaxNetworkRetries: stripego.Int64(0),
		EnableTelemetry:   stripego.Bool(false),
	})
	sc := stripego.NewClient(key, stripego.WithBackends(backends))
	return sc, nil
}
