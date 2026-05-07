package stripe

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	stripego "github.com/stripe/stripe-go/v85"
)

// Manager provides all Stripe billing operations.  When Stripe is not
// configured (STRIPE_SECRET_KEY env var absent) every method returns
// ErrNotConfigured so callers can surface a clear message instead of
// panicking.
type Manager struct {
	app core.App
	sc  *stripego.Client // nil when unconfigured
}

// NewManager creates a Manager and initialises the Stripe client.
// It never returns an error; when the secret key is missing the Manager
// simply operates in "not configured" mode.
func NewManager(app core.App) *Manager {
	m := &Manager{app: app}
	sc, err := newStripeClient()
	if err != nil {
		if !errors.Is(err, ErrNotConfigured) {
			log.Printf("[stripe] WARNING: could not initialise Stripe client: %v", err)
		}
		return m
	}
	m.sc = sc
	return m
}

// IsConfigured reports whether the Stripe secret key is set.
func (m *Manager) IsConfigured() bool {
	return m.sc != nil
}

// WebhookSecret returns the STRIPE_WEBHOOK_SECRET env var value.
// An empty string means webhook signature verification is skipped (dev
// only – the webhook handler will reject events when the secret is
// absent unless STRIPE_SKIP_WEBHOOK_VERIFY=true is also set).
func WebhookSecret() string {
	return os.Getenv("STRIPE_WEBHOOK_SECRET")
}

// -----------------------------------------------------------------------
// Customer helpers
// -----------------------------------------------------------------------

// ensureCustomer returns the existing Stripe customer ID for the user, or
// creates one and persists it to the stripe_customers table.
func (m *Manager) ensureCustomer(userID string) (string, error) {
	// Look up existing record
	existingID, err := m.getStripeCustomerID(userID)
	if err == nil && existingID != "" {
		return existingID, nil
	}

	// Fetch user details
	usersCol, err := m.app.FindCollectionByNameOrId("users")
	if err != nil {
		return "", fmt.Errorf("stripe: users collection not found: %w", err)
	}
	user, err := m.app.FindRecordById(usersCol, userID)
	if err != nil {
		return "", fmt.Errorf("stripe: user %s not found: %w", userID, err)
	}

	email := user.GetString("email")
	name := user.GetString("name")

	// Create Stripe customer
	var stripeCustomer *stripego.Customer
	if retryErr := retryDo(defaultRetry, func() error {
		params := &stripego.CustomerCreateParams{
			Email: stripego.String(email),
			Name:  stripego.String(name),
			Metadata: map[string]string{
				"nannyapi_user_id": userID,
			},
		}
		var createErr error
		stripeCustomer, createErr = m.sc.V1Customers.Create(context.Background(), params)
		return createErr
	}); retryErr != nil {
		return "", fmt.Errorf("stripe: create customer: %w", retryErr)
	}

	// Persist to DB
	col, err := m.app.FindCollectionByNameOrId("stripe_customers")
	if err != nil {
		return "", fmt.Errorf("stripe: stripe_customers collection not found: %w", err)
	}
	rec := core.NewRecord(col)
	rec.Set("user_id", userID)
	rec.Set("stripe_customer_id", stripeCustomer.ID)
	rec.Set("email", email)
	if err := m.app.Save(rec); err != nil {
		return "", fmt.Errorf("stripe: save customer record: %w", err)
	}

	log.Printf("[stripe] created customer %s for user %s", stripeCustomer.ID, userID)
	return stripeCustomer.ID, nil
}

// getStripeCustomerID looks up the persisted Stripe customer ID for a user.
func (m *Manager) getStripeCustomerID(userID string) (string, error) {
	records, err := m.app.FindAllRecords("stripe_customers",
		dbx.NewExp("user_id = {:uid}", dbx.Params{"uid": userID}),
	)
	if err != nil || len(records) == 0 {
		return "", errors.New("not found")
	}
	return records[0].GetString("stripe_customer_id"), nil
}

// -----------------------------------------------------------------------
// Subscription queries
// -----------------------------------------------------------------------

// GetSubscription returns the local subscription record for a user.
// Returns nil, nil when the user has no subscription record.
func (m *Manager) GetSubscription(userID string) (*types.StripeSubscription, error) {
	records, err := m.app.FindAllRecords("stripe_subscriptions",
		dbx.NewExp("user_id = {:uid}", dbx.Params{"uid": userID}),
	)
	if err != nil || len(records) == 0 {
		return nil, nil
	}
	return recordToSubscription(records[0]), nil
}

