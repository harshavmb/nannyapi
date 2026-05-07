package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Migration 1744000000 – create payment-provider-agnostic billing tables.
//
// Tables:
//   - product_catalog – mirrors the pricing config as PocketBase records so
//     the portal can read products without calling external APIs.
//   - billing_transactions – stores every payment event (subscription start,
//     renewal, credit purchase, refund) with correlation to user, product, and
//     provider-specific IDs.
func init() {
	m.Register(func(app core.App) error {
		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// ------------------------------------------------------------------ //
		// product_catalog
		// ------------------------------------------------------------------ //
		if pc, _ := app.FindCollectionByNameOrId("product_catalog"); pc == nil {
			pc = core.NewBaseCollection("product_catalog")

			// slug: "pro_subscription", "credit_bundle"
			pc.Fields.Add(&core.TextField{
				Name:     "slug",
				Required: true,
				Max:      64,
			})

			pc.Fields.Add(&core.TextField{
				Name:     "name",
				Required: true,
				Max:      128,
			})

			pc.Fields.Add(&core.TextField{
				Name:     "description",
				Required: false,
				Max:      512,
			})

			// "subscription" or "one_time"
			pc.Fields.Add(&core.TextField{
				Name:     "type",
				Required: true,
				Max:      32,
			})

			// Currency code: "eur", "usd", etc.
			pc.Fields.Add(&core.TextField{
				Name:     "currency",
				Required: true,
				Max:      8,
			})

			// Amount in smallest unit (cents). E.g. €10 = 1000
			pc.Fields.Add(&core.NumberField{
				Name:     "amount",
				Required: true,
			})

			// Billing interval: "month", "year", "one_time"
			pc.Fields.Add(&core.TextField{
				Name:     "interval",
				Required: false,
				Max:      16,
			})

			// For token bundles: number of tokens granted per unit purchased
			pc.Fields.Add(&core.NumberField{
				Name:     "tokens_per_unit",
				Required: false,
			})

			// External provider references (Stripe product/price IDs, etc.)
			pc.Fields.Add(&core.TextField{
				Name:     "provider",
				Required: false,
				Max:      32,
			})

			pc.Fields.Add(&core.TextField{
				Name:     "provider_product_id",
				Required: false,
				Max:      128,
			})

			pc.Fields.Add(&core.TextField{
				Name:     "provider_price_id",
				Required: false,
				Max:      128,
			})

			pc.Fields.Add(&core.BoolField{
				Name:     "active",
				Required: false,
			})

			pc.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			pc.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

			if err := app.Save(pc); err != nil {
				return err
			}
		}

		// ------------------------------------------------------------------ //
		// billing_transactions
		// ------------------------------------------------------------------ //
		if bt, _ := app.FindCollectionByNameOrId("billing_transactions"); bt == nil {
			bt = core.NewBaseCollection("billing_transactions")

			bt.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: false, // keep transaction history even if user is deleted
				MaxSelect:     1,
			})

			// Transaction type: "subscription_created", "subscription_renewed",
			// "credits_purchased", "subscription_canceled", "refund"
			bt.Fields.Add(&core.TextField{
				Name:     "type",
				Required: true,
				Max:      64,
			})

			// Status: "succeeded", "failed", "pending", "refunded"
			bt.Fields.Add(&core.TextField{
				Name:     "status",
				Required: true,
				Max:      32,
			})

			// Currency code
			bt.Fields.Add(&core.TextField{
				Name:     "currency",
				Required: true,
				Max:      8,
			})

			// Amount in smallest unit (cents). E.g. €10 = 1000
			bt.Fields.Add(&core.NumberField{
				Name:     "amount",
				Required: false,
			})

			// Human-readable description
			bt.Fields.Add(&core.TextField{
				Name:     "description",
				Required: false,
				Max:      512,
			})

			// Quantity (e.g., number of credit bundles purchased)
			bt.Fields.Add(&core.NumberField{
				Name:     "quantity",
				Required: false,
			})

			// Product reference (slug from product_catalog)
			bt.Fields.Add(&core.TextField{
				Name:     "product_slug",
				Required: false,
				Max:      64,
			})

			// Payment provider: "stripe", "paypal", etc.
			bt.Fields.Add(&core.TextField{
				Name:     "provider",
				Required: true,
				Max:      32,
			})

			// Provider-specific transaction/payment intent ID
			bt.Fields.Add(&core.TextField{
				Name:     "provider_transaction_id",
				Required: false,
				Max:      128,
			})

			// Provider-specific subscription ID (for recurring payments)
			bt.Fields.Add(&core.TextField{
				Name:     "provider_subscription_id",
				Required: false,
				Max:      128,
			})

			// Provider customer ID
			bt.Fields.Add(&core.TextField{
				Name:     "provider_customer_id",
				Required: false,
				Max:      128,
			})

			// Invoice ID from provider
			bt.Fields.Add(&core.TextField{
				Name:     "provider_invoice_id",
				Required: false,
				Max:      128,
			})

			// Period for subscription renewals
			bt.Fields.Add(&core.DateField{
				Name:     "period_start",
				Required: false,
			})

			bt.Fields.Add(&core.DateField{
				Name:     "period_end",
				Required: false,
			})

			// Customer email at time of transaction (denormalized for history)
			bt.Fields.Add(&core.TextField{
				Name:     "customer_email",
				Required: false,
				Max:      255,
			})

			bt.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			bt.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

			if err := app.Save(bt); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		for _, name := range []string{"billing_transactions", "product_catalog"} {
			if coll, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(coll); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
