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
	"encoding/json"
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

// ErrInvoiceNotFound is returned when an invoice record cannot be found
// for the given user.
var ErrInvoiceNotFound = errors.New("stripe: invoice not found")

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

// defaultCreditBundleTokens is the fallback when pricing.config.json is unreadable.
const defaultCreditBundleTokens int64 = 1_000_000

// creditBundleTokens reads credit_bundle_tokens from pricing.config.json.
// Falls back to 1,000,000 if the config file is missing or the field is unset.
func creditBundleTokens() int64 {
	configPath := os.Getenv("NANNYAPI_PRICING_CONFIG")
	if configPath == "" {
		configPath = "pricing.config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultCreditBundleTokens
	}
	var cfg struct {
		CreditBundleTokens int64 `json:"credit_bundle_tokens"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.CreditBundleTokens <= 0 {
		return defaultCreditBundleTokens
	}
	return cfg.CreditBundleTokens
}

// defaultProMonthlyTokenLimit is the fallback Pro tier monthly token limit.
const defaultProMonthlyTokenLimit int64 = 10_000_000

// proMonthlyTokenLimit reads the Pro tier's monthly_token_limit from
// pricing.config.json. Falls back to 10,000,000 if unavailable.
func proMonthlyTokenLimit() int64 {
	configPath := os.Getenv("NANNYAPI_PRICING_CONFIG")
	if configPath == "" {
		configPath = "pricing.config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return defaultProMonthlyTokenLimit
	}
	var cfg struct {
		Tiers map[string]struct {
			MonthlyTokenLimit int64 `json:"monthly_token_limit"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultProMonthlyTokenLimit
	}
	if pro, ok := cfg.Tiers["pro"]; ok && pro.MonthlyTokenLimit > 0 {
		return pro.MonthlyTokenLimit
	}
	return defaultProMonthlyTokenLimit
}
