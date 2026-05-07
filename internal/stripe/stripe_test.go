package stripe

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	stripego "github.com/stripe/stripe-go/v85"
)

// ---------------------------------------------------------------------------
// retryDo / backoffDelay
// ---------------------------------------------------------------------------

func TestRetryDo_SuccessOnFirstAttempt(t *testing.T) {
	calls := 0
	err := retryDo(defaultRetry, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryDo_NonRetryableError_ReturnedImmediately(t *testing.T) {
	sentinel := errors.New("non-retryable")
	calls := 0
	err := retryDo(defaultRetry, func() error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retries), got %d", calls)
	}
}

func TestRetryDo_RateLimitRetries(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 3,
		baseDelay:   1 * time.Millisecond, // speed up test
		maxDelay:    10 * time.Millisecond,
	}
	calls := 0
	err := retryDo(cfg, func() error {
		calls++
		// Always return a 429 error
		return &stripego.Error{
			HTTPStatusCode: http.StatusTooManyRequests,
			Msg:            "rate limited",
		}
	})
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if calls != cfg.maxAttempts {
		t.Errorf("expected %d calls, got %d", cfg.maxAttempts, calls)
	}
}

func TestRetryDo_SucceedsAfterOneRateLimit(t *testing.T) {
	cfg := retryConfig{
		maxAttempts: 3,
		baseDelay:   1 * time.Millisecond,
		maxDelay:    10 * time.Millisecond,
	}
	calls := 0
	err := retryDo(cfg, func() error {
		calls++
		if calls == 1 {
			return &stripego.Error{
				HTTPStatusCode: http.StatusTooManyRequests,
				Msg:            "rate limited",
			}
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected nil after retry success, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestBackoffDelay_InBounds(t *testing.T) {
	cfg := retryConfig{
		baseDelay: 100 * time.Millisecond,
		maxDelay:  5 * time.Second,
	}
	for attempt := 0; attempt < 5; attempt++ {
		d := backoffDelay(cfg, attempt)
		if d < cfg.baseDelay/4 {
			t.Errorf("attempt %d: delay %s is below minimum %s", attempt, d, cfg.baseDelay/4)
		}
		if d > cfg.maxDelay {
			t.Errorf("attempt %d: delay %s exceeds maxDelay %s", attempt, d, cfg.maxDelay)
		}
	}
}

// ---------------------------------------------------------------------------
// extractInvoiceSubscriptionID
// ---------------------------------------------------------------------------

func TestExtractInvoiceSubscriptionID_Nil(t *testing.T) {
	got := extractInvoiceSubscriptionID(&stripego.Invoice{})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractInvoiceSubscriptionID_NilDetails(t *testing.T) {
	invoice := &stripego.Invoice{
		Parent: &stripego.InvoiceParent{},
	}
	got := extractInvoiceSubscriptionID(invoice)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractInvoiceSubscriptionID_NilSubscription(t *testing.T) {
	invoice := &stripego.Invoice{
		Parent: &stripego.InvoiceParent{
			SubscriptionDetails: &stripego.InvoiceParentSubscriptionDetails{},
		},
	}
	got := extractInvoiceSubscriptionID(invoice)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractInvoiceSubscriptionID_Valid(t *testing.T) {
	const want = "sub_abc123"
	invoice := &stripego.Invoice{
		Parent: &stripego.InvoiceParent{
			SubscriptionDetails: &stripego.InvoiceParentSubscriptionDetails{
				Subscription: &stripego.Subscription{ID: want},
			},
		},
	}
	got := extractInvoiceSubscriptionID(invoice)
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// newStripeClient (environment-gated)
// ---------------------------------------------------------------------------

func TestNewStripeClient_NotConfigured(t *testing.T) {
	// Ensure STRIPE_SECRET_KEY is unset for this test.
	t.Setenv("STRIPE_SECRET_KEY", "")
	sc, err := newStripeClient()
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
	if sc != nil {
		t.Error("expected nil client when not configured")
	}
}

func TestNewStripeClient_Configured(t *testing.T) {
	// Use a fake test key (sk_test_*) – client construction succeeds without
	// making any network calls.
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_key_for_unit_tests")
	sc, err := newStripeClient()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if sc == nil {
		t.Error("expected non-nil client")
	}
}

// ---------------------------------------------------------------------------
// creditBundleTokens
// ---------------------------------------------------------------------------

func TestCreditBundleTokens_ReadsFromConfig(t *testing.T) {
	// Write a temp config with a custom bundle size
	tmpFile := t.TempDir() + "/pricing.json"
	if err := os.WriteFile(tmpFile, []byte(`{"credit_bundle_tokens": 2000000}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NANNYAPI_PRICING_CONFIG", tmpFile)

	got := creditBundleTokens()
	if got != 2_000_000 {
		t.Errorf("expected 2000000, got %d", got)
	}
}

func TestCreditBundleTokens_FallsBackToDefault(t *testing.T) {
	t.Setenv("NANNYAPI_PRICING_CONFIG", "/nonexistent/path.json")

	got := creditBundleTokens()
	if got != defaultCreditBundleTokens {
		t.Errorf("expected %d, got %d", defaultCreditBundleTokens, got)
	}
}

func TestCreditBundleTokens_InvalidJSON_FallsBack(t *testing.T) {
	tmpFile := t.TempDir() + "/bad.json"
	if err := os.WriteFile(tmpFile, []byte(`not json`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NANNYAPI_PRICING_CONFIG", tmpFile)

	got := creditBundleTokens()
	if got != defaultCreditBundleTokens {
		t.Errorf("expected %d, got %d", defaultCreditBundleTokens, got)
	}
}

func TestCreditBundleTokens_ZeroValue_FallsBack(t *testing.T) {
	tmpFile := t.TempDir() + "/zero.json"
	if err := os.WriteFile(tmpFile, []byte(`{"credit_bundle_tokens": 0}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NANNYAPI_PRICING_CONFIG", tmpFile)

	got := creditBundleTokens()
	if got != defaultCreditBundleTokens {
		t.Errorf("expected %d, got %d", defaultCreditBundleTokens, got)
	}
}
