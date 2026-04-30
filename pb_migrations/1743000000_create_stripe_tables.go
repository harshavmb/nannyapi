package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Migration 1743000000 – create Stripe billing tables.
//
// Tables:
//   - stripe_customers  – 1-to-1 mapping between a nannyapi user and a
//     Stripe Customer object.
//   - stripe_subscriptions – mirrors Stripe subscription state so the app
//     never needs to call Stripe synchronously during request handling.
func init() {
	m.Register(func(app core.App) error {
		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// ------------------------------------------------------------------ //
		// stripe_customers
		// ------------------------------------------------------------------ //
		if sc, _ := app.FindCollectionByNameOrId("stripe_customers"); sc == nil {
			sc = core.NewBaseCollection("stripe_customers")

			sc.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: true,
				MaxSelect:     1,
			})

			// Stripe customer IDs are at most 18 chars (cus_XXXXXXXXXXXXXXXXXX)
			sc.Fields.Add(&core.TextField{
				Name:     "stripe_customer_id",
				Required: true,
				Max:      64,
			})

			sc.Fields.Add(&core.TextField{
				Name:     "email",
				Required: false,
				Max:      255,
			})

			sc.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			sc.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

			if err := app.Save(sc); err != nil {
				return err
			}
		}

		// ------------------------------------------------------------------ //
		// stripe_subscriptions
		// ------------------------------------------------------------------ //
		if ss, _ := app.FindCollectionByNameOrId("stripe_subscriptions"); ss == nil {
			ss = core.NewBaseCollection("stripe_subscriptions")

			ss.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: true,
				MaxSelect:     1,
			})

			ss.Fields.Add(&core.TextField{
				Name:     "stripe_subscription_id",
				Required: true,
				Max:      64,
			})

			ss.Fields.Add(&core.TextField{
				Name:     "stripe_customer_id",
				Required: true,
				Max:      64,
			})

			ss.Fields.Add(&core.TextField{
				Name:     "stripe_price_id",
				Required: false,
				Max:      64,
			})

			// status: active | trialing | past_due | canceled | incomplete |
			//         incomplete_expired | unpaid | paused
			ss.Fields.Add(&core.TextField{
				Name:     "status",
				Required: true,
				Max:      32,
			})

			ss.Fields.Add(&core.DateField{
				Name:     "current_period_start",
				Required: false,
			})

			ss.Fields.Add(&core.DateField{
				Name:     "current_period_end",
				Required: false,
			})

			ss.Fields.Add(&core.BoolField{
				Name:     "cancel_at_period_end",
				Required: false,
			})

			ss.Fields.Add(&core.DateField{
				Name:     "canceled_at",
				Required: false,
			})

			// Optional: cancellation reason stored by the app (not sent to Stripe)
			ss.Fields.Add(&core.TextField{
				Name:     "cancel_reason",
				Required: false,
				Max:      500,
			})

			ss.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
			ss.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

			if err := app.Save(ss); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// Down: drop tables in reverse order (children first)
		for _, name := range []string{"stripe_subscriptions", "stripe_customers"} {
			if coll, err := app.FindCollectionByNameOrId(name); err == nil {
				if err := app.Delete(coll); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
