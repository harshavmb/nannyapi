package tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nannyagent/nannyapi/internal/hooks"
	"github.com/nannyagent/nannyapi/internal/pricing"
	"github.com/nannyagent/nannyapi/internal/types"
	_ "github.com/nannyagent/nannyapi/pb_migrations"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

// setupPricingTestApp creates test app with pricing enabled
func setupPricingTestApp(t *testing.T) (*tests.TestApp, *pricing.Manager) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Register hooks
	app.OnRecordCreate("users").BindFunc(hooks.OnUserCreate(app))
	app.OnRecordUpdate("users").BindFunc(hooks.OnUserUpdate(app))

	// Create pricing config in DB
	enablePricingConfig(app, t)

	mgr := pricing.NewManager(app)

	// Register pricing hooks (including default tier on user create)
	hooks.RegisterPricingHooks(app, mgr)

	return app, mgr
}

// enablePricingConfig inserts pricing config into DB
func enablePricingConfig(app *tests.TestApp, t *testing.T) {
	collection, err := app.FindCollectionByNameOrId("pricing_config")
	if err != nil {
		t.Fatalf("pricing_config collection not found: %v", err)
	}

	config := types.PricingConfig{
		Enabled: true,
		Tiers: map[types.TierType]types.TierConfig{
			types.TierFree: {
				Name:                      types.TierFree,
				MaxAgents:                 2,
				DailyTokenLimit:           200000,
				MonthlyTokenLimit:         1000000,
				DailyInvestigationLimit:   5,
				MonthlyInvestigationLimit: 25,
				PricePerMonth:             0,
				ContactEmail:              "support@nannyai.dev",
			},
			types.TierPro: {
				Name:                      types.TierPro,
				MaxAgents:                 -1,
				DailyTokenLimit:           -1,
				MonthlyTokenLimit:         10000000,
				DailyInvestigationLimit:   -1,
				MonthlyInvestigationLimit: -1,
				PricePerMonth:             10,
				ContactEmail:              "support@nannyai.dev",
			},
		},
	}

	configJSON, _ := json.Marshal(config)

	record := core.NewRecord(collection)
	record.Set("config", string(configJSON))
	record.Set("enabled", true)

	if err := app.Save(record); err != nil {
		t.Fatalf("Failed to save pricing config: %v", err)
	}
}

// TestPricingDisabledByDefault tests that pricing is off when no config exists
func TestPricingDisabledByDefault(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	mgr := pricing.NewManager(app)

	if mgr.IsEnabled() {
		t.Error("Pricing should be disabled by default (self-host mode)")
	}
}

// TestPricingEnabledFromDB tests config loading from database
func TestPricingEnabledFromDB(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	if !mgr.IsEnabled() {
		t.Error("Pricing should be enabled after config is loaded")
	}

	config := mgr.GetConfig()
	if config == nil {
		t.Fatal("Config should not be nil")
	}

	if len(config.Tiers) != 2 {
		t.Errorf("Expected 2 tiers, got %d", len(config.Tiers))
	}
}

// TestPricingEnabledFromFile tests config loading from JSON file
func TestPricingEnabledFromFile(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "pricing.json")
	config := types.PricingConfig{
		Enabled: true,
		Tiers: map[types.TierType]types.TierConfig{
			types.TierFree: {
				Name:              types.TierFree,
				MaxAgents:         3,
				DailyTokenLimit:   500000,
				MonthlyTokenLimit: 2000000,
			},
		},
	}
	data, _ := json.Marshal(config)
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NANNYAPI_PRICING_CONFIG", configPath)

	mgr := pricing.NewManager(app)

	if !mgr.IsEnabled() {
		t.Error("Pricing should be enabled from file")
	}

	cfg := mgr.GetConfig()
	freeTier := cfg.Tiers[types.TierFree]
	if freeTier.MaxAgents != 3 {
		t.Errorf("Expected max_agents=3 from file config, got %d", freeTier.MaxAgents)
	}
}

// TestFreeUserAgentLimit tests that free users cannot exceed agent limit
func TestFreeUserAgentLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "free-agent-limit@test.com", "Password123!@#")

	// First two agents should succeed
	a1 := createTestAgent(app, t, user.Id, "agent-1")
	a1.Set("status", "active")
	_ = app.Save(a1)

	a2 := createTestAgent(app, t, user.Id, "agent-2")
	a2.Set("status", "active")
	_ = app.Save(a2)

	// Third agent should be blocked
	limitErr := mgr.CheckAgentLimit(user.Id)
	if limitErr == nil {
		t.Fatal("Expected agent limit error for free user with 2 agents")
	}

	if limitErr.Code != "agent_limit_reached" {
		t.Errorf("Expected code 'agent_limit_reached', got '%s'", limitErr.Code)
	}

	if limitErr.Limit != 2 {
		t.Errorf("Expected limit=2, got %d", limitErr.Limit)
	}

	if limitErr.Used != 2 {
		t.Errorf("Expected used=2, got %d", limitErr.Used)
	}
}

