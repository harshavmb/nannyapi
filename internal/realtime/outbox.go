// Package realtime provides an audit-log ("outbox") for realtime events
// broadcast via PocketBase's SSE mechanism, plus a small helper API used by
// other packages (e.g. the stuck-operation reaper) to record synthetic
// events that did not originate from a record CRUD call.
//
// # Motivation
//
// PocketBase's built-in realtime publish path is fire-and-forget: the
// event is handed to each subscriber's buffered channel in a goroutine;
// there is no persistence, no retry, no delivery ACK. When an agent's SSE
// connection is briefly interrupted — a common occurrence on flaky
// networks — the message is lost with zero trace on the server, which is
// exactly the "black hole" the user asked us to address.
//
// This package does NOT replace PocketBase's realtime broadcaster. Instead
// it writes a row to the `realtime_messages` collection every time one of
// the tracked operation records (patch_operations, reboot_operations,
// investigations) is created or updated, giving us a durable audit trail
// that operators can query to answer "did the server attempt to tell the
// agent?" The actual SSE broadcast still happens via PocketBase's
// OnModelAfterCreateSuccess/UpdateSuccess hooks.
package realtime

import (
	"encoding/json"

	"github.com/pocketbase/pocketbase/core"
)

// Tracked resource (collection) names.
const (
	ResourcePatchOperations  = "patch_operations"
	ResourceRebootOperations = "reboot_operations"
	ResourceInvestigations   = "investigations"
)

// Delivery statuses recorded in the outbox.
const (
	DeliveryLogged       = "logged"        // standard CRUD-derived event
	DeliveryReaperFailed = "reaper_failed" // synthetic event emitted by the reaper
)

// trackedResources returns the set of collections whose create/update events
// we persist to the outbox.
func trackedResources() map[string]struct{} {
	return map[string]struct{}{
		ResourcePatchOperations:  {},
		ResourceRebootOperations: {},
		ResourceInvestigations:   {},
	}
}

// LogEvent persists a single outbox entry. Errors are logged but never
// returned to the caller so that an outbox failure can never break the
// originating write path.
func LogEvent(
	app core.App,
	resourceType string,
	resourceID string,
	action string,
	agentID string,
	userID string,
	resourceStatus string,
	deliveryStatus string,
	payload map[string]any,
	errMsg string,
) {
	coll, err := app.FindCollectionByNameOrId("realtime_messages")
	if err != nil {
		// Migration not applied yet (unit tests, fresh install) — noop.
		app.Logger().Debug("realtime outbox collection unavailable", "error", err)
		return
	}

	rec := core.NewRecord(coll)
	rec.Set("resource_type", resourceType)
	rec.Set("resource_id", resourceID)
	rec.Set("action", action)
	rec.Set("resource_status", resourceStatus)
	rec.Set("agent_id", agentID)
	rec.Set("user_id", userID)
	rec.Set("delivery_status", deliveryStatus)
	if errMsg != "" {
		// Trim to the column max to avoid noisy save failures on very long
		// stack traces.
		if len(errMsg) > 500 {
			errMsg = errMsg[:500]
		}
		rec.Set("error", errMsg)
	}
	if payload != nil {
		if b, merr := json.Marshal(payload); merr == nil {
			rec.Set("payload", string(b))
		}
	}

	if serr := app.Save(rec); serr != nil {
		app.Logger().Warn("failed to persist realtime outbox entry",
			"resource_type", resourceType,
			"resource_id", resourceID,
			"error", serr)
		return
	}

	// Additionally surface the event in the structured log so that
	// operators tailing the server log can see at a glance that an event
	// was emitted.
	app.Logger().Info("realtime event",
		"resource_type", resourceType,
		"resource_id", resourceID,
		"action", action,
		"status", resourceStatus,
		"delivery", deliveryStatus,
		"agent_id", agentID,
		"user_id", userID,
	)
}

// RegisterOutboxHooks binds after-create and after-update hooks for the
// tracked collections so that every change is mirrored into the outbox.
// Safe to call once on application startup.
func RegisterOutboxHooks(app core.App) {
	logFromRecord := func(action string) func(e *core.RecordEvent) error {
		return func(e *core.RecordEvent) error {
			coll := e.Record.Collection()
			if coll == nil {
				return e.Next()
			}
			if _, ok := trackedResources()[coll.Name]; !ok {
				return e.Next()
			}
			LogEvent(
				app,
				coll.Name,
				e.Record.Id,
				action,
				e.Record.GetString("agent_id"),
				e.Record.GetString("user_id"),
				e.Record.GetString("status"),
				DeliveryLogged,
				nil,
				"",
			)
			return e.Next()
		}
	}

	for name := range trackedResources() {
		app.OnRecordAfterCreateSuccess(name).BindFunc(logFromRecord("create"))
		app.OnRecordAfterUpdateSuccess(name).BindFunc(logFromRecord("update"))
	}
}
