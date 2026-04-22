package tests

import (
	"fmt"
	"os"
	"testing"
	"time"

	realtimeoutbox "github.com/nannyagent/nannyapi/internal/realtime"
	"github.com/nannyagent/nannyapi/internal/reaper"
	_ "github.com/nannyagent/nannyapi/pb_migrations"
	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
)

// buildReaperApp returns a fresh TestApp with all migrations applied and the
// outbox hooks registered — exactly what the reaper relies on in production.
func buildReaperApp(t *testing.T) *pbtests.TestApp {
	t.Helper()
	app, err := pbtests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RunAllMigrations(); err != nil {
		app.Cleanup()
		t.Fatalf("migrations: %v", err)
	}
	realtimeoutbox.RegisterOutboxHooks(app)
	return app
}

// seedPatchOp inserts a patch_operations row with the provided status and a
// forced `updated` timestamp so the reaper sees it as stale. Returns the id.
func seedPatchOp(t *testing.T, app *pbtests.TestApp, userID, agentID, status string, updated time.Time) string {
	t.Helper()
	coll, err := app.FindCollectionByNameOrId("patch_operations")
	if err != nil {
		t.Fatalf("patch_operations: %v", err)
	}
	r := core.NewRecord(coll)
	r.Set("user_id", userID)
	r.Set("agent_id", agentID)
	r.Set("mode", "dry-run")
	r.Set("status", status)
	r.Set("script_url", "/api/files/x/y/z.sh")
	if err := app.Save(r); err != nil {
		t.Fatalf("save patch op: %v", err)
	}
	// Backdate `updated` so the reaper considers it stuck.
	if _, err := app.DB().NewQuery("UPDATE patch_operations SET updated = {:u} WHERE id = {:id}").
		Bind(map[string]any{"u": updated.UTC().Format("2006-01-02 15:04:05.000Z"), "id": r.Id}).Execute(); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	return r.Id
}

// TestReaperMarksStuckPatchOp verifies that Run() transitions a stuck
// patch_operations record to "failed" and appends a reap_failed outbox row.
func TestReaperMarksStuckPatchOp(t *testing.T) {
	app := buildReaperApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("rp_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "reap-host")

	// Timeout: 60s; updated 2h ago → stuck.
	id := seedPatchOp(t, app, user.Id, agent.Id, "pending", time.Now().Add(-2*time.Hour))

	reaper.Run(app, reaper.Config{Timeout: 60 * time.Second})

	// Fetch and verify status.
	rec, err := app.FindRecordById("patch_operations", id)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got := rec.GetString("status"); got != "failed" {
		t.Fatalf("expected status=failed, got %q", got)
	}
	if rec.GetString("error_msg") == "" {
		t.Fatal("expected error_msg to be populated")
	}

	// Verify a reap_failed outbox row exists.
	outboxRows, err := app.FindRecordsByFilter(
		"realtime_messages",
		"resource_type = {:rt} && resource_id = {:id} && delivery_status = {:ds}",
		"-created", 10, 0,
		map[string]any{"rt": "patch_operations", "id": id, "ds": realtimeoutbox.DeliveryReaperFailed},
	)
	if err != nil {
		t.Fatalf("outbox query: %v", err)
	}
	if len(outboxRows) == 0 {
		t.Fatal("expected at least one reap_failed outbox row")
	}
}

// TestReaperLeavesFreshOpsAlone ensures that recently-updated records stay
// untouched.
func TestReaperLeavesFreshOpsAlone(t *testing.T) {
	app := buildReaperApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("fresh_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "fresh-host")

	// updated 10s ago with 60s timeout → NOT stuck.
	id := seedPatchOp(t, app, user.Id, agent.Id, "pending", time.Now().Add(-10*time.Second))

	reaper.Run(app, reaper.Config{Timeout: 60 * time.Second})

	rec, err := app.FindRecordById("patch_operations", id)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.GetString("status"); got != "pending" {
		t.Fatalf("expected status=pending, got %q", got)
	}
}

// TestOutboxRecordsPatchOpCreate verifies the outbox hook records a
// "logged" entry whenever a tracked record is created.
func TestOutboxRecordsPatchOpCreate(t *testing.T) {
	app := buildReaperApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("ob_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "outbox-host")

	id := seedPatchOp(t, app, user.Id, agent.Id, "pending", time.Now())

	rows, err := app.FindRecordsByFilter(
		"realtime_messages",
		"resource_type = {:rt} && resource_id = {:id} && action = {:a}",
		"-created", 10, 0,
		map[string]any{"rt": "patch_operations", "id": id, "a": "create"},
	)
	if err != nil {
		t.Fatalf("outbox query: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one outbox row for create")
	}
	if got := rows[0].GetString("delivery_status"); got != realtimeoutbox.DeliveryLogged {
		t.Fatalf("expected delivery_status=%q, got %q", realtimeoutbox.DeliveryLogged, got)
	}
	if got := rows[0].GetString("agent_id"); got != agent.Id {
		t.Fatalf("agent_id mismatch: got %q", got)
	}
}

// TestReaperConfigFromEnv verifies environment overrides are honored.
func TestReaperConfigFromEnv(t *testing.T) {
	t.Setenv("STUCK_OP_TIMEOUT_SECONDS", "120")
	t.Setenv("STUCK_OP_SCAN_INTERVAL_SECONDS", "15")
	cfg := reaper.DefaultConfig()
	if cfg.Timeout != 120*time.Second {
		t.Fatalf("expected 120s timeout, got %s", cfg.Timeout)
	}
	if cfg.ScanInterval != 15*time.Second {
		t.Fatalf("expected 15s interval, got %s", cfg.ScanInterval)
	}

	// Invalid values fall back to defaults.
	_ = os.Unsetenv("STUCK_OP_TIMEOUT_SECONDS")
	_ = os.Unsetenv("STUCK_OP_SCAN_INTERVAL_SECONDS")
	t.Setenv("STUCK_OP_TIMEOUT_SECONDS", "not-a-number")
	cfg = reaper.DefaultConfig()
	if cfg.Timeout != 3600*time.Second {
		t.Fatalf("expected default 3600s timeout on invalid env, got %s", cfg.Timeout)
	}
}