// HasActiveSubscription returns true when the user has an active (or
// trialing) subscription that is not scheduled for cancellation today.
func (m *Manager) HasActiveSubscription(userID string) (bool, error) {
	sub, err := m.GetSubscription(userID)
	if err != nil {
		return false, err
	}
	if sub == nil {
		return false, nil
	}
	return sub.IsActive(), nil
}

// -----------------------------------------------------------------------
// CreateSubscriptionCheckout
// -----------------------------------------------------------------------

// CreateSubscriptionCheckout creates a Stripe Checkout Session in
// subscription mode and returns the hosted checkout URL.
//
// Rules:
//   - The user must not already have an active subscription.
//   - STRIPE_PRO_PRICE_ID env var must be set to the Stripe Price ID for
//     the monthly Pro plan.
func (m *Manager) CreateSubscriptionCheckout(userID, successURL, cancelURL string) (string, error) {
	if !m.IsConfigured() {
		return "", ErrNotConfigured
	}

	priceID := os.Getenv("STRIPE_PRO_PRICE_ID")
	if priceID == "" {
		return "", errors.New("stripe: STRIPE_PRO_PRICE_ID env var is not set")
	}

	// Guard: block double subscriptions
	active, err := m.HasActiveSubscription(userID)
	if err != nil {
		return "", err
	}
	if active {
		return "", ErrActiveSubscriptionExists
	}

	customerID, err := m.ensureCustomer(userID)
	if err != nil {
		return "", err
	}

	var session *stripego.CheckoutSession
	if retryErr := retryDo(defaultRetry, func() error {
		params := &stripego.CheckoutSessionCreateParams{
			Customer:   stripego.String(customerID),
			Mode:       stripego.String(string(stripego.CheckoutSessionModeSubscription)),
			SuccessURL: stripego.String(successURL),
			CancelURL:  stripego.String(cancelURL),
			LineItems: []*stripego.CheckoutSessionCreateLineItemParams{
				{
					Price:    stripego.String(priceID),
					Quantity: stripego.Int64(1),
				},
			},
			// Store the user ID so we can reconcile in the webhook
			ClientReferenceID: stripego.String(userID),
			// Let Stripe automatically handle tax (small business, no tax configured)
			AutomaticTax: &stripego.CheckoutSessionCreateAutomaticTaxParams{
				Enabled: stripego.Bool(false),
			},
		}
		var createErr error
		session, createErr = m.sc.V1CheckoutSessions.Create(context.Background(), params)
		return createErr
	}); retryErr != nil {
		return "", fmt.Errorf("stripe: create checkout session: %w", retryErr)
	}

	log.Printf("[stripe] checkout session %s created for user %s", session.ID, userID)
	return session.URL, nil
}

// -----------------------------------------------------------------------
// CancelSubscription
// -----------------------------------------------------------------------

// CancelSubscription schedules the user's active subscription to cancel
// at the end of the current billing period.  Stripe will continue to
// bill through the current period, then stop.
func (m *Manager) CancelSubscription(userID, reason string) error {
	if !m.IsConfigured() {
		return ErrNotConfigured
	}

	sub, err := m.GetSubscription(userID)
	if err != nil {
		return err
	}
	if sub == nil || !sub.IsActive() {
		return errors.New("stripe: no active subscription found")
	}
	if sub.CancelAtPeriodEnd {
		return errors.New("stripe: subscription is already scheduled for cancellation")
	}

	if retryErr := retryDo(defaultRetry, func() error {
		_, updateErr := m.sc.V1Subscriptions.Update(context.Background(), sub.StripeSubscriptionID, &stripego.SubscriptionUpdateParams{
			CancelAtPeriodEnd: stripego.Bool(true),
		})
		return updateErr
	}); retryErr != nil {
		return fmt.Errorf("stripe: cancel subscription: %w", retryErr)
	}

	// Update local record
	if updateErr := m.updateLocalSubscription(sub.StripeSubscriptionID, func(rec *core.Record) {
		rec.Set("cancel_at_period_end", true)
		if reason != "" {
			rec.Set("cancel_reason", reason)
		}
	}); updateErr != nil {
		log.Printf("[stripe] WARNING: failed to update local subscription after cancel: %v", updateErr)
	}

	log.Printf("[stripe] subscription %s for user %s set to cancel at period end", sub.StripeSubscriptionID, userID)
	return nil
}

