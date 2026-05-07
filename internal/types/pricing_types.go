package types

import "time"

// TierType represents the pricing tier
type TierType string

const (
	TierFree TierType = "free"
	TierPro  TierType = "pro"
)

// UsagePeriod represents the reset period
type UsagePeriod string

const (
	PeriodDaily   UsagePeriod = "daily"
	PeriodMonthly UsagePeriod = "monthly"
)

// TierConfig holds the configurable limits for a tier
type TierConfig struct {
	Name                      TierType `json:"name"`
	MaxAgents                 int      `json:"max_agents"`                  // -1 = unlimited
	DailyTokenLimit           int64    `json:"daily_token_limit"`           // -1 = unlimited
	MonthlyTokenLimit         int64    `json:"monthly_token_limit"`         // -1 = unlimited
	DailyInvestigationLimit   int      `json:"daily_investigation_limit"`   // -1 = unlimited
	MonthlyInvestigationLimit int      `json:"monthly_investigation_limit"` // -1 = unlimited
	PricePerMonth             float64  `json:"price_per_month"`
	ContactEmail              string   `json:"contact_email"`
}

// PricingConfig holds the full pricing configuration
type PricingConfig struct {
	Enabled            bool                    `json:"enabled"`
	Currency           string                  `json:"currency"`
	CreditBundleTokens int64                   `json:"credit_bundle_tokens"`
	CreditBundlePrice  float64                 `json:"credit_bundle_price"`
	Tiers              map[TierType]TierConfig `json:"tiers"`
}

// UserTierOverride represents an admin override for a specific user
type UserTierOverride struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Tier      TierType   `json:"tier"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // nil = permanent
	Reason    string     `json:"reason,omitempty"`
	GrantedBy string     `json:"granted_by"`
	CreatedAt time.Time  `json:"created_at"`
}

// UserUsage tracks current usage for a user
type UserUsage struct {
	ID                        string    `json:"id"`
	UserID                    string    `json:"user_id"`
	DailyTokensUsed           int64     `json:"daily_tokens_used"`
	MonthlyTokensUsed         int64     `json:"monthly_tokens_used"`
	DailyInvestigationsUsed   int       `json:"daily_investigations_used"`
	MonthlyInvestigationsUsed int       `json:"monthly_investigations_used"`
	DailyResetAt              time.Time `json:"daily_reset_at"`
	MonthlyResetAt            time.Time `json:"monthly_reset_at"`
}

// UsageLimitError represents a limit breach
type UsageLimitError struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Limit    int64  `json:"limit"`
	Used     int64  `json:"used"`
	ResetsAt string `json:"resets_at"`
}

// AdminTierPromotionRequest is for admin to promote a user
type AdminTierPromotionRequest struct {
	UserID       string `json:"user_id"`
	Tier         string `json:"tier"`
	DurationDays int    `json:"duration_days,omitempty"` // 0 = permanent
	Reason       string `json:"reason,omitempty"`
}

// AdminUpdateLimitsRequest is for admin to update limits for a user
type AdminUpdateLimitsRequest struct {
	UserID                    string `json:"user_id"`
	MaxAgents                 *int   `json:"max_agents,omitempty"`
	DailyTokenLimit           *int64 `json:"daily_token_limit,omitempty"`
	MonthlyTokenLimit         *int64 `json:"monthly_token_limit,omitempty"`
	DailyInvestigationLimit   *int   `json:"daily_investigation_limit,omitempty"`
	MonthlyInvestigationLimit *int   `json:"monthly_investigation_limit,omitempty"`
}

// UserTierInfo is the public-facing tier info for a user
type UserTierInfo struct {
	Tier                      TierType `json:"tier"`
	MaxAgents                 int      `json:"max_agents"`
	DailyTokenLimit           int64    `json:"daily_token_limit"`
	MonthlyTokenLimit         int64    `json:"monthly_token_limit"`
	DailyInvestigationLimit   int      `json:"daily_investigation_limit"`
	MonthlyInvestigationLimit int      `json:"monthly_investigation_limit"`
	DailyTokensUsed           int64    `json:"daily_tokens_used"`
	MonthlyTokensUsed         int64    `json:"monthly_tokens_used"`
	DailyInvestigationsUsed   int      `json:"daily_investigations_used"`
	MonthlyInvestigationsUsed int      `json:"monthly_investigations_used"`
	DailyResetsAt             string   `json:"daily_resets_at"`
	MonthlyResetsAt           string   `json:"monthly_resets_at"`
}

// PricingPublicInfo is what's shown publicly on the portal
type PricingPublicInfo struct {
	Tiers []PricingPublicTier `json:"tiers"`
}

// PricingPublicTier is public tier info (no admin details)
type PricingPublicTier struct {
	Name                      string  `json:"name"`
	MaxAgents                 int     `json:"max_agents"`
	DailyTokenLimit           int64   `json:"daily_token_limit"`
	MonthlyTokenLimit         int64   `json:"monthly_token_limit"`
	DailyInvestigationLimit   int     `json:"daily_investigation_limit"`
	MonthlyInvestigationLimit int     `json:"monthly_investigation_limit"`
	PricePerMonth             float64 `json:"price_per_month"`
	ContactEmail              string  `json:"contact_email,omitempty"`
}
