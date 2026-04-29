package pricing

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// Manager handles all pricing and rate-limiting logic.
// All limits are configurable via DB (pricing_config collection) or
// a config file (NANNYAPI_PRICING_CONFIG env var). When self-hosted
// and pricing is disabled, no enforcement occurs.
type Manager struct {
	app     core.App
	mu      sync.RWMutex
	config  *types.PricingConfig
	enabled bool
}

// NewManager creates a new pricing manager.
// It loads configuration from DB or config file on initialization.
func NewManager(app core.App) *Manager {
	m := &Manager{app: app}
	m.loadConfig()
	return m
}

// IsEnabled returns whether pricing enforcement is active
func (m *Manager) IsEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// loadConfig loads pricing config from:
// 1. NANNYAPI_PRICING_CONFIG env var (path to JSON file)
// 2. pricing_config collection in DB
// 3. Defaults to disabled (self-host mode)
func (m *Manager) loadConfig() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check env var for config file path
	configPath := os.Getenv("NANNYAPI_PRICING_CONFIG")
	if configPath != "" {
		if cfg, err := loadConfigFromFile(configPath); err == nil {
			m.config = cfg
			m.enabled = cfg.Enabled
			m.seedConfigToDBIfEmpty(cfg)
			return
		}
	}

	// Try loading from DB
	if cfg, err := m.loadConfigFromDB(); err == nil {
		m.config = cfg
		m.enabled = cfg.Enabled
		return
	}

	// Default: disabled (self-host mode)
	m.config = &types.PricingConfig{Enabled: false}
	m.enabled = false
}

// ReloadConfig reloads the pricing configuration
func (m *Manager) ReloadConfig() {
	m.loadConfig()
}

// GetConfig returns the current pricing config (admin only)
func (m *Manager) GetConfig() *types.PricingConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// GetPublicInfo returns public pricing info (no admin details)
func (m *Manager) GetPublicInfo() *types.PricingPublicInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if !m.enabled || m.config == nil {
		return nil
	}

	info := &types.PricingPublicInfo{}
	for _, tier := range m.config.Tiers {
		info.Tiers = append(info.Tiers, types.PricingPublicTier{
			Name:                      string(tier.Name),
			MaxAgents:                 tier.MaxAgents,
			DailyTokenLimit:           tier.DailyTokenLimit,
			MonthlyTokenLimit:         tier.MonthlyTokenLimit,
			DailyInvestigationLimit:   tier.DailyInvestigationLimit,
			MonthlyInvestigationLimit: tier.MonthlyInvestigationLimit,
			PricePerMonth:             tier.PricePerMonth,
			ContactEmail:              tier.ContactEmail,
		})
	}
	return info
}

// GetUserTier determines the effective tier for a user (checking overrides)
func (m *Manager) GetUserTier(userID string) types.TierType {
	if !m.IsEnabled() {
		return types.TierPro // Self-hosted = unlimited
	}

	// Check for active tier override
	override, err := m.getActiveTierOverride(userID)
	if err == nil && override != nil {
		return override.Tier
	}

	// Check user's tier field in DB
	usersCollection, err := m.app.FindCollectionByNameOrId("users")
	if err != nil {
		return types.TierFree
	}

	user, err := m.app.FindRecordById(usersCollection, userID)
	if err != nil {
		return types.TierFree
	}

	tier := user.GetString("tier")
	if tier == string(types.TierPro) {
		return types.TierPro
	}
	return types.TierFree
}