// TestProUserUnlimitedAgents tests that pro users have no agent limit
func TestProUserUnlimitedAgents(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "pro-unlimited@test.com", "Password123!@#")

	// Set user to pro tier
	user.Set("tier", "pro")
	if err := app.Save(user); err != nil {
		t.Fatalf("Failed to update user tier: %v", err)
	}

	// Create several agents
	for i := 0; i < 5; i++ {
		a := createTestAgent(app, t, user.Id, fmt.Sprintf("agent-pro-%d", i))
		a.Set("status", "active")
		_ = app.Save(a)
	}

	// Should not be limited
	limitErr := mgr.CheckAgentLimit(user.Id)
	if limitErr != nil {
		t.Errorf("Pro user should not have agent limit, got: %s", limitErr.Message)
	}
}

// TestFreeUserDailyTokenLimit tests daily token limit enforcement
func TestFreeUserDailyTokenLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "token-limit@test.com", "Password123!@#")

	// Should be fine with small usage
	limitErr := mgr.CheckTokenLimit(user.Id, 100000)
	if limitErr != nil {
		t.Errorf("Should allow 100k tokens, got: %s", limitErr.Message)
	}

	// Record 190k tokens
	if err := mgr.RecordTokenUsage(user.Id, 190000); err != nil {
		t.Fatalf("Failed to record token usage: %v", err)
	}

	// Should still allow 5k more (under 200k)
	limitErr = mgr.CheckTokenLimit(user.Id, 5000)
	if limitErr != nil {
		t.Errorf("Should allow 5k tokens when at 190k, got: %s", limitErr.Message)
	}

	// Should not allow 15k more (would exceed 200k)
	limitErr = mgr.CheckTokenLimit(user.Id, 15000)
	if limitErr == nil {
		t.Error("Expected token limit error when exceeding 200k daily")
	}

	if limitErr != nil && limitErr.Code != "daily_token_limit_reached" {
		t.Errorf("Expected code 'daily_token_limit_reached', got '%s'", limitErr.Code)
	}
}

// TestFreeUserMonthlyTokenLimit tests monthly token limit enforcement
func TestFreeUserMonthlyTokenLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "monthly-token@test.com", "Password123!@#")

	// Simulate heavy monthly usage by recording directly in DB
	usageCollection, err := app.FindCollectionByNameOrId("user_usage")
	if err != nil {
		t.Fatalf("user_usage collection not found: %v", err)
	}

	record := core.NewRecord(usageCollection)
	record.Set("user_id", user.Id)
	record.Set("daily_tokens_used", 50000)
	record.Set("monthly_tokens_used", 950000) // Close to 1M limit
	record.Set("daily_investigations_used", 0)
	record.Set("monthly_investigations_used", 0)
	record.Set("daily_reset_at", time.Now().Add(12*time.Hour)) // Not yet reset
	record.Set("monthly_reset_at", time.Now().Add(15*24*time.Hour))

	if err := app.Save(record); err != nil {
		t.Fatalf("Failed to save usage: %v", err)
	}

	// Should allow 40k (under monthly 1M)
	limitErr := mgr.CheckTokenLimit(user.Id, 40000)
	if limitErr != nil {
		t.Errorf("Should allow 40k at 950k monthly, got: %s", limitErr.Message)
	}

	// Should not allow 60k (would exceed 1M monthly)
	limitErr = mgr.CheckTokenLimit(user.Id, 60000)
	if limitErr == nil {
		t.Error("Expected monthly token limit error at 950k + 60k")
	}

	if limitErr != nil && limitErr.Code != "monthly_token_limit_reached" {
		t.Errorf("Expected 'monthly_token_limit_reached', got '%s'", limitErr.Code)
	}
}

// TestFreeUserDailyInvestigationLimit tests daily investigation limit
func TestFreeUserDailyInvestigationLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "inv-limit@test.com", "Password123!@#")

	// Record 5 investigations
	for i := 0; i < 5; i++ {
		if err := mgr.RecordInvestigationUsage(user.Id); err != nil {
			t.Fatalf("Failed to record investigation: %v", err)
		}
	}

	// Sixth should be blocked
	limitErr := mgr.CheckInvestigationLimit(user.Id)
	if limitErr == nil {
		t.Error("Expected investigation limit error after 5 daily investigations")
	}

	if limitErr != nil && limitErr.Code != "daily_investigation_limit_reached" {
		t.Errorf("Expected 'daily_investigation_limit_reached', got '%s'", limitErr.Code)
	}
}