// -----------------------------------------------------------------------
// ReactivateSubscription
// -----------------------------------------------------------------------

// ReactivateSubscription undoes a pending cancellation (cancel_at_period_end
// = true) so the subscription continues as normal.
func (m *Manager) ReactivateSubscription(userID string) error {
	if !m.IsConfigured() {
		return ErrNotConfigured
	}

	sub, err := m.GetSubscription(userID)
	if err != nil {
		return err
	}
	if sub == nil || !sub.IsActive() {
		return errors.New("stripe: no active subscription found")
	}
	if !sub.CancelAtPeriodEnd {
		return errors.New("stripe: subscription is not scheduled for cancellation; nothing to reactivate")
	}

	if retryErr := retryDo(defaultRetry, func() error {
		_, updateErr := m.sc.V1Subscriptions.Update(context.Background(), sub.StripeSubscriptionID, &stripego.SubscriptionUpdateParams{
			CancelAtPeriodEnd: stripego.Bool(false),
		})
		return updateErr
	}); retryErr != nil {
		return fmt.Errorf("stripe: reactivate subscription: %w", retryErr)
	}

	// Update local record
	if updateErr := m.updateLocalSubscription(sub.StripeSubscriptionID, func(rec *core.Record) {
		rec.Set("cancel_at_period_end", false)
		rec.Set("cancel_reason", "")
	}); updateErr != nil {
		log.Printf("[stripe] WARNING: failed to update local subscription after reactivate: %v", updateErr)
	}

	log.Printf("[stripe] subscription %s for user %s reactivated", sub.StripeSubscriptionID, userID)
	return nil
}

// -----------------------------------------------------------------------
// BuyCreditsCheckout
// -----------------------------------------------------------------------

// BuyCreditsCheckout creates a one-time Stripe Checkout Session allowing
// a Pro subscriber to purchase extra token bundles for the current month.
//
// The STRIPE_CREDITS_PRICE_ID env var must be set to the Stripe Price ID
// for a single credit bundle.  Quantity controls how many bundles to
// purchase.
//
// Non-Pro subscribers cannot buy credits through this endpoint – they
// must subscribe first.
func (m *Manager) BuyCreditsCheckout(userID string, quantity int64, successURL, cancelURL string) (string, error) {
	if !m.IsConfigured() {
		return "", ErrNotConfigured
	}

	if quantity < 1 || quantity > 100 {
		return "", errors.New("stripe: quantity must be between 1 and 100")
	}

	creditsPriceID := os.Getenv("STRIPE_CREDITS_PRICE_ID")
	if creditsPriceID == "" {
		return "", errors.New("stripe: STRIPE_CREDITS_PRICE_ID env var is not set")
	}

	// Only Pro subscribers can top-up
	active, err := m.HasActiveSubscription(userID)
	if err != nil {
		return "", err
	}
	if !active {
		return "", errors.New("stripe: an active Pro subscription is required to purchase extra credits")
	}

	customerID, err := m.ensureCustomer(userID)
	if err != nil {
		return "", err
	}

	var session *stripego.CheckoutSession
	if retryErr := retryDo(defaultRetry, func() error {
		params := &stripego.CheckoutSessionCreateParams{
			Customer:   stripego.String(customerID),
			Mode:       stripego.String(string(stripego.CheckoutSessionModePayment)),
			SuccessURL: stripego.String(successURL),
			CancelURL:  stripego.String(cancelURL),
			LineItems: []*stripego.CheckoutSessionCreateLineItemParams{
				{
					Price:    stripego.String(creditsPriceID),
					Quantity: stripego.Int64(quantity),
				},
			},
			ClientReferenceID: stripego.String(userID),
			AutomaticTax: &stripego.CheckoutSessionCreateAutomaticTaxParams{
				Enabled: stripego.Bool(false),
			},
		}
		var createErr error
		session, createErr = m.sc.V1CheckoutSessions.Create(context.Background(), params)
		return createErr
	}); retryErr != nil {
		return "", fmt.Errorf("stripe: create credits checkout session: %w", retryErr)
	}

	log.Printf("[stripe] credits checkout session %s created for user %s (qty=%d)", session.ID, userID, quantity)
	return session.URL, nil
}

