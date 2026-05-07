package stripe

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// TransactionType constants for billing_transactions.type
const (
	TxTypeSubscriptionCreated  = "subscription_created"
	TxTypeSubscriptionRenewed  = "subscription_renewed"
	TxTypeCreditsPurchased     = "credits_purchased"
	TxTypeSubscriptionCanceled = "subscription_canceled"
	TxTypePaymentFailed        = "payment_failed"
)

// TxStatus constants for billing_transactions.status
const (
	TxStatusSucceeded = "succeeded"
	TxStatusFailed    = "failed"
	TxStatusPending   = "pending"
)

// billingTransaction holds the fields needed to record a transaction.
type billingTransaction struct {
	UserID                 string
	Type                   string
	Status                 string
	Currency               string
	Amount                 int64 // in smallest unit (cents)
	Description            string
	Quantity               int64
	ProductSlug            string
	Provider               string
	ProviderTransactionID  string
	ProviderSubscriptionID string
	ProviderCustomerID     string
	ProviderInvoiceID      string
	PeriodStart            *time.Time
	PeriodEnd              *time.Time
	CustomerEmail          string
}

// recordTransaction persists a billing transaction to PocketBase.
func (m *Manager) recordTransaction(tx billingTransaction) error {
	col, err := m.app.FindCollectionByNameOrId("billing_transactions")
	if err != nil {
		log.Printf("[billing] WARNING: billing_transactions collection not found: %v", err)
		return nil // non-fatal: don't break payment flow if migration hasn't run
	}

	rec := core.NewRecord(col)
	rec.Set("user_id", tx.UserID)
	rec.Set("type", tx.Type)
	rec.Set("status", tx.Status)
	rec.Set("currency", tx.Currency)
	rec.Set("amount", tx.Amount)
	rec.Set("description", tx.Description)
	rec.Set("quantity", tx.Quantity)
	rec.Set("product_slug", tx.ProductSlug)
	rec.Set("provider", tx.Provider)
	rec.Set("provider_transaction_id", tx.ProviderTransactionID)
	rec.Set("provider_subscription_id", tx.ProviderSubscriptionID)
	rec.Set("provider_customer_id", tx.ProviderCustomerID)
	rec.Set("provider_invoice_id", tx.ProviderInvoiceID)
	rec.Set("customer_email", tx.CustomerEmail)

	if tx.PeriodStart != nil {
		rec.Set("period_start", tx.PeriodStart.Format(time.RFC3339))
	}
	if tx.PeriodEnd != nil {
		rec.Set("period_end", tx.PeriodEnd.Format(time.RFC3339))
	}

	if err := m.app.Save(rec); err != nil {
		log.Printf("[billing] WARNING: failed to save transaction: %v", err)
		return nil // non-fatal
	}
	return nil
}

// getUserEmail fetches the email for a user ID.
func (m *Manager) getUserEmail(userID string) string {
	usersCol, err := m.app.FindCollectionByNameOrId("users")
	if err != nil {
		return ""
	}
	user, err := m.app.FindRecordById(usersCol, userID)
	if err != nil {
		return ""
	}
	return user.GetString("email")
}