// TestFreeUserMonthlyInvestigationLimit tests monthly investigation limit
func TestFreeUserMonthlyInvestigationLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "monthly-inv@test.com", "Password123!@#")

	// Set monthly usage near limit directly
	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	record := core.NewRecord(usageCollection)
	record.Set("user_id", user.Id)
	record.Set("daily_tokens_used", 0)
	record.Set("monthly_tokens_used", 0)
	record.Set("daily_investigations_used", 3)
	record.Set("monthly_investigations_used", 25) // At monthly limit
	record.Set("daily_reset_at", time.Now().Add(12*time.Hour))
	record.Set("monthly_reset_at", time.Now().Add(15*24*time.Hour))

	if err := app.Save(record); err != nil {
		t.Fatalf("Failed to save usage: %v", err)
	}

	limitErr := mgr.CheckInvestigationLimit(user.Id)
	if limitErr == nil {
		t.Error("Expected monthly investigation limit error at 25")
	}

	if limitErr != nil && limitErr.Code != "monthly_investigation_limit_reached" {
		t.Errorf("Expected 'monthly_investigation_limit_reached', got '%s'", limitErr.Code)
	}
}

// TestProUserNoInvestigationLimit tests pro users have no investigation limits
func TestProUserNoInvestigationLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "pro-inv@test.com", "Password123!@#")
	user.Set("tier", "pro")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	// Record many investigations
	for i := 0; i < 20; i++ {
		_ = mgr.RecordInvestigationUsage(user.Id)
	}

	// Should never be limited
	limitErr := mgr.CheckInvestigationLimit(user.Id)
	if limitErr != nil {
		t.Errorf("Pro user should not have investigation limit, got: %s", limitErr.Message)
	}
}

// TestProUserMonthlyTokenLimit tests pro user monthly token limit (10M)
func TestProUserMonthlyTokenLimit(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "pro-token@test.com", "Password123!@#")
	user.Set("tier", "pro")
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	// Set monthly near 10M
	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	record := core.NewRecord(usageCollection)
	record.Set("user_id", user.Id)
	record.Set("daily_tokens_used", 500000)
	record.Set("monthly_tokens_used", 9500000)
	record.Set("daily_investigations_used", 0)
	record.Set("monthly_investigations_used", 0)
	record.Set("daily_reset_at", time.Now().Add(12*time.Hour))
	record.Set("monthly_reset_at", time.Now().Add(15*24*time.Hour))

	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}

	// Should allow 400k (under 10M)
	limitErr := mgr.CheckTokenLimit(user.Id, 400000)
	if limitErr != nil {
		t.Errorf("Pro user should allow 400k at 9.5M, got: %s", limitErr.Message)
	}

	// Should not allow 600k (would exceed 10M)
	limitErr = mgr.CheckTokenLimit(user.Id, 600000)
	if limitErr == nil {
		t.Error("Expected monthly token limit at 9.5M + 600k for pro user")
	}
}

// TestAdminPromoteUserToPro tests admin promoting a user to pro tier
func TestAdminPromoteUserToPro(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "promote@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin@test.com", "Password123!@#")

	// User starts as free
	tier := mgr.GetUserTier(user.Id)
	if tier != types.TierFree {
		t.Errorf("Expected free tier, got %s", tier)
	}

	// Promote to pro
	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 0, "testing")
	if err != nil {
		t.Fatalf("Failed to promote user: %v", err)
	}

	// Should now be pro
	tier = mgr.GetUserTier(user.Id)
	if tier != types.TierPro {
		t.Errorf("Expected pro tier after promotion, got %s", tier)
	}
}

// TestAdminPromoteUserTrialExpiry tests that trial promotions expire
func TestAdminPromoteUserTrialExpiry(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "trial@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-trial@test.com", "Password123!@#")

	// Promote for 7 days
	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 7, "7-day trial")
	if err != nil {
		t.Fatalf("Failed to promote: %v", err)
	}

	// Should be pro
	tier := mgr.GetUserTier(user.Id)
	if tier != types.TierPro {
		t.Errorf("Expected pro tier during trial, got %s", tier)
	}

	// Manually expire the override
	overridesCollection, _ := app.FindCollectionByNameOrId("tier_overrides")
	records, _ := app.FindRecordsByFilter(overridesCollection,
		"user_id = {:userId}", "", 1, 0,
		map[string]any{"userId": user.Id})

	if len(records) > 0 {
		records[0].Set("expires_at", time.Now().Add(-1*time.Hour)) // Already expired
		_ = app.Save(records[0])
	}

	// Should revert to free
	tier = mgr.GetUserTier(user.Id)
	if tier != types.TierFree {
		t.Errorf("Expected free tier after trial expiry, got %s", tier)
	}
}

// TestAdminRevokeOverride tests revoking a tier override
func TestAdminRevokeOverride(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "revoke@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-revoke@test.com", "Password123!@#")

	// Promote then revoke
	_ = mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 0, "test")

	tier := mgr.GetUserTier(user.Id)
	if tier != types.TierPro {
		t.Errorf("Expected pro, got %s", tier)
	}

	err := mgr.RevokeUserTierOverride(user.Id)
	if err != nil {
		t.Fatalf("Failed to revoke: %v", err)
	}

	tier = mgr.GetUserTier(user.Id)
	if tier != types.TierFree {
		t.Errorf("Expected free after revoke, got %s", tier)
	}
}