// -----------------------------------------------------------------------
// Webhook event processing
// -----------------------------------------------------------------------

// HandleCheckoutSessionCompleted processes a checkout.session.completed
// event.  For subscription-mode sessions this activates the subscription
// and promotes the user tier to Pro.  For payment-mode sessions (credits)
// it credits token bundles.
func (m *Manager) HandleCheckoutSessionCompleted(session *stripego.CheckoutSession) error {
	userID := session.ClientReferenceID
	if userID == "" {
		log.Printf("[stripe] checkout.session.completed: no client_reference_id, ignoring")
		return nil
	}

	switch session.Mode {
	case stripego.CheckoutSessionModeSubscription:
		// In v85 session.Subscription is *stripego.Subscription (may be nil or
		// only have the ID set when the event payload is not expanded).
		var stripeSub *stripego.Subscription
		if session.Subscription != nil && session.Subscription.ID != "" {
			if retryErr := retryDo(defaultRetry, func() error {
				var getErr error
				stripeSub, getErr = m.sc.V1Subscriptions.Retrieve(context.Background(), session.Subscription.ID, &stripego.SubscriptionRetrieveParams{})
				return getErr
			}); retryErr != nil {
				return fmt.Errorf("stripe: get subscription after checkout: %w", retryErr)
			}
		} else {
			return errors.New("stripe: subscription ID missing from checkout session")
		}

		if err := m.upsertLocalSubscription(userID, stripeSub); err != nil {
			return err
		}

		// Record transaction
		customerID := ""
		if stripeSub.Customer != nil {
			customerID = stripeSub.Customer.ID
		}
		paymentIntentID := ""
		if session.PaymentIntent != nil {
			paymentIntentID = session.PaymentIntent.ID
		}
		_ = m.recordTransaction(billingTransaction{
			UserID:                 userID,
			Type:                   TxTypeSubscriptionCreated,
			Status:                 TxStatusSucceeded,
			Currency:               pricingCurrency(),
			Amount:                 session.AmountTotal,
			Description:            "Pro subscription created",
			Quantity:               1,
			ProductSlug:            "pro_subscription",
			Provider:               "stripe",
			ProviderTransactionID:  paymentIntentID,
			ProviderSubscriptionID: stripeSub.ID,
			ProviderCustomerID:     customerID,
			CustomerEmail:          m.getUserEmail(userID),
		})
		return nil

	case stripego.CheckoutSessionModePayment:
		// Credit purchase: grant additional tokens to the user
		quantity, err := m.grantCreditTokens(userID, session)
		if err != nil {
			return err
		}

		// Record credit purchase transaction
		customerID := ""
		if session.Customer != nil {
			customerID = session.Customer.ID
		}
		paymentIntentID := ""
		if session.PaymentIntent != nil {
			paymentIntentID = session.PaymentIntent.ID
		}
		_ = m.recordTransaction(billingTransaction{
			UserID:                userID,
			Type:                  TxTypeCreditsPurchased,
			Status:                TxStatusSucceeded,
			Currency:              pricingCurrency(),
			Amount:                session.AmountTotal,
			Description:           "Token credit bundle purchased",
			Quantity:              quantity,
			ProductSlug:           "credit_bundle",
			Provider:              "stripe",
			ProviderTransactionID: paymentIntentID,
			ProviderCustomerID:    customerID,
			CustomerEmail:         m.getUserEmail(userID),
		})
		return nil

	default:
		log.Printf("[stripe] checkout.session.completed: unhandled mode %s", session.Mode)
		return nil
	}
}

// HandleSubscriptionUpdated syncs an updated Stripe subscription to the
// local DB.  Called for customer.subscription.updated and
// customer.subscription.created events.
func (m *Manager) HandleSubscriptionUpdated(stripeSub *stripego.Subscription) error {
	if stripeSub.Customer == nil {
		log.Printf("[stripe] customer.subscription.updated: customer is nil, ignoring")
		return nil
	}
	// Find the user via Stripe customer ID
	userID, err := m.userIDForStripeCustomer(stripeSub.Customer.ID)
	if err != nil {
		log.Printf("[stripe] customer.subscription.updated: user not found for customer %s", stripeSub.Customer.ID)
		return nil // non-fatal: may be a customer not yet in our DB
	}
	return m.upsertLocalSubscription(userID, stripeSub)
}

