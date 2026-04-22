package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Backfills `created` and `updated` autodate fields on the operation
// collections that the stuck-operation reaper relies on for staleness
// detection. Prior to this migration these collections were created
// without the autodate system fields, so `record.GetDateTime("updated")`
// silently returned the zero time everywhere.
//
// Idempotent: only adds fields that do not already exist.
func init() {
	m.Register(func(app core.App) error {
		for _, name := range []string{"patch_operations", "reboot_operations", "investigations"} {
			coll, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				// Collection may not exist on very old deployments; skip.
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
		return nil
	}, func(app core.App) error {
		// Non-destructive: leave the autodate columns in place on rollback.
		return nil
	})
}
