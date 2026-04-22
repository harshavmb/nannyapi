// Package reaper periodically scans tracked operation collections for
// records that have been stuck in a non-terminal state (pending / running /
// sent / rebooting / in_progress) for longer than a configurable timeout,
// marks them failed, and emits a synthetic realtime event so subscribers
// (agents, frontends) are informed.
//
// This directly addresses the problem that PocketBase's realtime layer is
// fire-and-forget: even when the original "start" event was delivered, the
// agent may have died mid-execution and no completion event will ever
// arrive. Without the reaper, the record sits in "pending" forever and the
// UI shows a perpetual spinner.
//
// Configuration (environment):
//
//	STUCK_OP_TIMEOUT_SECONDS       How long a record can remain in a
//	                                non-terminal state before being
//	                                reaped. Default: 3600 (1h).
//	STUCK_OP_SCAN_INTERVAL_SECONDS How often the reaper scans.
//	                                Default: 300 (5 min).
package reaper

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/nannyagent/nannyapi/internal/realtime"
	"github.com/pocketbase/pocketbase/core"
)

// Config controls the reaper's behavior. Zero values fall back to
// environment variables, and then to the documented defaults.
type Config struct {
	Timeout      time.Duration
	ScanInterval time.Duration
}

// DefaultConfig returns a Config populated from the environment or from the
// built-in defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:      envSeconds("STUCK_OP_TIMEOUT_SECONDS", 3600),
		ScanInterval: envSeconds("STUCK_OP_SCAN_INTERVAL_SECONDS", 300),
	}
}

func envSeconds(key string, def int) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(def) * time.Second
}

// targetSpec describes how the reaper identifies stuck records in a given
// collection and which status / error fields to set when reaping one.
type targetSpec struct {
	collection    string   // collection name
	stuckStatuses []string // statuses considered "stuck"
	terminalFail  string   // status string to transition to
	errorField    string   // field where the timeout message is written
}

func tracked() []targetSpec {
	return []targetSpec{
		{
			collection:    realtime.ResourcePatchOperations,
			stuckStatuses: []string{"pending", "running"},
			terminalFail:  "failed",
			errorField:    "error_msg",
		},
		{
			collection:    realtime.ResourceRebootOperations,
			stuckStatuses: []string{"pending", "sent", "rebooting"},
			terminalFail:  "timeout", // matches existing RebootStatusTimeout
			errorField:    "error_message",
		},
		{
			collection:    realtime.ResourceInvestigations,
			stuckStatuses: []string{"pending", "in_progress"},
			terminalFail:  "failed",
			errorField:    "error",
		},
	}
}

// Run performs a single reap pass. Exported so tests and manual triggers
// (e.g. an admin endpoint) can invoke it deterministically. It never
// returns an error — individual failures are logged and do not interrupt
// the pass.
func Run(app core.App, cfg Config) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3600 * time.Second
	}
	cutoff := time.Now().Add(-cfg.Timeout).UTC()

	for _, t := range tracked() {
		reapCollection(app, t, cutoff, cfg.Timeout)
	}
}

func reapCollection(app core.App, t targetSpec, cutoff time.Time, timeout time.Duration) {
	coll, err := app.FindCollectionByNameOrId(t.collection)
	if err != nil {
		app.Logger().Debug("reaper: collection not found; skipping", "collection", t.collection)
		return
	}

	// Build status IN (...) clause defensively — PB's filter DSL accepts
	// repeated "status = X || status = Y".
	if len(t.stuckStatuses) == 0 {
		return
	}
	filter := ""
	params := map[string]any{"cutoff": cutoff}
	for i, s := range t.stuckStatuses {
		key := fmt.Sprintf("s%d", i)
		if i > 0 {
			filter += " || "
		}
		filter += "status = {:" + key + "}"
		params[key] = s
	}
	// Updated older than cutoff, in one of the stuck statuses.
	filter = "(" + filter + ") && updated < {:cutoff}"

	records, err := app.FindRecordsByFilter(coll, filter, "-updated", 200, 0, params)
	if err != nil {
		app.Logger().Warn("reaper: failed to scan", "collection", t.collection, "error", err)
		return
	}
	if len(records) == 0 {
		return
	}
	msg := fmt.Sprintf("operation timed out after %d seconds with no progress", int(timeout.Seconds()))
	for _, r := range records {
		prev := r.GetString("status")
		r.Set("status", t.terminalFail)
		if t.errorField != "" {
			r.Set(t.errorField, msg)
		}
		if err := app.Save(r); err != nil {
			app.Logger().Error("reaper: failed to mark stuck op",
				"collection", t.collection,
				"id", r.Id,
				"error", err)
			continue
		}
		// Save will fire OnRecordAfterUpdateSuccess which both (a) causes
		// PocketBase to broadcast a realtime "update" event to any
		// subscribed clients, and (b) writes a DeliveryLogged outbox
		// entry via realtime.RegisterOutboxHooks. On top of that we add
		// an explicit "reap_failed" outbox entry so the synthetic nature
		// of this transition is unambiguous in the audit trail.
		realtime.LogEvent(
			app,
			t.collection,
			r.Id,
			"reap_failed",
			r.GetString("agent_id"),
			r.GetString("user_id"),
			t.terminalFail,
			realtime.DeliveryReaperFailed,
			map[string]any{
				"previous_status": prev,
				"timeout_seconds": int(timeout.Seconds()),
			},
			msg,
		)
	}
	app.Logger().Info("reaper: reaped stuck operations",
		"collection", t.collection,
		"count", len(records),
		"timeout_seconds", int(timeout.Seconds()),
	)
}

// Register starts the reaper goroutine as part of app startup. The
// goroutine stops when the process exits; PocketBase does not expose a
// lifecycle hook for graceful shutdown of background tasks.
func Register(app core.App) {
	cfg := DefaultConfig()
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		go func() {
			app.Logger().Info("stuck-operation reaper started",
				"timeout_seconds", int(cfg.Timeout.Seconds()),
				"interval_seconds", int(cfg.ScanInterval.Seconds()),
			)
			// Initial delay gives the app a moment to finish wiring up.
			time.Sleep(10 * time.Second)
			Run(app, cfg)
			ticker := time.NewTicker(cfg.ScanInterval)
			defer ticker.Stop()
			for range ticker.C {
				Run(app, cfg)
			}
		}()
		return e.Next()
	})
}