// HandleSubscriptionDeleted handles customer.subscription.deleted.
// It marks the subscription as canceled in the local DB and downgrades
// the user tier to Free.
func (m *Manager) HandleSubscriptionDeleted(stripeSub *stripego.Subscription) error {
	if stripeSub.Customer == nil {
		log.Printf("[stripe] customer.subscription.deleted: customer is nil, ignoring")
		return nil
	}
	userID, err := m.userIDForStripeCustomer(stripeSub.Customer.ID)
	if err != nil {
		log.Printf("[stripe] customer.subscription.deleted: user not found for customer %s", stripeSub.Customer.ID)
		return nil
	}

	if updateErr := m.updateLocalSubscription(stripeSub.ID, func(rec *core.Record) {
		rec.Set("status", string(types.StripeSubCanceled))
		now := time.Now()
		rec.Set("canceled_at", now.Format(time.RFC3339))
	}); updateErr != nil {
		log.Printf("[stripe] WARNING: failed to update local subscription on delete: %v", updateErr)
	}

	// Downgrade user tier to Free
	if err := m.setUserTier(userID, types.TierFree); err != nil {
		log.Printf("[stripe] WARNING: failed to downgrade user %s to free tier: %v", userID, err)
	}

	// Record cancellation transaction
	_ = m.recordTransaction(billingTransaction{
		UserID:                 userID,
		Type:                   TxTypeSubscriptionCanceled,
		Status:                 TxStatusSucceeded,
		Currency:               pricingCurrency(),
		Amount:                 0,
		Description:            "Pro subscription canceled",
		Quantity:               1,
		ProductSlug:            "pro_subscription",
		Provider:               "stripe",
		ProviderSubscriptionID: stripeSub.ID,
		ProviderCustomerID:     stripeSub.Customer.ID,
		CustomerEmail:          m.getUserEmail(userID),
	})

	log.Printf("[stripe] subscription %s for user %s deleted; downgraded to free tier", stripeSub.ID, userID)
	return nil
}

// HandleInvoicePaymentSucceeded handles invoice.payment_succeeded, which
// fires on every successful renewal.  We update the subscription period
// in local DB.
func (m *Manager) HandleInvoicePaymentSucceeded(invoice *stripego.Invoice) error {
	// In v85 the subscription is accessed via invoice.Parent.SubscriptionDetails
	subID := extractInvoiceSubscriptionID(invoice)
	if subID == "" {
		return nil // one-time payment; credits are handled in checkout.session.completed
	}

	var stripeSub *stripego.Subscription
	if err := retryDo(defaultRetry, func() error {
		var getErr error
		stripeSub, getErr = m.sc.V1Subscriptions.Retrieve(context.Background(), subID, &stripego.SubscriptionRetrieveParams{})
		return getErr
	}); err != nil {
		return fmt.Errorf("stripe: get subscription on invoice payment: %w", err)
	}

	if stripeSub.Customer == nil {
		return nil
	}
	userID, err := m.userIDForStripeCustomer(stripeSub.Customer.ID)
	if err != nil {
		return nil
	}

	if err := m.upsertLocalSubscription(userID, stripeSub); err != nil {
		return err
	}

	// Record renewal transaction
	var periodStart, periodEnd *time.Time
	if len(stripeSub.Items.Data) > 0 {
		item := stripeSub.Items.Data[0]
		ps := time.Unix(item.CurrentPeriodStart, 0).UTC()
		pe := time.Unix(item.CurrentPeriodEnd, 0).UTC()
		periodStart = &ps
		periodEnd = &pe
	}
	_ = m.recordTransaction(billingTransaction{
		UserID:                 userID,
		Type:                   TxTypeSubscriptionRenewed,
		Status:                 TxStatusSucceeded,
		Currency:               pricingCurrency(),
		Amount:                 invoice.AmountPaid,
		Description:            "Pro subscription renewed",
		Quantity:               1,
		ProductSlug:            "pro_subscription",
		Provider:               "stripe",
		ProviderTransactionID:  invoice.ID,
		ProviderSubscriptionID: subID,
		ProviderCustomerID:     stripeSub.Customer.ID,
		ProviderInvoiceID:      invoice.ID,
		PeriodStart:            periodStart,
		PeriodEnd:              periodEnd,
		CustomerEmail:          m.getUserEmail(userID),
	})
	return nil
}