// GetUserLimits returns the effective limits for a user
func (m *Manager) GetUserLimits(userID string) *types.TierConfig {
	if !m.IsEnabled() {
		return &types.TierConfig{
			Name:                      types.TierPro,
			MaxAgents:                 -1,
			DailyTokenLimit:           -1,
			MonthlyTokenLimit:         -1,
			DailyInvestigationLimit:   -1,
			MonthlyInvestigationLimit: -1,
		}
	}

	// Check for user-specific limit overrides
	userLimits, err := m.getUserLimitOverrides(userID)
	if err == nil && userLimits != nil {
		return userLimits
	}

	// Return tier defaults
	tier := m.GetUserTier(userID)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if cfg, ok := m.config.Tiers[tier]; ok {
		return &cfg
	}

	// Fallback: if config is missing for this tier, return zero limits
	// (admin must configure tiers via config file or DB)
	return &types.TierConfig{
		Name:                      types.TierFree,
		MaxAgents:                 0,
		DailyTokenLimit:           0,
		MonthlyTokenLimit:         0,
		DailyInvestigationLimit:   0,
		MonthlyInvestigationLimit: 0,
	}
}

// CheckAgentLimit checks if user can register more agents
func (m *Manager) CheckAgentLimit(userID string) *types.UsageLimitError {
	if !m.IsEnabled() {
		return nil
	}

	limits := m.GetUserLimits(userID)
	if limits.MaxAgents == -1 {
		return nil // unlimited
	}

	// Count agents belonging to user
	count, err := m.app.CountRecords("agents", dbx.NewExp(
		"user_id = {:userId}",
		dbx.Params{"userId": userID},
	))
	if err != nil {
		return nil
	}

	if int(count) >= limits.MaxAgents {
		return &types.UsageLimitError{
			Code:    "agent_limit_reached",
			Message: fmt.Sprintf("You have reached the maximum number of agents (%d) for your plan. Upgrade to Pro for unlimited agents.", limits.MaxAgents),
			Limit:   int64(limits.MaxAgents),
			Used:    count,
		}
	}

	return nil
}

// CheckTokenLimit checks if user can use more tokens
func (m *Manager) CheckTokenLimit(userID string, tokensToUse int64) *types.UsageLimitError {
	if !m.IsEnabled() {
		return nil
	}

	limits := m.GetUserLimits(userID)
	usage := m.getOrCreateUsage(userID)
	m.resetUsageIfNeeded(usage)

	// Check daily limit
	if limits.DailyTokenLimit != -1 {
		if usage.DailyTokensUsed+tokensToUse > limits.DailyTokenLimit {
			return &types.UsageLimitError{
				Code:     "daily_token_limit_reached",
				Message:  fmt.Sprintf("Daily token limit of %d reached. Resets at midnight.", limits.DailyTokenLimit),
				Limit:    limits.DailyTokenLimit,
				Used:     usage.DailyTokensUsed,
				ResetsAt: nextMidnight().Format(time.RFC3339),
			}
		}
	}

	// Check monthly limit
	if limits.MonthlyTokenLimit != -1 {
		if usage.MonthlyTokensUsed+tokensToUse > limits.MonthlyTokenLimit {
			return &types.UsageLimitError{
				Code:     "monthly_token_limit_reached",
				Message:  fmt.Sprintf("Monthly token limit of %d reached. Resets on the 1st of next month.", limits.MonthlyTokenLimit),
				Limit:    limits.MonthlyTokenLimit,
				Used:     usage.MonthlyTokensUsed,
				ResetsAt: nextMonthStart().Format(time.RFC3339),
			}
		}
	}

	return nil
}

