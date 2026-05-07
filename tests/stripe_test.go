package tests

import (
	"errors"
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
	t.Setenv("STRIPE_SECRET_KEY", "")
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
	t.Setenv("STRIPE_SECRET_KEY", "")
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
	t.Setenv("STRIPE_SECRET_KEY", "")
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

// ---------------------------------------------------------------------------
// Invoice endpoints
// ---------------------------------------------------------------------------

// TestStripeManager_GetInvoices_EmptyForNewUser verifies that a new user
// with no invoice history gets an empty but valid response.
func TestStripeManager_GetInvoices_EmptyForNewUser(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	app := setupTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "invoice-new@example.com", "Password1!")
	m := stripeintegration.NewManager(app)

	result, err := m.GetInvoices(user.Id, 1, 10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.TotalItems != 0 {
		t.Errorf("expected 0 total items for new user, got %d", result.TotalItems)
	}
	if result.TotalPages != 1 {
		t.Errorf("expected 1 total page (minimum), got %d", result.TotalPages)
	}
	if result.Page != 1 {
		t.Errorf("expected page 1, got %d", result.Page)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected empty items slice, got %d items", len(result.Items))
	}
}

// TestStripeManager_GetInvoices_Pagination verifies pagination math with
// pre-seeded invoice records.
func TestStripeManager_GetInvoices_Pagination(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	app := setupTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "invoice-page@example.com", "Password1!")

	// Seed 25 invoice records directly into the DB
	col, err := app.FindCollectionByNameOrId("stripe_invoices")
	if err != nil {
		t.Skipf("stripe_invoices collection not found: %v", err)
	}

	for i := 0; i < 25; i++ {
		rec := core.NewRecord(col)
		rec.Set("user_id", user.Id)
		rec.Set("stripe_invoice_id", "in_test_"+string(rune('A'+i)))
		rec.Set("stripe_customer_id", "cus_test")
		rec.Set("status", "paid")
		rec.Set("currency", "eur")
		rec.Set("amount_due", 1000+i)
		rec.Set("amount_paid", 1000+i)
		rec.Set("invoice_created", "2026-04-01 00:00:00.000Z")
		if err := app.Save(rec); err != nil {
			t.Fatalf("failed to seed invoice %d: %v", i, err)
		}
	}

	m := stripeintegration.NewManager(app)

	// Page 1 with perPage=10
	result, err := m.GetInvoices(user.Id, 1, 10)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if result.TotalItems != 25 {
		t.Errorf("expected 25 total items, got %d", result.TotalItems)
	}
	if result.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", result.TotalPages)
	}
	if len(result.Items) != 10 {
		t.Errorf("expected 10 items on page 1, got %d", len(result.Items))
	}

	// Page 3 should have 5 items
	result, err = m.GetInvoices(user.Id, 3, 10)
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(result.Items) != 5 {
		t.Errorf("expected 5 items on page 3, got %d", len(result.Items))
	}

	// Page 4 (beyond range) should be empty
	result, err = m.GetInvoices(user.Id, 4, 10)
	if err != nil {
		t.Fatalf("page 4: %v", err)
	}
	if len(result.Items) != 0 {
		t.Errorf("expected 0 items on page 4, got %d", len(result.Items))
	}
}

// TestStripeManager_GetInvoicePDFURL_NotFound verifies that requesting a PDF
// for a non-existent invoice returns an error.
func TestStripeManager_GetInvoicePDFURL_NotFound(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_pdf")
	app := setupTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "invoice-pdf@example.com", "Password1!")
	m := stripeintegration.NewManager(app)

	_, err := m.GetInvoicePDFURL(user.Id, "nonexistent-record-id")
	if err == nil {
		t.Fatal("expected error for nonexistent invoice, got nil")
	}
}

// TestStripeManager_GetInvoicePDFURL_WrongUser verifies that a user cannot
// access another user's invoice PDF.
func TestStripeManager_GetInvoicePDFURL_WrongUser(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "sk_test_fake_pdf_wrong")
	app := setupTestApp(t)
	defer app.Cleanup()

	user1 := createTestUser(app, t, "invoice-owner@example.com", "Password1!")
	user2 := createTestUser(app, t, "invoice-thief@example.com", "Password1!")

	// Create an invoice for user1
	col, err := app.FindCollectionByNameOrId("stripe_invoices")
	if err != nil {
		t.Skipf("stripe_invoices collection not found: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user_id", user1.Id)
	rec.Set("stripe_invoice_id", "in_owned_by_user1")
	rec.Set("stripe_customer_id", "cus_user1")
	rec.Set("status", "paid")
	rec.Set("currency", "eur")
	rec.Set("amount_due", 1000)
	rec.Set("amount_paid", 1000)
	rec.Set("invoice_created", "2026-04-01 00:00:00.000Z")
	if err := app.Save(rec); err != nil {
		t.Fatalf("failed to seed invoice: %v", err)
	}

	m := stripeintegration.NewManager(app)

	// user2 should not be able to get user1's invoice PDF
	_, err = m.GetInvoicePDFURL(user2.Id, rec.Id)
	if err == nil {
		t.Fatal("expected error when accessing another user's invoice, got nil")
	}
}

// TestStripeManager_SyncInvoices_NotConfigured verifies that SyncInvoices
// returns ErrNotConfigured when Stripe is not set up.
func TestStripeManager_SyncInvoices_NotConfigured(t *testing.T) {
	t.Setenv("STRIPE_SECRET_KEY", "")
	app := setupTestApp(t)
	defer app.Cleanup()

	m := stripeintegration.NewManager(app)

	err := m.SyncInvoices("some-user-id")
	if !errors.Is(err, stripeintegration.ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}