// TestAdminCustomLimits tests custom per-user limit overrides
func TestAdminCustomLimits(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "custom-limits@test.com", "Password123!@#")

	// Default free limits
	limits := mgr.GetUserLimits(user.Id)
	if limits.MaxAgents != 2 {
		t.Errorf("Expected default max_agents=2, got %d", limits.MaxAgents)
	}

	// Admin sets custom limit
	maxAgents := 5
	err := mgr.UpdateUserLimits(user.Id, types.AdminUpdateLimitsRequest{
		UserID:    user.Id,
		MaxAgents: &maxAgents,
	})
	if err != nil {
		t.Fatalf("Failed to update limits: %v", err)
	}

	// Should now have custom limit
	limits = mgr.GetUserLimits(user.Id)
	if limits.MaxAgents != 5 {
		t.Errorf("Expected custom max_agents=5, got %d", limits.MaxAgents)
	}
}

// TestUsageResetMidnight tests that daily usage resets at midnight
func TestUsageResetMidnight(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "reset-daily@test.com", "Password123!@#")

	// Record usage
	_ = mgr.RecordTokenUsage(user.Id, 100000)
	_ = mgr.RecordInvestigationUsage(user.Id)

	// Set reset time to the past to simulate midnight passing
	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	records, _ := app.FindRecordsByFilter(usageCollection,
		"user_id = {:userId}", "", 1, 0,
		map[string]any{"userId": user.Id})

	if len(records) > 0 {
		records[0].Set("daily_reset_at", time.Now().Add(-1*time.Hour))
		_ = app.Save(records[0])
	}

	// Check usage - should be reset
	info := mgr.GetUserUsageInfo(user.Id)
	if info.DailyTokensUsed != 0 {
		t.Errorf("Expected daily tokens reset to 0, got %d", info.DailyTokensUsed)
	}
	if info.DailyInvestigationsUsed != 0 {
		t.Errorf("Expected daily investigations reset to 0, got %d", info.DailyInvestigationsUsed)
	}

	// Monthly should NOT be reset (only daily was expired)
	if info.MonthlyTokensUsed == 0 {
		t.Error("Expected monthly tokens to NOT be reset when only daily expired")
	}
}

// TestUsageResetMonthly tests that monthly usage resets on 1st
func TestUsageResetMonthly(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "reset-monthly@test.com", "Password123!@#")

	// Record usage
	_ = mgr.RecordTokenUsage(user.Id, 500000)

	// Set monthly reset time to the past
	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	records, _ := app.FindRecordsByFilter(usageCollection,
		"user_id = {:userId}", "", 1, 0,
		map[string]any{"userId": user.Id})

	if len(records) > 0 {
		records[0].Set("monthly_reset_at", time.Now().Add(-1*time.Hour))
		_ = app.Save(records[0])
	}

	// Check usage - should be reset
	info := mgr.GetUserUsageInfo(user.Id)
	if info.MonthlyTokensUsed != 0 {
		t.Errorf("Expected monthly tokens reset to 0, got %d", info.MonthlyTokensUsed)
	}
}

// TestGetUserUsageInfo tests the public usage info response
func TestGetUserUsageInfo(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "usage-info@test.com", "Password123!@#")

	// Record some usage
	_ = mgr.RecordTokenUsage(user.Id, 50000)
	_ = mgr.RecordInvestigationUsage(user.Id)

	info := mgr.GetUserUsageInfo(user.Id)

	if info.Tier != types.TierFree {
		t.Errorf("Expected free tier, got %s", info.Tier)
	}
	if info.MaxAgents != 2 {
		t.Errorf("Expected max_agents=2, got %d", info.MaxAgents)
	}
	if info.DailyTokensUsed != 50000 {
		t.Errorf("Expected daily_tokens_used=50000, got %d", info.DailyTokensUsed)
	}
	if info.DailyInvestigationsUsed != 1 {
		t.Errorf("Expected daily_investigations_used=1, got %d", info.DailyInvestigationsUsed)
	}
	if info.DailyResetsAt == "" {
		t.Error("Expected daily_resets_at to be set")
	}
	if info.MonthlyResetsAt == "" {
		t.Error("Expected monthly_resets_at to be set")
	}
}

// TestPublicPricingInfo tests that public info doesn't leak admin details
func TestPublicPricingInfo(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	info := mgr.GetPublicInfo()
	if info == nil {
		t.Fatal("Expected public pricing info")
	}

	if len(info.Tiers) != 2 {
		t.Errorf("Expected 2 public tiers, got %d", len(info.Tiers))
	}

	// Verify no admin details leaked (struct only has public fields)
	for _, tier := range info.Tiers {
		if tier.Name == "" {
			t.Error("Expected tier name to be present")
		}
	}
}

