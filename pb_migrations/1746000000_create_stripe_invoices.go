package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Migration 1746000000 – create stripe_invoices collection.
//
// Stores invoice metadata fetched from Stripe so the portal can display
// billing history without calling Stripe on every page load.  PDF URLs
// are NOT stored (they expire); instead a dedicated endpoint proxies
// the download on demand.
func init() {
	m.Register(func(app core.App) error {
		if existing, _ := app.FindCollectionByNameOrId("stripe_invoices"); existing != nil {
			return nil // already exists
		}

		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		col := core.NewBaseCollection("stripe_invoices")

		col.Fields.Add(&core.RelationField{
			Name:          "user_id",
			Required:      true,
			CollectionId:  usersCollection.Id,
			CascadeDelete: false, // preserve history
			MaxSelect:     1,
		})

		// Stripe invoice ID (e.g. "in_1TS...")
		col.Fields.Add(&core.TextField{
			Name:     "stripe_invoice_id",
			Required: true,
			Max:      128,
		})

		// Stripe customer ID
		col.Fields.Add(&core.TextField{
			Name:     "stripe_customer_id",
			Required: false,
			Max:      128,
		})

		// Stripe subscription ID (if associated)
		col.Fields.Add(&core.TextField{
			Name:     "stripe_subscription_id",
			Required: false,
			Max:      128,
		})

		// Invoice number (human-readable, e.g. "ABC-001")
		col.Fields.Add(&core.TextField{
			Name:     "invoice_number",
			Required: false,
			Max:      64,
		})

		// Invoice status: "draft", "open", "paid", "uncollectible", "void"
		col.Fields.Add(&core.TextField{
			Name:     "status",
			Required: true,
			Max:      32,
		})

		// Currency code
		col.Fields.Add(&core.TextField{
			Name:     "currency",
			Required: true,
			Max:      8,
		})

		// Amount due in smallest unit (cents)
		col.Fields.Add(&core.NumberField{
			Name:     "amount_due",
			Required: false,
		})

		// Amount paid in smallest unit (cents)
		col.Fields.Add(&core.NumberField{
			Name:     "amount_paid",
			Required: false,
		})

		// Billing period start (the month this invoice covers)
		col.Fields.Add(&core.DateField{
			Name:     "period_start",
			Required: false,
		})

		// Billing period end
		col.Fields.Add(&core.DateField{
			Name:     "period_end",
			Required: false,
		})

		// Invoice created timestamp
		col.Fields.Add(&core.DateField{
			Name:     "invoice_created",
			Required: false,
		})

		// Invoice finalized timestamp (when it became payable)
		col.Fields.Add(&core.DateField{
			Name:     "finalized_at",
			Required: false,
		})

		// Invoice paid timestamp
		col.Fields.Add(&core.DateField{
			Name:     "paid_at",
			Required: false,
		})

		// Description / memo (first line item description)
		col.Fields.Add(&core.TextField{
			Name:     "description",
			Required: false,
			Max:      512,
		})

		// Hosted invoice URL (Stripe-hosted page, long-lived)
		col.Fields.Add(&core.TextField{
			Name:     "hosted_invoice_url",
			Required: false,
			Max:      1024,
		})

		col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

		return app.Save(col)
	}, func(app core.App) error {
		col, _ := app.FindCollectionByNameOrId("stripe_invoices")
		if col != nil {
			return app.Delete(col)
		}
		return nil
	})
}