// CheckInvestigationLimit checks if user can create more investigations
func (m *Manager) CheckInvestigationLimit(userID string) *types.UsageLimitError {
	if !m.IsEnabled() {
		return nil
	}

	limits := m.GetUserLimits(userID)
	usage := m.getOrCreateUsage(userID)
	m.resetUsageIfNeeded(usage)

	// Check daily limit
	if limits.DailyInvestigationLimit != -1 {
		if usage.DailyInvestigationsUsed >= limits.DailyInvestigationLimit {
			return &types.UsageLimitError{
				Code:     "daily_investigation_limit_reached",
				Message:  fmt.Sprintf("Daily investigation limit of %d reached. Resets at midnight.", limits.DailyInvestigationLimit),
				Limit:    int64(limits.DailyInvestigationLimit),
				Used:     int64(usage.DailyInvestigationsUsed),
				ResetsAt: nextMidnight().Format(time.RFC3339),
			}
		}
	}

	// Check monthly limit
	if limits.MonthlyInvestigationLimit != -1 {
		if usage.MonthlyInvestigationsUsed >= limits.MonthlyInvestigationLimit {
			return &types.UsageLimitError{
				Code:     "monthly_investigation_limit_reached",
				Message:  fmt.Sprintf("Monthly investigation limit of %d reached. Resets on the 1st of next month.", limits.MonthlyInvestigationLimit),
				Limit:    int64(limits.MonthlyInvestigationLimit),
				Used:     int64(usage.MonthlyInvestigationsUsed),
				ResetsAt: nextMonthStart().Format(time.RFC3339),
			}
		}
	}

	return nil
}

// RecordTokenUsage records token usage for a user
func (m *Manager) RecordTokenUsage(userID string, tokens int64) error {
	if !m.IsEnabled() {
		return nil
	}

	usage := m.getOrCreateUsage(userID)
	m.resetUsageIfNeeded(usage)

	collection, err := m.app.FindCollectionByNameOrId("user_usage")
	if err != nil {
		return err
	}

	record, err := m.app.FindRecordById(collection, usage.ID)
	if err != nil {
		return err
	}

	record.Set("daily_tokens_used", usage.DailyTokensUsed+tokens)
	record.Set("monthly_tokens_used", usage.MonthlyTokensUsed+tokens)

	return m.app.Save(record)
}

// RecordInvestigationUsage records investigation usage for a user
func (m *Manager) RecordInvestigationUsage(userID string) error {
	if !m.IsEnabled() {
		return nil
	}

	usage := m.getOrCreateUsage(userID)
	m.resetUsageIfNeeded(usage)

	collection, err := m.app.FindCollectionByNameOrId("user_usage")
	if err != nil {
		return err
	}

	record, err := m.app.FindRecordById(collection, usage.ID)
	if err != nil {
		return err
	}

	record.Set("daily_investigations_used", usage.DailyInvestigationsUsed+1)
	record.Set("monthly_investigations_used", usage.MonthlyInvestigationsUsed+1)

	return m.app.Save(record)
}

// GetUserUsageInfo returns the public usage info for a user
func (m *Manager) GetUserUsageInfo(userID string) *types.UserTierInfo {
	tier := m.GetUserTier(userID)
	limits := m.GetUserLimits(userID)
	usage := m.getOrCreateUsage(userID)
	m.resetUsageIfNeeded(usage)

	return &types.UserTierInfo{
		Tier:                      tier,
		MaxAgents:                 limits.MaxAgents,
		DailyTokenLimit:           limits.DailyTokenLimit,
		MonthlyTokenLimit:         limits.MonthlyTokenLimit,
		DailyInvestigationLimit:   limits.DailyInvestigationLimit,
		MonthlyInvestigationLimit: limits.MonthlyInvestigationLimit,
		DailyTokensUsed:           usage.DailyTokensUsed,
		MonthlyTokensUsed:         usage.MonthlyTokensUsed,
		DailyInvestigationsUsed:   usage.DailyInvestigationsUsed,
		MonthlyInvestigationsUsed: usage.MonthlyInvestigationsUsed,
		DailyResetsAt:             nextMidnight().Format(time.RFC3339),
		MonthlyResetsAt:           nextMonthStart().Format(time.RFC3339),
	}
}