// TestSaveAndReloadConfig tests persisting config to DB
func TestSaveAndReloadConfig(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	// Update config
	newConfig := &types.PricingConfig{
		Enabled: true,
		Tiers: map[types.TierType]types.TierConfig{
			types.TierFree: {
				Name:              types.TierFree,
				MaxAgents:         3, // Changed from 2
				DailyTokenLimit:   300000,
				MonthlyTokenLimit: 1500000,
			},
			types.TierPro: {
				Name:              types.TierPro,
				MaxAgents:         -1,
				DailyTokenLimit:   -1,
				MonthlyTokenLimit: 15000000,
			},
		},
	}

	err := mgr.SaveConfigToDB(newConfig)
	if err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Reload and verify
	mgr.ReloadConfig()
	cfg := mgr.GetConfig()

	if cfg.Tiers[types.TierFree].MaxAgents != 3 {
		t.Errorf("Expected max_agents=3 after save, got %d", cfg.Tiers[types.TierFree].MaxAgents)
	}
}

// TestSelfHostedMode tests that self-hosted deployments have no limits
func TestSelfHostedMode(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := app.RunAllMigrations(); err != nil {
		t.Fatal(err)
	}

	// No pricing config = self-hosted mode
	mgr := pricing.NewManager(app)

	if mgr.IsEnabled() {
		t.Error("Should be disabled in self-hosted mode")
	}

	user := createTestUser(app, t, "selfhost@test.com", "Password123!@#")

	// All checks should pass
	if limitErr := mgr.CheckAgentLimit(user.Id); limitErr != nil {
		t.Error("Self-hosted should have no agent limits")
	}
	if limitErr := mgr.CheckTokenLimit(user.Id, 999999999); limitErr != nil {
		t.Error("Self-hosted should have no token limits")
	}
	if limitErr := mgr.CheckInvestigationLimit(user.Id); limitErr != nil {
		t.Error("Self-hosted should have no investigation limits")
	}
}

// TestErrorMessagesAreDescriptive tests that error messages are user-friendly
func TestErrorMessagesAreDescriptive(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "error-msg@test.com", "Password123!@#")

	// Hit agent limit
	ea1 := createTestAgent(app, t, user.Id, "err-agent-1")
	ea1.Set("status", "active")
	_ = app.Save(ea1)

	ea2 := createTestAgent(app, t, user.Id, "err-agent-2")
	ea2.Set("status", "active")
	_ = app.Save(ea2)

	limitErr := mgr.CheckAgentLimit(user.Id)
	if limitErr == nil {
		t.Fatal("Expected error")
	}

	// Should mention upgrade path
	if limitErr.Message == "" {
		t.Error("Error message should not be empty")
	}

	// Should have a machine-readable code
	if limitErr.Code == "" {
		t.Error("Error code should not be empty")
	}

	// Token limit error
	_ = mgr.RecordTokenUsage(user.Id, 195000)
	tokenErr := mgr.CheckTokenLimit(user.Id, 10000)
	if tokenErr == nil {
		t.Fatal("Expected token limit error")
	}

	if tokenErr.ResetsAt == "" {
		t.Error("Token limit error should include reset time")
	}
}

// TestUserTierFieldDefaultsFree verifies new users get tier="free" by default
func TestUserTierFieldDefaultsFree(t *testing.T) {
	app, _ := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "default-tier@test.com", "Password123!@#")

	// Reload from DB to confirm persisted value
	freshUser, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatalf("Failed to reload user: %v", err)
	}

	tier := freshUser.GetString("tier")
	if tier != "free" {
		t.Errorf("Expected new user tier='free', got '%s'", tier)
	}
}

// TestUserTierFieldSyncsOnPromote verifies users.tier updates when promoted
func TestUserTierFieldSyncsOnPromote(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "promote-sync@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-sync@test.com", "Password123!@#")

	// Verify starts at free
	freshUser, _ := app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "free" {
		t.Fatalf("Expected initial tier='free', got '%s'", freshUser.GetString("tier"))
	}

	// Promote to pro (permanent)
	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 0, "permanent upgrade")
	if err != nil {
		t.Fatalf("PromoteUserTier failed: %v", err)
	}

	// Verify users.tier is now "pro"
	freshUser, _ = app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "pro" {
		t.Errorf("Expected tier='pro' after permanent promotion, got '%s'", freshUser.GetString("tier"))
	}

	// Promote with trial (7 days) should also set tier
	user2 := createTestUser(app, t, "trial-sync@test.com", "Password123!@#")
	err = mgr.PromoteUserTier(admin.Id, user2.Id, types.TierPro, 7, "7-day trial")
	if err != nil {
		t.Fatalf("PromoteUserTier (trial) failed: %v", err)
	}

	freshUser2, _ := app.FindRecordById("users", user2.Id)
	if freshUser2.GetString("tier") != "pro" {
		t.Errorf("Expected tier='pro' during trial, got '%s'", freshUser2.GetString("tier"))
	}
}

// TestUserTierFieldSyncsOnRevoke verifies users.tier resets to "free" when revoked
func TestUserTierFieldSyncsOnRevoke(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "revoke-sync@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-revoke-sync@test.com", "Password123!@#")

	// Promote then revoke
	_ = mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 0, "test")

	freshUser, _ := app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "pro" {
		t.Fatalf("Expected tier='pro' after promotion, got '%s'", freshUser.GetString("tier"))
	}

	err := mgr.RevokeUserTierOverride(user.Id)
	if err != nil {
		t.Fatalf("RevokeUserTierOverride failed: %v", err)
	}

	freshUser, _ = app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "free" {
		t.Errorf("Expected tier='free' after revoke, got '%s'", freshUser.GetString("tier"))
	}
}