// pricingCurrency reads the currency from pricing.config.json. Defaults to "eur".
func pricingCurrency() string {
	configPath := os.Getenv("NANNYAPI_PRICING_CONFIG")
	if configPath == "" {
		configPath = "pricing.config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "eur"
	}
	var cfg struct {
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Currency == "" {
		return "eur"
	}
	return cfg.Currency
}

// SeedProductCatalog reads pricing.config.json and upserts records into
// the product_catalog collection. Called on app startup.
func SeedProductCatalog(app core.App) error {
	col, err := app.FindCollectionByNameOrId("product_catalog")
	if err != nil {
		log.Printf("[billing] product_catalog collection not found, skipping seed")
		return nil
	}

	configPath := os.Getenv("NANNYAPI_PRICING_CONFIG")
	if configPath == "" {
		configPath = "pricing.config.json"
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("[billing] pricing config not found at %s, skipping catalog seed", configPath)
		return nil
	}

	var cfg struct {
		Enabled            bool    `json:"enabled"`
		Currency           string  `json:"currency"`
		CreditBundleTokens int64   `json:"credit_bundle_tokens"`
		CreditBundlePrice  float64 `json:"credit_bundle_price"`
		Tiers              map[string]struct {
			Name          string  `json:"name"`
			PricePerMonth float64 `json:"price_per_month"`
		} `json:"tiers"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("billing: parse pricing config: %w", err)
	}

	if !cfg.Enabled {
		return nil
	}

	currency := cfg.Currency
	if currency == "" {
		currency = "eur"
	}

	proPriceID := os.Getenv("STRIPE_PRO_PRICE_ID")
	creditsPriceID := os.Getenv("STRIPE_CREDITS_PRICE_ID")

	// Upsert Pro subscription product
	if pro, ok := cfg.Tiers["pro"]; ok && pro.PricePerMonth > 0 {
		if err := upsertProduct(app, col, productSeed{
			Slug:              "pro_subscription",
			Name:              "NannyAPI Pro",
			Description:       "Pro tier subscription with 10M monthly tokens and unlimited agents/investigations",
			Type:              "subscription",
			Currency:          currency,
			Amount:            int64(math.Round(pro.PricePerMonth * 100)), // convert to cents
			Interval:          "month",
			TokensPerUnit:     0,
			Provider:          "stripe",
			ProviderProductID: os.Getenv("STRIPE_PRO_PRODUCT_ID"),
			ProviderPriceID:   proPriceID,
		}); err != nil {
			return err
		}
	}

	// Upsert credit bundle product
	if cfg.CreditBundleTokens > 0 && cfg.CreditBundlePrice > 0 {
		if err := upsertProduct(app, col, productSeed{
			Slug:              "credit_bundle",
			Name:              "Token Credit Bundle",
			Description:       fmt.Sprintf("%d additional tokens", cfg.CreditBundleTokens),
			Type:              "one_time",
			Currency:          currency,
			Amount:            int64(math.Round(cfg.CreditBundlePrice * 100)),
			Interval:          "one_time",
			TokensPerUnit:     cfg.CreditBundleTokens,
			Provider:          "stripe",
			ProviderProductID: os.Getenv("STRIPE_CREDITS_PRODUCT_ID"),
			ProviderPriceID:   creditsPriceID,
		}); err != nil {
			return err
		}
	}

	log.Printf("[billing] product catalog seeded from pricing config")
	return nil
}

type productSeed struct {
	Slug              string
	Name              string
	Description       string
	Type              string
	Currency          string
	Amount            int64
	Interval          string
	TokensPerUnit     int64
	Provider          string
	ProviderProductID string
	ProviderPriceID   string
}

func upsertProduct(app core.App, col *core.Collection, p productSeed) error {
	// Find existing by slug
	records, _ := app.FindAllRecords("product_catalog",
		dbx.NewExp("slug = {:slug}", dbx.Params{"slug": p.Slug}),
	)

	var rec *core.Record
	if len(records) > 0 {
		rec = records[0]
	} else {
		rec = core.NewRecord(col)
	}

	rec.Set("slug", p.Slug)
	rec.Set("name", p.Name)
	rec.Set("description", p.Description)
	rec.Set("type", p.Type)
	rec.Set("currency", p.Currency)
	rec.Set("amount", p.Amount)
	rec.Set("interval", p.Interval)
	rec.Set("tokens_per_unit", p.TokensPerUnit)
	rec.Set("provider", p.Provider)
	rec.Set("provider_product_id", p.ProviderProductID)
	rec.Set("provider_price_id", p.ProviderPriceID)
	rec.Set("active", true)

	return app.Save(rec)
}