// PromoteUserTier promotes a user to a tier (optionally for a limited duration)
func (m *Manager) PromoteUserTier(adminID, userID string, tier types.TierType, durationDays int, reason string) error {
	collection, err := m.app.FindCollectionByNameOrId("tier_overrides")
	if err != nil {
		return fmt.Errorf("tier_overrides collection not found: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("tier", string(tier))
	record.Set("granted_by", adminID)
	record.Set("reason", reason)
	record.Set("active", true)

	if durationDays > 0 {
		expiresAt := time.Now().AddDate(0, 0, durationDays)
		record.Set("expires_at", expiresAt)
	}

	if err := m.app.Save(record); err != nil {
		return err
	}

	// Always sync users.tier field so it's visible in the admin dashboard
	user, err := m.app.FindRecordById("users", userID)
	if err == nil {
		user.Set("tier", string(tier))
		_ = m.app.Save(user)
	}

	return nil
}

// UpdateUserLimits allows admin to set custom limits for a specific user
func (m *Manager) UpdateUserLimits(userID string, req types.AdminUpdateLimitsRequest) error {
	collection, err := m.app.FindCollectionByNameOrId("user_limit_overrides")
	if err != nil {
		return fmt.Errorf("user_limit_overrides collection not found: %w", err)
	}

	// Find existing override or create new
	records, err := m.app.FindRecordsByFilter(collection,
		"user_id = {:userId}",
		"", 1, 0,
		map[string]any{"userId": userID})

	var record *core.Record
	if err == nil && len(records) > 0 {
		record = records[0]
	} else {
		record = core.NewRecord(collection)
		record.Set("user_id", userID)
	}

	if req.MaxAgents != nil {
		record.Set("max_agents", *req.MaxAgents)
	}
	if req.DailyTokenLimit != nil {
		record.Set("daily_token_limit", *req.DailyTokenLimit)
	}
	if req.MonthlyTokenLimit != nil {
		record.Set("monthly_token_limit", *req.MonthlyTokenLimit)
	}
	if req.DailyInvestigationLimit != nil {
		record.Set("daily_investigation_limit", *req.DailyInvestigationLimit)
	}
	if req.MonthlyInvestigationLimit != nil {
		record.Set("monthly_investigation_limit", *req.MonthlyInvestigationLimit)
	}

	return m.app.Save(record)
}

// RevokeUserTierOverride deactivates a tier override
func (m *Manager) RevokeUserTierOverride(userID string) error {
	collection, err := m.app.FindCollectionByNameOrId("tier_overrides")
	if err != nil {
		return err
	}

	records, err := m.app.FindRecordsByFilter(collection,
		"user_id = {:userId} && active = true",
		"", 0, 0,
		map[string]any{"userId": userID})
	if err != nil {
		return err
	}

	for _, r := range records {
		r.Set("active", false)
		if err := m.app.Save(r); err != nil {
			return err
		}
	}

	// Reset users.tier field to free
	user, err := m.app.FindRecordById("users", userID)
	if err == nil {
		user.Set("tier", string(types.TierFree))
		_ = m.app.Save(user)
	}

	return nil
}

// SaveConfigToDB persists the pricing config to the database
func (m *Manager) SaveConfigToDB(config *types.PricingConfig) error {
	collection, err := m.app.FindCollectionByNameOrId("pricing_config")
	if err != nil {
		return err
	}

	// Find existing config or create new
	records, err := m.app.FindRecordsByFilter(collection, "1=1", "", 1, 0, nil)

	var record *core.Record
	if err == nil && len(records) > 0 {
		record = records[0]
	} else {
		record = core.NewRecord(collection)
	}

	configJSON, err := json.Marshal(config)
	if err != nil {
		return err
	}

	record.Set("config", string(configJSON))
	record.Set("enabled", config.Enabled)

	if err := m.app.Save(record); err != nil {
		return err
	}

	// Reload in-memory config
	m.mu.Lock()
	m.config = config
	m.enabled = config.Enabled
	m.mu.Unlock()

	return nil
}

// seedConfigToDBIfEmpty saves the config to the DB only if the pricing_config table is empty.
// This ensures admins can manage pricing from the PocketBase dashboard.
// Must be called while holding m.mu.
func (m *Manager) seedConfigToDBIfEmpty(cfg *types.PricingConfig) {
	if m.app.DB() == nil {
		return
	}
	collection, err := m.app.FindCollectionByNameOrId("pricing_config")
	if err != nil {
		return
	}
	records, _ := m.app.FindRecordsByFilter(collection, "1=1", "", 1, 0, nil)
	if len(records) > 0 {
		return // already has data
	}
	// Seed — release lock since SaveConfigToDB acquires it
	m.mu.Unlock()
	_ = m.SaveConfigToDB(cfg)
	m.mu.Lock()
}

// --- internal helpers ---

func (m *Manager) getActiveTierOverride(userID string) (*types.UserTierOverride, error) {
	records, err := m.app.FindRecordsByFilter("tier_overrides",
		"user_id = {:userId} && active = {:active}",
		"", 1, 0,
		map[string]any{"userId": userID, "active": true})
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("no active override")
	}

	record := records[0]

	// Check expiry
	expiresAt := record.GetDateTime("expires_at")
	if !expiresAt.IsZero() && time.Now().After(expiresAt.Time()) {
		// Expired - deactivate and reset users.tier
		record.Set("active", false)
		_ = m.app.Save(record)
		user, userErr := m.app.FindRecordById("users", userID)
		if userErr == nil {
			user.Set("tier", string(types.TierFree))
			_ = m.app.Save(user)
		}
		return nil, fmt.Errorf("override expired")
	}

	tier := types.TierType(record.GetString("tier"))
	return &types.UserTierOverride{
		ID:        record.Id,
		UserID:    userID,
		Tier:      tier,
		GrantedBy: record.GetString("granted_by"),
		Reason:    record.GetString("reason"),
	}, nil
}

func (m *Manager) getUserLimitOverrides(userID string) (*types.TierConfig, error) {
	collection, err := m.app.FindCollectionByNameOrId("user_limit_overrides")
	if err != nil {
		return nil, err
	}

	records, err := m.app.FindRecordsByFilter(collection,
		"user_id = {:userId}",
		"", 1, 0,
		map[string]any{"userId": userID})
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("no overrides")
	}

	record := records[0]

	// Only return if at least one field is set
	maxAgents := record.GetInt("max_agents")
	dailyTokens := record.GetInt("daily_token_limit")
	monthlyTokens := record.GetInt("monthly_token_limit")
	dailyInv := record.GetInt("daily_investigation_limit")
	monthlyInv := record.GetInt("monthly_investigation_limit")

	if maxAgents == 0 && dailyTokens == 0 && monthlyTokens == 0 && dailyInv == 0 && monthlyInv == 0 {
		return nil, fmt.Errorf("no overrides set")
	}

	// Start with tier defaults and override what's set
	tier := m.GetUserTier(userID)
	m.mu.RLock()
	baseCfg, ok := m.config.Tiers[tier]
	m.mu.RUnlock()

	if !ok {
		baseCfg = types.TierConfig{Name: tier}
	}

	if maxAgents != 0 {
		baseCfg.MaxAgents = maxAgents
	}
	if dailyTokens != 0 {
		baseCfg.DailyTokenLimit = int64(dailyTokens)
	}
	if monthlyTokens != 0 {
		baseCfg.MonthlyTokenLimit = int64(monthlyTokens)
	}
	if dailyInv != 0 {
		baseCfg.DailyInvestigationLimit = dailyInv
	}
	if monthlyInv != 0 {
		baseCfg.MonthlyInvestigationLimit = monthlyInv
	}

	return &baseCfg, nil
}

