package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Creates the agent_static_tokens collection used to store long-lived,
// user-owned API tokens that can be shared by one or more agents.
func init() {
	m.Register(func(app core.App) error {
		existing, _ := app.FindCollectionByNameOrId("agent_static_tokens")
		if existing != nil {
			return nil
		}

		usersCollection, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		coll := core.NewBaseCollection("agent_static_tokens")

		coll.Fields.Add(&core.RelationField{
			Name:          "user_id",
			Required:      true,
			CollectionId:  usersCollection.Id,
			CascadeDelete: true,
			MaxSelect:     1,
		})

		coll.Fields.Add(&core.TextField{
			Name:     "name",
			Required: true,
			Max:      120,
		})

		// SHA-256 hash of the issued plaintext token (never store the plaintext).
		coll.Fields.Add(&core.TextField{
			Name:     "token_hash",
			Required: true,
			Max:      128,
		})

		// Human-readable prefix (first N chars of the token) for UI display.
		coll.Fields.Add(&core.TextField{
			Name:     "token_prefix",
			Required: false,
			Max:      32,
		})

		// nil/zero => never expires.
		coll.Fields.Add(&core.DateField{
			Name:     "expires_at",
			Required: false,
		})

		coll.Fields.Add(&core.BoolField{
			Name:     "revoked",
			Required: false,
		})

		coll.Fields.Add(&core.DateField{
			Name:     "revoked_at",
			Required: false,
		})

		coll.Fields.Add(&core.DateField{
			Name:     "last_used_at",
			Required: false,
		})

		// Autodate created/updated timestamps.
		coll.Fields.Add(&core.AutodateField{
			Name:     "created",
			OnCreate: true,
		})
		coll.Fields.Add(&core.AutodateField{
			Name:     "updated",
			OnCreate: true,
			OnUpdate: true,
		})

		// Index lookup by hash.
		coll.AddIndex("idx_agent_static_tokens_hash", true, "token_hash", "")
		coll.AddIndex("idx_agent_static_tokens_user", false, "user_id", "")

		return app.Save(coll)
	}, func(app core.App) error {
		coll, _ := app.FindCollectionByNameOrId("agent_static_tokens")
		if coll != nil {
			return app.Delete(coll)
		}
		return nil
	})
}
