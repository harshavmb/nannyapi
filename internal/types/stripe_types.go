package types

import "time"

// StripeSubscriptionStatus mirrors Stripe's subscription status values.
type StripeSubscriptionStatus string

const (
	StripeSubActive            StripeSubscriptionStatus = "active"
	StripeSubTrialing          StripeSubscriptionStatus = "trialing"
	StripeSubPastDue           StripeSubscriptionStatus = "past_due"
	StripeSubCanceled          StripeSubscriptionStatus = "canceled"
	StripeSubIncomplete        StripeSubscriptionStatus = "incomplete"
	StripeSubIncompleteExpired StripeSubscriptionStatus = "incomplete_expired"
	StripeSubUnpaid            StripeSubscriptionStatus = "unpaid"
	StripeSubPaused            StripeSubscriptionStatus = "paused"
)

// IsActive returns true when the subscription is billable / grants access.
func (s StripeSubscriptionStatus) IsActive() bool {
	return s == StripeSubActive || s == StripeSubTrialing
}

// StripeCustomer maps a nannyapi user to a Stripe customer.
type StripeCustomer struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	StripeCustomerID string    `json:"stripe_customer_id"`
	Email            string    `json:"email"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// StripeSubscription stores subscription state mirrored from Stripe.
type StripeSubscription struct {
	ID                   string                   `json:"id"`
	UserID               string                   `json:"user_id"`
	StripeSubscriptionID string                   `json:"stripe_subscription_id"`
	StripeCustomerID     string                   `json:"stripe_customer_id"`
	StripePriceID        string                   `json:"stripe_price_id"`
	Status               StripeSubscriptionStatus `json:"status"`
	CurrentPeriodStart   time.Time                `json:"current_period_start"`
	CurrentPeriodEnd     time.Time                `json:"current_period_end"`
	CancelAtPeriodEnd    bool                     `json:"cancel_at_period_end"`
	CanceledAt           *time.Time               `json:"canceled_at,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
}

// IsActive returns true when the subscription grants Pro access.
func (s *StripeSubscription) IsActive() bool {
	return s.Status.IsActive()
}

// SubscribeRequest is the payload for POST /api/stripe/subscribe.
type SubscribeRequest struct {
	// SuccessURL is where Stripe redirects after successful payment.
	SuccessURL string `json:"success_url"`
	// CancelURL is where Stripe redirects if the user cancels checkout.
	CancelURL string `json:"cancel_url"`
}

// CancelSubscriptionRequest is the payload for POST /api/stripe/cancel-subscription.
// Cancellation always takes effect at the end of the current billing period.
type CancelSubscriptionRequest struct {
	// Reason is an optional human-readable cancellation reason stored locally.
	Reason string `json:"reason,omitempty"`
}

// ReactivateSubscriptionRequest is the payload for POST /api/stripe/reactivate-subscription.
// Only valid when the subscription is still active but cancel_at_period_end is true.
type ReactivateSubscriptionRequest struct{}

// BuyCreditsRequest is the payload for POST /api/stripe/buy-credits.
// Allows a Pro subscriber to purchase an extra token bundle for the current month.
type BuyCreditsRequest struct {
	// Quantity is the number of credit bundles to purchase (1 bundle = 1 000 000 tokens).
	Quantity int64 `json:"quantity"`
	// SuccessURL is where Stripe redirects after payment.
	SuccessURL string `json:"success_url"`
	// CancelURL is where Stripe redirects if the user cancels.
	CancelURL string `json:"cancel_url"`
}

// SubscriptionResponse is the public-facing subscription summary.
type SubscriptionResponse struct {
	HasSubscription   bool                     `json:"has_subscription"`
	Status            StripeSubscriptionStatus `json:"status,omitempty"`
	PlanName          string                   `json:"plan_name,omitempty"`
	CurrentPeriodEnd  *time.Time               `json:"current_period_end,omitempty"`
	CancelAtPeriodEnd bool                     `json:"cancel_at_period_end"`
	// CheckoutURL is populated only when a new checkout session was created.
	CheckoutURL string `json:"checkout_url,omitempty"`
}

// CheckoutSessionResponse wraps a Stripe checkout session URL.
type CheckoutSessionResponse struct {
	CheckoutURL string `json:"checkout_url"`
}
