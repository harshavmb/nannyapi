package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// --- pricing_config collection ---
		// Stores the global pricing configuration (JSON blob)
		pricingConfig, _ := app.FindCollectionByNameOrId("pricing_config")
		if pricingConfig == nil {
			pricingConfig = core.NewBaseCollection("pricing_config")

			pricingConfig.Fields.Add(&core.TextField{
				Name:     "config",
				Required: true,
				Max:      10000,
			})

			pricingConfig.Fields.Add(&core.BoolField{
				Name:     "enabled",
				Required: false,
			})

			if err := app.Save(pricingConfig); err != nil {
				return err
			}
		}

		// --- tier_overrides collection ---
		// Admin-granted tier promotions (time-limited or permanent)
		tierOverrides, _ := app.FindCollectionByNameOrId("tier_overrides")
		if tierOverrides == nil {
			usersCollection, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}

			tierOverrides = core.NewBaseCollection("tier_overrides")

			tierOverrides.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: true,
				MaxSelect:     1,
			})

			tierOverrides.Fields.Add(&core.TextField{
				Name:     "tier",
				Required: true,
				Max:      20,
			})

			tierOverrides.Fields.Add(&core.TextField{
				Name:     "granted_by",
				Required: true,
				Max:      255,
			})

			tierOverrides.Fields.Add(&core.TextField{
				Name:     "reason",
				Required: false,
				Max:      500,
			})

			tierOverrides.Fields.Add(&core.BoolField{
				Name:     "active",
				Required: false,
			})

			tierOverrides.Fields.Add(&core.DateField{
				Name:     "expires_at",
				Required: false,
			})

			if err := app.Save(tierOverrides); err != nil {
				return err
			}
		}

		// --- user_limit_overrides collection ---
		// Per-user custom limits set by admin
		userLimitOverrides, _ := app.FindCollectionByNameOrId("user_limit_overrides")
		if userLimitOverrides == nil {
			usersCollection, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}

			userLimitOverrides = core.NewBaseCollection("user_limit_overrides")

			userLimitOverrides.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: true,
				MaxSelect:     1,
			})

			userLimitOverrides.Fields.Add(&core.NumberField{
				Name:     "max_agents",
				Required: false,
			})

			userLimitOverrides.Fields.Add(&core.NumberField{
				Name:     "daily_token_limit",
				Required: false,
			})

			userLimitOverrides.Fields.Add(&core.NumberField{
				Name:     "monthly_token_limit",
				Required: false,
			})

			userLimitOverrides.Fields.Add(&core.NumberField{
				Name:     "daily_investigation_limit",
				Required: false,
			})

			userLimitOverrides.Fields.Add(&core.NumberField{
				Name:     "monthly_investigation_limit",
				Required: false,
			})

			if err := app.Save(userLimitOverrides); err != nil {
				return err
			}
		}

		// --- user_usage collection ---
		// Tracks daily and monthly usage per user
		userUsage, _ := app.FindCollectionByNameOrId("user_usage")
		if userUsage == nil {
			usersCollection, err := app.FindCollectionByNameOrId("users")
			if err != nil {
				return err
			}

			userUsage = core.NewBaseCollection("user_usage")

			userUsage.Fields.Add(&core.RelationField{
				Name:          "user_id",
				Required:      true,
				CollectionId:  usersCollection.Id,
				CascadeDelete: true,
				MaxSelect:     1,
			})

			userUsage.Fields.Add(&core.NumberField{
				Name:     "daily_tokens_used",
				Required: false,
			})

			userUsage.Fields.Add(&core.NumberField{
				Name:     "monthly_tokens_used",
				Required: false,
			})

			userUsage.Fields.Add(&core.NumberField{
				Name:     "daily_investigations_used",
				Required: false,
			})

			userUsage.Fields.Add(&core.NumberField{
				Name:     "monthly_investigations_used",
				Required: false,
			})

			userUsage.Fields.Add(&core.DateField{
				Name:     "daily_reset_at",
				Required: true,
			})

			userUsage.Fields.Add(&core.DateField{
				Name:     "monthly_reset_at",
				Required: true,
			})

			if err := app.Save(userUsage); err != nil {
				return err
			}
		}

		// --- Add "tier" field to users collection ---
		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		// Check if tier field already exists
		if usersCollection.Fields.GetByName("tier") == nil {
			usersCollection.Fields.Add(&core.TextField{
				Name:     "tier",
				Required: false,
				Max:      20,
			})

			if err := app.Save(usersCollection); err != nil {
				return err
			}
		}

		return nil
	}, func(app core.App) error {
		// Rollback
		collections := []string{"pricing_config", "tier_overrides", "user_limit_overrides", "user_usage"}
		for _, name := range collections {
			col, _ := app.FindCollectionByNameOrId(name)
			if col != nil {
				_ = app.Delete(col)
			}
		}
		return nil
	})
}