func (m *Manager) getOrCreateUsage(userID string) *types.UserUsage {
	collection, err := m.app.FindCollectionByNameOrId("user_usage")
	if err != nil {
		return &types.UserUsage{UserID: userID, DailyResetAt: nextMidnight(), MonthlyResetAt: nextMonthStart()}
	}

	records, err := m.app.FindRecordsByFilter(collection,
		"user_id = {:userId}",
		"", 1, 0,
		map[string]any{"userId": userID})

	if err == nil && len(records) > 0 {
		record := records[0]
		return &types.UserUsage{
			ID:                        record.Id,
			UserID:                    userID,
			DailyTokensUsed:           int64(record.GetInt("daily_tokens_used")),
			MonthlyTokensUsed:         int64(record.GetInt("monthly_tokens_used")),
			DailyInvestigationsUsed:   record.GetInt("daily_investigations_used"),
			MonthlyInvestigationsUsed: record.GetInt("monthly_investigations_used"),
			DailyResetAt:              record.GetDateTime("daily_reset_at").Time(),
			MonthlyResetAt:            record.GetDateTime("monthly_reset_at").Time(),
		}
	}

	// Create new usage record
	record := core.NewRecord(collection)
	record.Set("user_id", userID)
	record.Set("daily_tokens_used", 0)
	record.Set("monthly_tokens_used", 0)
	record.Set("daily_investigations_used", 0)
	record.Set("monthly_investigations_used", 0)
	record.Set("daily_reset_at", nextMidnight())
	record.Set("monthly_reset_at", nextMonthStart())

	if err := m.app.Save(record); err != nil {
		return &types.UserUsage{UserID: userID, DailyResetAt: nextMidnight(), MonthlyResetAt: nextMonthStart()}
	}

	return &types.UserUsage{
		ID:             record.Id,
		UserID:         userID,
		DailyResetAt:   nextMidnight(),
		MonthlyResetAt: nextMonthStart(),
	}
}