// HandleInvoicePaymentFailed handles invoice.payment_failed.
// We log it and update the status; no immediate tier downgrade – Stripe
// will send customer.subscription.updated with status=past_due / unpaid.
func (m *Manager) HandleInvoicePaymentFailed(invoice *stripego.Invoice) error {
	subID := extractInvoiceSubscriptionID(invoice)
	customerID := ""
	if invoice.Customer != nil {
		customerID = invoice.Customer.ID
	}
	log.Printf("[stripe] invoice.payment_failed for subscription %s (customer %s)",
		subID, customerID)

	// Record failed payment transaction
	userID := ""
	if customerID != "" {
		if uid, err := m.userIDForStripeCustomer(customerID); err == nil {
			userID = uid
		}
	}
	if userID != "" {
		_ = m.recordTransaction(billingTransaction{
			UserID:                 userID,
			Type:                   TxTypePaymentFailed,
			Status:                 TxStatusFailed,
			Currency:               pricingCurrency(),
			Amount:                 invoice.AmountDue,
			Description:            "Payment failed for subscription renewal",
			Quantity:               1,
			ProductSlug:            "pro_subscription",
			Provider:               "stripe",
			ProviderSubscriptionID: subID,
			ProviderCustomerID:     customerID,
			ProviderInvoiceID:      invoice.ID,
			CustomerEmail:          m.getUserEmail(userID),
		})
	}
	return nil
}

// extractInvoiceSubscriptionID returns the Stripe subscription ID from an
// invoice using the v85 API structure (invoice.Parent.SubscriptionDetails).
func extractInvoiceSubscriptionID(invoice *stripego.Invoice) string {
	if invoice.Parent == nil {
		return ""
	}
	if invoice.Parent.SubscriptionDetails == nil {
		return ""
	}
	if invoice.Parent.SubscriptionDetails.Subscription == nil {
		return ""
	}
	return invoice.Parent.SubscriptionDetails.Subscription.ID
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

// upsertLocalSubscription creates or updates the local subscription record
// mirroring the given Stripe subscription, and synchronises the user tier.
func (m *Manager) upsertLocalSubscription(userID string, stripeSub *stripego.Subscription) error {
	col, err := m.app.FindCollectionByNameOrId("stripe_subscriptions")
	if err != nil {
		return err
	}

	// Check if we already have a local record for this subscription
	records, _ := m.app.FindAllRecords("stripe_subscriptions",
		dbx.NewExp("stripe_subscription_id = {:sid}", dbx.Params{"sid": stripeSub.ID}),
	)

	var rec *core.Record
	if len(records) > 0 {
		rec = records[0]
	} else {
		rec = core.NewRecord(col)
		rec.Set("user_id", userID)
		rec.Set("stripe_subscription_id", stripeSub.ID)
	}

	customerID := ""
	if stripeSub.Customer != nil {
		customerID = stripeSub.Customer.ID
	}

	priceID := ""
	if len(stripeSub.Items.Data) > 0 && stripeSub.Items.Data[0].Price != nil {
		priceID = stripeSub.Items.Data[0].Price.ID
	}

	rec.Set("stripe_customer_id", customerID)
	rec.Set("stripe_price_id", priceID)
	rec.Set("status", string(stripeSub.Status))
	// In Stripe API v85 CurrentPeriodStart/End moved to SubscriptionItem.
	if len(stripeSub.Items.Data) > 0 {
		item := stripeSub.Items.Data[0]
		rec.Set("current_period_start", time.Unix(item.CurrentPeriodStart, 0).UTC().Format(time.RFC3339))
		rec.Set("current_period_end", time.Unix(item.CurrentPeriodEnd, 0).UTC().Format(time.RFC3339))
	}
	rec.Set("cancel_at_period_end", stripeSub.CancelAtPeriodEnd)
	if stripeSub.CanceledAt != 0 {
		rec.Set("canceled_at", time.Unix(stripeSub.CanceledAt, 0).UTC().Format(time.RFC3339))
	}

	if err := m.app.Save(rec); err != nil {
		return fmt.Errorf("stripe: save subscription record: %w", err)
	}

	// Sync tier
	status := types.StripeSubscriptionStatus(stripeSub.Status)
	if status.IsActive() {
		if err := m.setUserTier(userID, types.TierPro); err != nil {
			log.Printf("[stripe] WARNING: failed to set user %s to pro: %v", userID, err)
		}
	} else if status == types.StripeSubCanceled || status == types.StripeSubIncompleteExpired || status == types.StripeSubUnpaid {
		if err := m.setUserTier(userID, types.TierFree); err != nil {
			log.Printf("[stripe] WARNING: failed to downgrade user %s to free: %v", userID, err)
		}
	}

	return nil
}

// updateLocalSubscription applies a mutation function to the local
// subscription record identified by stripeSubscriptionID.
func (m *Manager) updateLocalSubscription(stripeSubscriptionID string, mutate func(*core.Record)) error {
	records, err := m.app.FindAllRecords("stripe_subscriptions",
		dbx.NewExp("stripe_subscription_id = {:sid}", dbx.Params{"sid": stripeSubscriptionID}),
	)
	if err != nil || len(records) == 0 {
		return fmt.Errorf("stripe: subscription %s not found in DB", stripeSubscriptionID)
	}
	rec := records[0]
	mutate(rec)
	return m.app.Save(rec)
}

// userIDForStripeCustomer resolves a Stripe customer ID to a nannyapi
// user ID using the stripe_customers table.
func (m *Manager) userIDForStripeCustomer(stripeCustomerID string) (string, error) {
	records, err := m.app.FindAllRecords("stripe_customers",
		dbx.NewExp("stripe_customer_id = {:cid}", dbx.Params{"cid": stripeCustomerID}),
	)
	if err != nil || len(records) == 0 {
		return "", fmt.Errorf("stripe: no user for customer %s", stripeCustomerID)
	}
	return records[0].GetString("user_id"), nil
}

// setUserTier updates the `tier` field on the users record.
func (m *Manager) setUserTier(userID string, tier types.TierType) error {
	usersCol, err := m.app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}
	user, err := m.app.FindRecordById(usersCol, userID)
	if err != nil {
		return err
	}
	user.Set("tier", string(tier))
	return m.app.Save(user)
}