// TestUserTierFieldSyncsOnTrialExpiry verifies users.tier resets when trial override expires
func TestUserTierFieldSyncsOnTrialExpiry(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "expiry-sync@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-expiry-sync@test.com", "Password123!@#")

	// Promote with trial
	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 7, "trial")
	if err != nil {
		t.Fatalf("PromoteUserTier failed: %v", err)
	}

	// Verify tier is pro
	freshUser, _ := app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "pro" {
		t.Fatalf("Expected tier='pro' during trial, got '%s'", freshUser.GetString("tier"))
	}

	// Manually expire the override
	overrides, _ := app.FindRecordsByFilter("tier_overrides",
		"user_id = {:userId} && active = true",
		"", 1, 0,
		map[string]any{"userId": user.Id})

	if len(overrides) == 0 {
		t.Fatal("No active override found")
	}
	overrides[0].Set("expires_at", time.Now().Add(-1*time.Hour))
	_ = app.Save(overrides[0])

	// Trigger tier resolution (which detects expiry and resets users.tier)
	tier := mgr.GetUserTier(user.Id)
	if tier != types.TierFree {
		t.Errorf("Expected GetUserTier='free' after expiry, got '%s'", tier)
	}

	// Verify the users.tier field was also reset
	freshUser, _ = app.FindRecordById("users", user.Id)
	if freshUser.GetString("tier") != "free" {
		t.Errorf("Expected users.tier='free' after trial expiry, got '%s'", freshUser.GetString("tier"))
	}
}

// TestUserTierFieldVisibleInDashboard verifies the tier field is queryable via filter
func TestUserTierFieldVisibleInDashboard(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	freeUser := createTestUser(app, t, "free-visible@test.com", "Password123!@#")
	proUser := createTestUser(app, t, "pro-visible@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-visible@test.com", "Password123!@#")

	_ = mgr.PromoteUserTier(admin.Id, proUser.Id, types.TierPro, 0, "upgrade")

	// Query free users
	freeUsers, err := app.FindRecordsByFilter("users",
		"tier = {:tier}", "", 0, 0,
		map[string]any{"tier": "free"})
	if err != nil {
		t.Fatalf("Failed to query free users: %v", err)
	}

	foundFree := false
	for _, u := range freeUsers {
		if u.Id == freeUser.Id {
			foundFree = true
		}
	}
	if !foundFree {
		t.Error("Free user not found when filtering by tier='free'")
	}

	// Query pro users
	proUsers, err := app.FindRecordsByFilter("users",
		"tier = {:tier}", "", 0, 0,
		map[string]any{"tier": "pro"})
	if err != nil {
		t.Fatalf("Failed to query pro users: %v", err)
	}

	foundPro := false
	for _, u := range proUsers {
		if u.Id == proUser.Id {
			foundPro = true
		}
	}
	if !foundPro {
		t.Error("Pro user not found when filtering by tier='pro'")
	}
}

// TestLimitOverrideZeroIsValid verifies that setting a limit to 0 actually blocks the resource
// (0 means "block" not "use tier default"; -1 means "use tier default")
func TestLimitOverrideZeroIsValid(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "zero-limit@test.com", "Password123!@#")

	// Set max_agents to 0 (block agent registration)
	intZero := 0
	err := mgr.UpdateUserLimits(user.Id, types.AdminUpdateLimitsRequest{
		UserID:    user.Id,
		MaxAgents: &intZero,
	})
	if err != nil {
		t.Fatalf("UpdateUserLimits failed: %v", err)
	}

	// Now CheckAgentLimit should fail even with 0 agents
	limitErr := mgr.CheckAgentLimit(user.Id)
	if limitErr == nil {
		t.Error("Expected agent limit error when max_agents=0, got nil")
	}
	if limitErr != nil && limitErr.Limit != 0 {
		t.Errorf("Expected limit=0, got %d", limitErr.Limit)
	}
}

// TestLimitOverrideMinusOneIsNotSet verifies that -1 means "use tier default"
func TestLimitOverrideMinusOneIsNotSet(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "default-limit@test.com", "Password123!@#")

	// Set all fields to -1 (should be treated as "not configured")
	minusOne := -1
	minusOneInt64 := int64(-1)
	err := mgr.UpdateUserLimits(user.Id, types.AdminUpdateLimitsRequest{
		UserID:                    user.Id,
		MaxAgents:                 &minusOne,
		DailyTokenLimit:           &minusOneInt64,
		MonthlyTokenLimit:         &minusOneInt64,
		DailyInvestigationLimit:   &minusOne,
		MonthlyInvestigationLimit: &minusOne,
	})
	if err != nil {
		t.Fatalf("UpdateUserLimits failed: %v", err)
	}

	// Should fall through to tier defaults (free tier: max_agents=2)
	limits := mgr.GetUserLimits(user.Id)
	if limits.MaxAgents != 2 {
		t.Errorf("Expected tier default max_agents=2, got %d", limits.MaxAgents)
	}
}