func (m *Manager) resetUsageIfNeeded(usage *types.UserUsage) {
	if usage.ID == "" {
		return
	}

	now := time.Now()
	needsSave := false

	collection, err := m.app.FindCollectionByNameOrId("user_usage")
	if err != nil {
		return
	}

	record, err := m.app.FindRecordById(collection, usage.ID)
	if err != nil {
		return
	}

	// Daily reset (midnight)
	if now.After(usage.DailyResetAt) {
		usage.DailyTokensUsed = 0
		usage.DailyInvestigationsUsed = 0
		usage.DailyResetAt = nextMidnight()
		record.Set("daily_tokens_used", 0)
		record.Set("daily_investigations_used", 0)
		record.Set("daily_reset_at", nextMidnight())
		needsSave = true
	}

	// Monthly reset (1st of month)
	if now.After(usage.MonthlyResetAt) {
		usage.MonthlyTokensUsed = 0
		usage.MonthlyInvestigationsUsed = 0
		usage.MonthlyResetAt = nextMonthStart()
		record.Set("monthly_tokens_used", 0)
		record.Set("monthly_investigations_used", 0)
		record.Set("monthly_reset_at", nextMonthStart())
		needsSave = true
	}

	if needsSave {
		_ = m.app.Save(record)
	}
}

func (m *Manager) loadConfigFromDB() (*types.PricingConfig, error) {
	// Guard against uninitialized DB (e.g. during migrate command)
	if m.app.DB() == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	collection, err := m.app.FindCollectionByNameOrId("pricing_config")
	if err != nil {
		return nil, err
	}

	records, err := m.app.FindRecordsByFilter(collection, "1=1", "", 1, 0, nil)
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("no config found")
	}

	configStr := records[0].GetString("config")
	var config types.PricingConfig
	if err := json.Unmarshal([]byte(configStr), &config); err != nil {
		return nil, err
	}

	return &config, nil
}

func loadConfigFromFile(path string) (*types.PricingConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config types.PricingConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// --- time helpers ---

func nextMidnight() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
}

func nextMonthStart() time.Time {
	now := time.Now()
	if now.Month() == 12 {
		return time.Date(now.Year()+1, 1, 1, 0, 0, 0, 0, now.Location())
	}
	return time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
}
