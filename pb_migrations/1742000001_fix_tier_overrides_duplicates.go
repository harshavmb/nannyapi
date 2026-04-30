package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		// Add autodate fields to tier_overrides and user_usage if missing
		for _, name := range []string{"tier_overrides", "user_usage"} {
			coll, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue
			}
			dirty := false
			if coll.Fields.GetByName("created") == nil {
				coll.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
				dirty = true
			}
			if coll.Fields.GetByName("updated") == nil {
				coll.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
				dirty = true
			}
			if dirty {
				if err := app.Save(coll); err != nil {
					return err
				}
			}
		}

		// Clean up duplicate active tier_overrides: keep only the most recent active override per user
		collection, err := app.FindCollectionByNameOrId("tier_overrides")
		if err != nil {
			return nil // collection doesn't exist yet, skip
		}

		// Find all active overrides (sorted by created desc so newest first)
		records, err := app.FindRecordsByFilter(collection,
			"active = true",
			"-created", 0, 0, nil)
		if err != nil || len(records) == 0 {
			goto indexes
		}

		{
			// Group by user_id, keep only the newest (first in list since sorted by -created)
			seen := make(map[string]bool)
			for _, r := range records {
				userID := r.GetString("user_id")
				if seen[userID] {
					// Duplicate — deactivate
					r.Set("active", false)
					_ = app.Save(r)
				} else {
					seen[userID] = true
				}
			}
		}

	indexes:
		// Add unique index to prevent future duplicates: only one active=true per user_id
		_, err = app.DB().NewQuery(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_tier_overrides_user_active
			ON tier_overrides (user_id)
			WHERE active = 1
		`).Execute()
		if err != nil {
			// Non-fatal: the application logic still enforces single-active
			_ = err
		}

		// Also add unique index on user_usage to prevent duplicate usage records
		_, err = app.DB().NewQuery(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_user_usage_user_id
			ON user_usage (user_id)
		`).Execute()
		if err != nil {
			_ = err
		}

		return nil
	}, func(app core.App) error {
		// Rollback: drop the indexes
		_, _ = app.DB().NewQuery(`DROP INDEX IF EXISTS idx_tier_overrides_user_active`).Execute()
		_, _ = app.DB().NewQuery(`DROP INDEX IF EXISTS idx_user_usage_user_id`).Execute()
		return nil
	})
}