// grantCreditTokens processes a successful one-time payment for extra
// token bundles.  The bundle size is read from pricing.config.json
// (credit_bundle_tokens field); defaults to 1,000,000 if not configured.
func (m *Manager) grantCreditTokens(userID string, session *stripego.CheckoutSession) (int64, error) {
	// Retrieve line items to get quantity.
	// V1CheckoutSessions.ListLineItems returns a *V1List; we fetch the first
	// page only (checkout sessions have at most a few line items).
	ctx := context.Background()
	lineItemList := m.sc.V1CheckoutSessions.ListLineItems(ctx, &stripego.CheckoutSessionListLineItemsParams{
		Session: stripego.String(session.ID),
	})
	var items []*stripego.LineItem
	for item, err := range lineItemList.All(ctx) {
		if err != nil {
			return 0, fmt.Errorf("stripe: list checkout line items: %w", err)
		}
		items = append(items, item)
	}

	bundleSize := creditBundleTokens()
	var totalQuantity int64
	var totalTokens int64
	for _, item := range items {
		totalQuantity += item.Quantity
		totalTokens += item.Quantity * bundleSize
	}
	if totalTokens == 0 {
		return totalQuantity, nil
	}

	log.Printf("[stripe] granting %d extra tokens to user %s", totalTokens, userID)
	return totalQuantity, m.addMonthlyTokens(userID, totalTokens)
}

// addMonthlyTokens increments the user's monthly token limit override.
// It adds to any existing override; if none exists, it sets the value to
// the Pro tier base limit + extra so the override (which is absolute)
// doesn't accidentally lower the user's allowance.
func (m *Manager) addMonthlyTokens(userID string, extra int64) error {
	records, _ := m.app.FindAllRecords("user_limit_overrides",
		dbx.NewExp("user_id = {:uid}", dbx.Params{"uid": userID}),
	)

	if len(records) > 0 {
		rec := records[0]
		current := rec.GetInt("monthly_token_limit")
		rec.Set("monthly_token_limit", int64(current)+extra)
		return m.app.Save(rec)
	}

	// Create a new override: base tier limit + purchased extra
	baseLimit := proMonthlyTokenLimit()
	col, err := m.app.FindCollectionByNameOrId("user_limit_overrides")
	if err != nil {
		return err
	}
	rec := core.NewRecord(col)
	rec.Set("user_id", userID)
	rec.Set("monthly_token_limit", baseLimit+extra)
	return m.app.Save(rec)
}

