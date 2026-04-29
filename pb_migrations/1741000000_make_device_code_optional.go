package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		agents, err := app.FindCollectionByNameOrId("agents")
		if err != nil {
			return err
		}

		// Make device_code_id optional so agents registered via static
		// tokens do not need a device_codes row.
		if f := agents.Fields.GetByName("device_code_id"); f != nil {
			if rf, ok := f.(*core.RelationField); ok {
				rf.Required = false
			}
		}

		// Add auth_method field: "device_code" (default/legacy) or "static_token".
		if agents.Fields.GetByName("auth_method") == nil {
			agents.Fields.Add(&core.TextField{
				Name:     "auth_method",
				Required: false,
				Max:      50,
			})
		}

		return app.Save(agents)
	}, func(app core.App) error {
		agents, err := app.FindCollectionByNameOrId("agents")
		if err != nil {
			return err
		}

		if f := agents.Fields.GetByName("device_code_id"); f != nil {
			if rf, ok := f.(*core.RelationField); ok {
				rf.Required = true
			}
		}

		if f := agents.Fields.GetByName("auth_method"); f != nil {
			agents.Fields.RemoveById(f.GetId())
		}

		return app.Save(agents)
	})
}