// TestSingleActiveTierOverride verifies only one active override exists per user after promotion
func TestSingleActiveTierOverride(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "single-override@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-single@test.com", "Password123!@#")

	// Promote twice - second should deactivate the first
	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 7, "first trial")
	if err != nil {
		t.Fatalf("First PromoteUserTier failed: %v", err)
	}

	err = mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 30, "extended trial")
	if err != nil {
		t.Fatalf("Second PromoteUserTier failed: %v", err)
	}

	// Count active overrides for this user
	collection, _ := app.FindCollectionByNameOrId("tier_overrides")
	activeRecords, err := app.FindRecordsByFilter(collection,
		"user_id = {:userId} && active = true",
		"", 0, 0,
		map[string]any{"userId": user.Id})
	if err != nil {
		t.Fatalf("Failed to query active overrides: %v", err)
	}

	if len(activeRecords) != 1 {
		t.Errorf("Expected exactly 1 active override, got %d", len(activeRecords))
	}

	// The active one should be the second (extended trial)
	if len(activeRecords) == 1 {
		reason := activeRecords[0].GetString("reason")
		if reason != "extended trial" {
			t.Errorf("Expected active override reason='extended trial', got '%s'", reason)
		}
	}

	// Verify total records (both old deactivated + new active)
	allRecords, _ := app.FindRecordsByFilter(collection,
		"user_id = {:userId}",
		"", 0, 0,
		map[string]any{"userId": user.Id})
	if len(allRecords) < 2 {
		t.Errorf("Expected at least 2 total override records (audit trail), got %d", len(allRecords))
	}
}

// TestTierOverridesHaveTimestamps verifies tier_overrides records have created/updated fields
func TestTierOverridesHaveTimestamps(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "timestamp-check@test.com", "Password123!@#")
	admin := createTestUser(app, t, "admin-timestamp@test.com", "Password123!@#")

	err := mgr.PromoteUserTier(admin.Id, user.Id, types.TierPro, 14, "timestamp test")
	if err != nil {
		t.Fatalf("PromoteUserTier failed: %v", err)
	}

	collection, _ := app.FindCollectionByNameOrId("tier_overrides")
	records, _ := app.FindRecordsByFilter(collection,
		"user_id = {:userId}",
		"", 1, 0,
		map[string]any{"userId": user.Id})

	if len(records) == 0 {
		t.Fatal("No tier_override record found")
	}

	record := records[0]

	// PocketBase BaseCollection auto-manages 'created' and 'updated' fields
	created := record.GetDateTime("created")
	if created.IsZero() {
		t.Error("Expected 'created' timestamp to be set on tier_overrides record")
	}

	updated := record.GetDateTime("updated")
	if updated.IsZero() {
		t.Error("Expected 'updated' timestamp to be set on tier_overrides record")
	}
}

// TestGetPublicInfoDeterministicOrder verifies tiers are returned in stable order (free, pro)
func TestGetPublicInfoDeterministicOrder(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	// Call multiple times to verify consistency
	for i := 0; i < 10; i++ {
		info := mgr.GetPublicInfo()
		if info == nil {
			t.Fatal("Expected public info")
		}
		if len(info.Tiers) != 2 {
			t.Fatalf("Expected 2 tiers, got %d", len(info.Tiers))
		}
		if info.Tiers[0].Name != "free" {
			t.Errorf("Iteration %d: Expected first tier to be 'free', got '%s'", i, info.Tiers[0].Name)
		}
		if info.Tiers[1].Name != "pro" {
			t.Errorf("Iteration %d: Expected second tier to be 'pro', got '%s'", i, info.Tiers[1].Name)
		}
	}
}

// TestConcurrentUsageCreation verifies no duplicate usage records under concurrent access
func TestConcurrentUsageCreation(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "concurrent-usage@test.com", "Password123!@#")

	// Simulate concurrent access by calling RecordTokenUsage in goroutines
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			done <- mgr.RecordTokenUsage(user.Id, 100)
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("RecordTokenUsage error: %v", err)
		}
	}

	// Verify only one usage record exists
	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	records, _ := app.FindRecordsByFilter(usageCollection,
		"user_id = {:userId}",
		"", 0, 0,
		map[string]any{"userId": user.Id})

	if len(records) != 1 {
		t.Errorf("Expected exactly 1 usage record, got %d (race condition!)", len(records))
	}

	// Total should be 1000 (10 goroutines * 100 tokens)
	if len(records) == 1 {
		total := records[0].GetInt("daily_tokens_used")
		if total != 1000 {
			t.Errorf("Expected daily_tokens_used=1000, got %d (lost writes!)", total)
		}
	}
}