// recordToSubscription converts a PocketBase Record into a StripeSubscription.
func recordToSubscription(rec *core.Record) *types.StripeSubscription {
	sub := &types.StripeSubscription{
		ID:                   rec.Id,
		UserID:               rec.GetString("user_id"),
		StripeSubscriptionID: rec.GetString("stripe_subscription_id"),
		StripeCustomerID:     rec.GetString("stripe_customer_id"),
		StripePriceID:        rec.GetString("stripe_price_id"),
		Status:               types.StripeSubscriptionStatus(rec.GetString("status")),
		CancelAtPeriodEnd:    rec.GetBool("cancel_at_period_end"),
	}

	if t := rec.GetDateTime("current_period_start"); !t.IsZero() {
		sub.CurrentPeriodStart = t.Time()
	}
	if t := rec.GetDateTime("current_period_end"); !t.IsZero() {
		sub.CurrentPeriodEnd = t.Time()
	}
	if t := rec.GetDateTime("canceled_at"); !t.IsZero() {
		tt := t.Time()
		sub.CanceledAt = &tt
	}
	if t := rec.GetDateTime("created"); !t.IsZero() {
		sub.CreatedAt = t.Time()
	}
	if t := rec.GetDateTime("updated"); !t.IsZero() {
		sub.UpdatedAt = t.Time()
	}
	return sub
}

// -----------------------------------------------------------------------
// SyncSubscription – pull subscription state from Stripe API
// -----------------------------------------------------------------------

// SyncSubscription queries Stripe for the customer's active subscriptions
// and syncs the local DB + user tier.  This is the fallback for environments
// where webhooks are not forwarded (e.g. local dev without Stripe CLI).
// It also records a billing transaction if a new subscription is discovered.
func (m *Manager) SyncSubscription(userID string) error {
	if !m.IsConfigured() {
		return ErrNotConfigured
	}

	customerID, err := m.getStripeCustomerID(userID)
	if err != nil || customerID == "" {
		// No Stripe customer yet – nothing to sync
		return nil
	}

	// List active/trialing subscriptions for this customer
	ctx := context.Background()
	params := &stripego.SubscriptionListParams{
		Customer: stripego.String(customerID),
		Status:   stripego.String("all"),
	}
	iter := m.sc.V1Subscriptions.List(ctx, params)

	var activeSub *stripego.Subscription
	for sub, err := range iter.All(ctx) {
		if err != nil {
			return fmt.Errorf("stripe: list subscriptions: %w", err)
		}
		status := types.StripeSubscriptionStatus(sub.Status)
		if status.IsActive() {
			activeSub = sub
			break
		}
	}

	if activeSub == nil {
		// No active sub on Stripe – check if we have a stale local record
		localSub, _ := m.GetSubscription(userID)
		if localSub != nil && localSub.IsActive() {
			// Stripe says no active sub but local says yes – downgrade
			if updateErr := m.updateLocalSubscription(localSub.StripeSubscriptionID, func(rec *core.Record) {
				rec.Set("status", string(types.StripeSubCanceled))
			}); updateErr != nil {
				log.Printf("[stripe] sync: failed to mark stale subscription: %v", updateErr)
			}
			_ = m.setUserTier(userID, types.TierFree)
		}
		return nil
	}

	// We have an active subscription on Stripe – check if we already have it locally
	localSub, _ := m.GetSubscription(userID)
	isNew := localSub == nil || localSub.StripeSubscriptionID != activeSub.ID

	if err := m.upsertLocalSubscription(userID, activeSub); err != nil {
		return err
	}

	// Record billing transaction for newly discovered subscriptions
	if isNew {
		_ = m.recordTransaction(billingTransaction{
			UserID:                 userID,
			Type:                   TxTypeSubscriptionCreated,
			Status:                 TxStatusSucceeded,
			Currency:               pricingCurrency(),
			Amount:                 0, // we don't have session.AmountTotal here
			Description:            "Pro subscription created (synced from Stripe)",
			Quantity:               1,
			ProductSlug:            "pro_subscription",
			Provider:               "stripe",
			ProviderSubscriptionID: activeSub.ID,
			ProviderCustomerID:     customerID,
			CustomerEmail:          m.getUserEmail(userID),
		})
		log.Printf("[stripe] sync: discovered new subscription %s for user %s", activeSub.ID, userID)
	}

	return nil
}
