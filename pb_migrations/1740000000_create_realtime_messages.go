package pb_migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Creates the realtime_messages collection used as an audit/outbox of every
// realtime event emitted for operation records (patch/reboot/investigation).
//
// PocketBase's built-in realtime (SSE) is fire-and-forget: if the subscriber
// (agent or browser) is disconnected at the instant of publish, the message
// is silently dropped. This collection gives us:
//
//   - A persistent trail of every outbound event (for debugging "why didn't
//     my agent receive X?").
//   - A place for the stuck-operation reaper to record synthetic "failed"
//     events when it times an operation out.
//
// Rows are INSERT-only and intentionally small; a trimmer cron can age them
// out if needed.
func init() {
	m.Register(func(app core.App) error {
		if existing, _ := app.FindCollectionByNameOrId("realtime_messages"); existing != nil {
			return nil
		}

		coll := core.NewBaseCollection("realtime_messages")

		// Originating collection name (e.g. "patch_operations").
		coll.Fields.Add(&core.TextField{Name: "resource_type", Required: true, Max: 64})
		// Record id within that collection.
		coll.Fields.Add(&core.TextField{Name: "resource_id", Required: true, Max: 64})
		// "create" | "update" | "reap_failed".
		coll.Fields.Add(&core.TextField{Name: "action", Required: true, Max: 32})
		// Status field of the resource at the time of the event (best effort).
		coll.Fields.Add(&core.TextField{Name: "resource_status", Required: false, Max: 64})
		// Optional agent/user correlation ids (plain text to avoid cross-coll relation weight).
		coll.Fields.Add(&core.TextField{Name: "agent_id", Required: false, Max: 64})
		coll.Fields.Add(&core.TextField{Name: "user_id", Required: false, Max: 64})
		// "logged" (default) | "reaper_failed". Reserved for future delivery ACKs.
		coll.Fields.Add(&core.TextField{Name: "delivery_status", Required: true, Max: 32})
		coll.Fields.Add(&core.TextField{Name: "error", Required: false, Max: 500})
		// JSON blob with any extra context.
		coll.Fields.Add(&core.JSONField{Name: "payload", Required: false, MaxSize: 100000})

		coll.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
		coll.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})

		coll.AddIndex("idx_realtime_messages_resource", false, "resource_type, resource_id", "")
		coll.AddIndex("idx_realtime_messages_agent", false, "agent_id", "")
		coll.AddIndex("idx_realtime_messages_created", false, "created", "")

		// Only superusers (and backend code via app.Save) can read this
		// collection. Leaving list/view rules nil (= disabled for
		// end-users) is intentional.

		return app.Save(coll)
	}, func(app core.App) error {
		coll, _ := app.FindCollectionByNameOrId("realtime_messages")
		if coll != nil {
			return app.Delete(coll)
		}
		return nil
	})
}
