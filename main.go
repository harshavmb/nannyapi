package main

import (
	"fmt"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/spf13/cobra"

	"github.com/nannyagent/nannyapi/internal/hooks"
	"github.com/nannyagent/nannyapi/internal/mfa"
	"github.com/nannyagent/nannyapi/internal/pricing"
	realtimeoutbox "github.com/nannyagent/nannyapi/internal/realtime"
	"github.com/nannyagent/nannyapi/internal/reaper"
	"github.com/nannyagent/nannyapi/internal/schedules"
	_ "github.com/nannyagent/nannyapi/pb_migrations"
)

// Version is set at build time via -ldflags
var Version = "dev"

func main() {
	// Check for --version flag before initializing PocketBase
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-v" {
			fmt.Printf("nannyapi version %s\n", Version)
			return
		}
	}

	app := pocketbase.New()

	// Add version command
	app.RootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version number",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("nannyapi version %s\n", Version)
		},
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: os.Getenv("PB_AUTOMIGRATE") == "true",
	})

	// Register auth hooks
	app.OnRecordCreate("users").BindFunc(hooks.OnUserCreate(app))
	app.OnRecordUpdate("users").BindFunc(hooks.OnUserUpdate(app))
	app.OnRecordAuthWithPasswordRequest("users").BindFunc(hooks.OnAuthWithPasswordRequest(app))

	// Register MFA hooks
	mfaHooks := hooks.NewMFAHooks(app)
	app.OnRecordAuthRequest("users").BindFunc(mfaHooks.OnAuthSuccess)

	// Register MFA routes
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		mfaHandler := mfa.NewHandler(app)
		mfaHandler.RegisterRoutes(e)
		return e.Next()
	})

	// Register agent hooks
	hooks.RegisterAgentHooks(app)

	// Register investigation and patch management hooks
	hooks.RegisterInvestigationHooks(app)
	hooks.RegisterPatchHooks(app)
	hooks.RegisterPackageExceptionHooks(app)

	// Register reboot hooks
	hooks.RegisterRebootHooks(app)

	// Register proxmox hooks
	hooks.RegisterProxmoxHooks(app)

	// Register pricing and rate-limiting
	pricingMgr := pricing.NewManager(app)
	hooks.RegisterPricingHooks(app, pricingMgr)

	// Reload pricing config once the app is fully bootstrapped (DB ready)
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		pricingMgr.ReloadConfig()
		return e.Next()
	})

	// Register schedulers for patch and reboot schedules
	schedules.RegisterScheduler(app)
	schedules.RegisterRebootScheduler(app)

	// Persist an audit trail for every realtime event we emit for tracked
	// operation records. Addresses the "realtime is a black hole" pain.
	realtimeoutbox.RegisterOutboxHooks(app)

	// Reap operations that are stuck in non-terminal states beyond the
	// configured timeout, and emit a synthetic failure event so UIs and
	// agents stop spinning forever.
	reaper.Register(app)

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