// TestUserUsageUniqueIndex verifies the unique index prevents duplicate user_usage rows
func TestUserUsageUniqueIndex(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "unique-usage@test.com", "Password123!@#")

	// First usage call creates the record
	_ = mgr.RecordTokenUsage(user.Id, 50)

	// Second call should update (not create a new record)
	_ = mgr.RecordTokenUsage(user.Id, 50)

	usageCollection, _ := app.FindCollectionByNameOrId("user_usage")
	records, _ := app.FindRecordsByFilter(usageCollection,
		"user_id = {:userId}",
		"", 0, 0,
		map[string]any{"userId": user.Id})

	if len(records) != 1 {
		t.Errorf("Expected exactly 1 usage record, got %d", len(records))
	}

	if len(records) == 1 {
		daily := records[0].GetInt("daily_tokens_used")
		if daily != 100 {
			t.Errorf("Expected daily_tokens_used=100, got %d", daily)
		}
	}
}

// TestAgentRegistrationBlockedAtLimit tests that the OnRecordCreate hook
// blocks agent creation when the user has reached their agent limit, and
// that the error is a structured ApiError the handler can return as JSON.
func TestAgentRegistrationBlockedAtLimit(t *testing.T) {
	app, _ := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "agent-hook-block@test.com", "Password123!@#")

	// Free tier allows 2 agents. Create 2.
	a1 := createTestAgent(app, t, user.Id, "hook-agent-1")
	a1.Set("status", "active")
	_ = app.Save(a1)

	a2 := createTestAgent(app, t, user.Id, "hook-agent-2")
	a2.Set("status", "active")
	_ = app.Save(a2)

	// Attempt to create a 3rd agent — should be blocked by pricing hook
	agentsCollection, err := app.FindCollectionByNameOrId("agents")
	if err != nil {
		t.Fatalf("agents collection not found: %v", err)
	}

	agentRecord := core.NewRecord(agentsCollection)
	agentRecord.Set("user_id", user.Id)
	agentRecord.Set("hostname", "hook-agent-3")
	agentRecord.Set("os_type", "linux")
	agentRecord.Set("platform_family", "debian")
	agentRecord.Set("version", "1.0.0")
	agentRecord.SetPassword("testpass123")

	saveErr := app.Save(agentRecord)
	if saveErr == nil {
		t.Fatal("Expected save to fail due to agent limit, but it succeeded")
	}

	// The error should be a *router.ApiError with status 429
	var apiErr *router.ApiError
	if !errors.As(saveErr, &apiErr) {
		t.Fatalf("Expected *router.ApiError, got %T: %v", saveErr, saveErr)
	}

	if apiErr.Status != 429 {
		t.Errorf("Expected HTTP 429, got %d", apiErr.Status)
	}

	if apiErr.Message == "" {
		t.Error("Expected non-empty error message")
	}

	// Verify the message mentions the agent limit
	if !strings.Contains(apiErr.Message, "maximum number of agents") {
		t.Errorf("Expected message about agent limit, got: %s", apiErr.Message)
	}
}

// TestAgentRegistrationAllowedAfterPromotion tests that promoting a user to
// pro lifts the agent limit and allows registration.
func TestAgentRegistrationAllowedAfterPromotion(t *testing.T) {
	app, mgr := setupPricingTestApp(t)
	defer app.Cleanup()

	user := createTestUser(app, t, "agent-promo@test.com", "Password123!@#")

	// Fill free tier limit (2 agents)
	a1 := createTestAgent(app, t, user.Id, "promo-agent-1")
	a1.Set("status", "active")
	_ = app.Save(a1)

	a2 := createTestAgent(app, t, user.Id, "promo-agent-2")
	a2.Set("status", "active")
	_ = app.Save(a2)

	// Confirm 3rd agent is blocked
	if limitErr := mgr.CheckAgentLimit(user.Id); limitErr == nil {
		t.Fatal("Expected agent limit error before promotion")
	}

	// Promote user to pro
	adminID := "test-admin-id"
	if err := mgr.PromoteUserTier(adminID, user.Id, types.TierPro, 14, "test promotion"); err != nil {
		t.Fatalf("Failed to promote: %v", err)
	}

	// Now 3rd agent should be allowed
	if limitErr := mgr.CheckAgentLimit(user.Id); limitErr != nil {
		t.Errorf("Pro user should not have agent limit, got: %s", limitErr.Message)
	}

	// Actually create the 3rd agent to verify hook passes
	agentsCollection, _ := app.FindCollectionByNameOrId("agents")
	agentRecord := core.NewRecord(agentsCollection)
	agentRecord.Set("user_id", user.Id)
	agentRecord.Set("hostname", "promo-agent-3")
	agentRecord.Set("os_type", "linux")
	agentRecord.Set("platform_family", "debian")
	agentRecord.Set("version", "1.0.0")
	agentRecord.SetPassword("testpass123")

	if err := app.Save(agentRecord); err != nil {
		t.Errorf("Pro user should be able to create 3rd agent, got error: %v", err)
	}
}
