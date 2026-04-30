package tests

import (
	"errors"
	"os"
	"testing"

	stripeintegration "github.com/nannyagent/nannyapi/internal/stripe"
	_ "github.com/nannyagent/nannyapi/pb_migrations"
	"github.com/pocketbase/pocketbase/core"
)

// ---------------------------------------------------------------------------
// Manager (unconfigured)
// ---------------------------------------------------------------------------

// TestStripeManager_NotConfigured verifies that all Manager operations return
// ErrNotConfigured when STRIPE_SECRET_KEY is not set, so self-hosted users get
// a clear error message rather than a panic.
func TestStripeManager_NotConfigured(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	app := setupTestApp(t)
	defer app.Cleanup()

	m := stripeintegration.NewManager(app)

	if m.IsConfigured() {
		t.Error("expected IsConfigured() = false without STRIPE_SECRET_KEY")
	}

	t.Run("CreateSubscriptionCheckout returns ErrNotConfigured", func(t *testing.T) {
		_, err := m.CreateSubscriptionCheckout("uid", "https://ok", "https://cancel")
		if !errors.Is(err, stripeintegration.ErrNotConfigured) {
			t.Errorf("expected ErrNotConfigured, got %v", err)
		}
	})

	t.Run("CancelSubscription returns ErrNotConfigured", func(t *testing.T) {
		err := m.CancelSubscription("uid", "")
		if !errors.Is(err, stripeintegration.ErrNotConfigured) {
			t.Errorf("expected ErrNotConfigured, got %v", err)
		}
	})

	t.Run("ReactivateSubscription returns ErrNotConfigured", func(t *testing.T) {
		err := m.ReactivateSubscription("uid")
		if !errors.Is(err, stripeintegration.ErrNotConfigured) {
			t.Errorf("expected ErrNotConfigured, got %v", err)
		}
	})

	t.Run("BuyCreditsCheckout returns ErrNotConfigured", func(t *testing.T) {
		_, err := m.BuyCreditsCheckout("uid", 1, "https://ok", "https://cancel")
		if !errors.Is(err, stripeintegration.ErrNotConfigured) {
			t.Errorf("expected ErrNotConfigured, got %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Manager.GetSubscription / HasActiveSubscription
// ---------------------------------------------------------------------------

// TestStripeManager_GetSubscription_NoRecord verifies that GetSubscription
// returns nil (not an error) for a user with no subscription in the DB.
func TestStripeManager_GetSubscription_NoRecord(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	app := setupTestApp(t)
	defer app.Cleanup()

	m := stripeintegration.NewManager(app)

	sub, err := m.GetSubscription("nonexistent-user-id")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if sub != nil {
		t.Errorf("expected nil subscription for unknown user, got %+v", sub)
	}
}

// TestStripeManager_HasActiveSubscription_NoRecord verifies that
// HasActiveSubscription returns false (not an error) for unknown users.
func TestStripeManager_HasActiveSubscription_NoRecord(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	app := setupTestApp(t)
	defer app.Cleanup()

	m := stripeintegration.NewManager(app)

	active, err := m.HasActiveSubscription("nonexistent-user-id")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if active {
		t.Error("expected HasActiveSubscription = false for unknown user")
	}
}

// ---------------------------------------------------------------------------
// Double-subscription guard
// ---------------------------------------------------------------------------

// TestStripeManager_DoubleSubscriptionPrevented verifies that
// CreateSubscriptionCheckout returns ErrActiveSubscriptionExists when a
// subscription is already active.  This test injects a fake active
// subscription record directly into the DB to simulate an already-subscribed
// user without making real Stripe API calls.
func TestStripeManager_DoubleSubscriptionPrevented(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_for_double_sub_test")
	t.Setenv("STRIPE_PRO_PRICE_ID", "price_fake123")

	app := setupTestApp(t)
	defer app.Cleanup()

	// Create a test user.
	user := createTestUser(app, t, "sub-test@example.com", "Password1!")

	// Insert a fake active stripe_subscription row.
	col, err := app.FindCollectionByNameOrId("stripe_subscriptions")
	if err != nil {
		t.Skipf("stripe_subscriptions collection not found (migration may not have run): %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user_id", user.Id)
	rec.Set("stripe_subscription_id", "sub_fake123")
	rec.Set("stripe_customer_id", "cus_fake123")
	rec.Set("status", "active")
	rec.Set("cancel_at_period_end", false)
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to insert fake subscription: %v", err)
	}

	m := stripeintegration.NewManager(app)

	_, err = m.CreateSubscriptionCheckout(user.Id, "https://ok", "https://cancel")
	if !errors.Is(err, stripeintegration.ErrActiveSubscriptionExists) {
		t.Errorf("expected ErrActiveSubscriptionExists, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// BuyCredits guard – requires active subscription
// ---------------------------------------------------------------------------

// TestStripeManager_BuyCredits_RequiresActiveSubscription verifies that
// BuyCreditsCheckout refuses to create a credits checkout when the user has
// no active subscription.
func TestStripeManager_BuyCredits_RequiresActiveSubscription(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_credits_test")
	t.Setenv("STRIPE_CREDITS_PRICE_ID", "price_credits_fake")

	app := setupTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "credits-test@example.com", "Password1!")

	m := stripeintegration.NewManager(app)

	_, err := m.BuyCreditsCheckout(user.Id, 2, "https://ok", "https://cancel")
	if err == nil {
		t.Fatal("expected error for non-subscriber, got nil")
	}
	// Should NOT be ErrNotConfigured (we ARE configured); should be a
	// "requires active subscription" error.
	if errors.Is(err, stripeintegration.ErrNotConfigured) {
		t.Error("unexpected ErrNotConfigured; user is not subscribed, should get subscription-required error")
	}
}

// ---------------------------------------------------------------------------
// Quantity validation
// ---------------------------------------------------------------------------

func TestStripeManager_BuyCredits_InvalidQuantity(t *testing.T) {
	os.Unsetenv("STRIPE_SECRET_KEY")
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_qty_test")
	t.Setenv("STRIPE_CREDITS_PRICE_ID", "price_credits_fake")

	app := setupTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "qty-test@example.com", "Password1!")
	m := stripeintegration.NewManager(app)

	for _, qty := range []int64{0, -1, 101, 200} {
		_, err := m.BuyCreditsCheckout(user.Id, qty, "https://ok", "https://cancel")
		if err == nil {
			t.Errorf("quantity %d: expected validation error, got nil", qty)
		}
	}
}
