package agents

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"
)

// StaticTokenPrefix is prepended to every generated static token so that
// they are easy to identify both in logs and in the middleware that resolves
// them.
const StaticTokenPrefix = "nsk_"

// generateStaticToken returns a new random static token string of the form
// "nsk_<64 hex chars>" (32 random bytes).
func generateStaticToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return StaticTokenPrefix + hex.EncodeToString(b), nil
}

// IsStaticToken returns true if the given bearer value looks like one of our
// static tokens (by prefix). The prefix check is cheap and allows the
// middleware to skip the regular auth-token lookup when resolving static
// tokens.
func IsStaticToken(token string) bool {
	return strings.HasPrefix(token, StaticTokenPrefix)
}

// tokenInfoFromRecord converts a PocketBase record to the public
// StaticTokenInfo DTO (excludes the token hash).
func tokenInfoFromRecord(r *core.Record) types.StaticTokenInfo {
	var expiresAt *time.Time
	if t := r.GetDateTime("expires_at").Time(); !t.IsZero() {
		expiresAt = &t
	}
	var revokedAt *time.Time
	if t := r.GetDateTime("revoked_at").Time(); !t.IsZero() {
		revokedAt = &t
	}
	var lastUsedAt *time.Time
	if t := r.GetDateTime("last_used_at").Time(); !t.IsZero() {
		lastUsedAt = &t
	}
	return types.StaticTokenInfo{
		ID:          r.Id,
		Name:        r.GetString("name"),
		TokenPrefix: r.GetString("token_prefix"),
		ExpiresAt:   expiresAt,
		Revoked:     r.GetBool("revoked"),
		RevokedAt:   revokedAt,
		LastUsedAt:  lastUsedAt,
		Created:     r.GetDateTime("created").Time(),
	}
}

// HandleCreateStaticToken creates a new long-lived static API token for the
// authenticated user. The plaintext token is returned exactly once.
func HandleCreateStaticToken(app core.App, c *core.RequestEvent) error {
	var req types.CreateStaticTokenRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "name required"})
	}
	if len(name) > 120 {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "name too long (max 120)"})
	}

	if !types.StaticTokenAllowedExpiryDays[req.ExpiresInDays] {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "expires_in_days must be one of 0, 30, 60, 90, 180, 365"})
	}

	authRec := c.Get("authRecord")
	if authRec == nil {
		return c.JSON(http.StatusUnauthorized, types.ErrorResponse{Error: "authentication required"})
	}
	user, ok := authRec.(*core.Record)
	if !ok || user.Collection().Name != "users" {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "users only"})
	}

	token, err := generateStaticToken()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate token"})
	}
	hash := HashToken(token)

	// Display-only prefix: "nsk_" + first 8 hex chars so UI can disambiguate.
	prefix := token[:min(len(token), len(StaticTokenPrefix)+8)]

	collection, err := app.FindCollectionByNameOrId("agent_static_tokens")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "static tokens collection unavailable"})
	}

	record := core.NewRecord(collection)
	record.Set("user_id", user.Id)
	record.Set("name", name)
	record.Set("token_hash", hash)
	record.Set("token_prefix", prefix)
	if req.ExpiresInDays > 0 {
		record.Set("expires_at", time.Now().Add(time.Duration(req.ExpiresInDays)*24*time.Hour))
	}
	record.Set("revoked", false)

	if err := app.Save(record); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create token: " + err.Error()})
	}

	return c.JSON(http.StatusOK, types.CreateStaticTokenResponse{
		Token:     token,
		TokenInfo: tokenInfoFromRecord(record),
	})
}

// HandleListStaticTokens returns all static tokens owned by the authenticated
// user. The plaintext token values are never returned.
func HandleListStaticTokens(app core.App, c *core.RequestEvent) error {
	authRec := c.Get("authRecord")
	if authRec == nil {
		return c.JSON(http.StatusUnauthorized, types.ErrorResponse{Error: "authentication required"})
	}
	user, ok := authRec.(*core.Record)
	if !ok || user.Collection().Name != "users" {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "users only"})
	}

	collection, err := app.FindCollectionByNameOrId("agent_static_tokens")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "static tokens collection unavailable"})
	}

	records, err := app.FindRecordsByFilter(collection, "user_id = {:uid}", "-created", 200, 0, map[string]any{"uid": user.Id})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to list tokens"})
	}

	out := make([]types.StaticTokenInfo, 0, len(records))
	for _, r := range records {
		out = append(out, tokenInfoFromRecord(r))
	}
	return c.JSON(http.StatusOK, types.ListStaticTokensResponse{Tokens: out})
}

// HandleRevokeStaticToken marks a static token as revoked; once revoked it
// can no longer be used to authenticate.
func HandleRevokeStaticToken(app core.App, c *core.RequestEvent) error {
	var req types.RevokeStaticTokenRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}
	if req.TokenID == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "token_id required"})
	}

	authRec := c.Get("authRecord")
	if authRec == nil {
		return c.JSON(http.StatusUnauthorized, types.ErrorResponse{Error: "authentication required"})
	}
	user, ok := authRec.(*core.Record)
	if !ok || user.Collection().Name != "users" {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "users only"})
	}

	collection, err := app.FindCollectionByNameOrId("agent_static_tokens")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "static tokens collection unavailable"})
	}

	record, err := app.FindRecordById(collection, req.TokenID)
	if err != nil {
		return c.JSON(http.StatusNotFound, types.ErrorResponse{Error: "token not found"})
	}
	if record.GetString("user_id") != user.Id {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "not your token"})
	}

	if record.GetBool("revoked") {
		return c.JSON(http.StatusOK, types.RevokeStaticTokenResponse{Success: true, Message: "already revoked"})
	}

	record.Set("revoked", true)
	record.Set("revoked_at", time.Now())
	// Replace the hash with a per-record sentinel so the unique index is
	// preserved but the original hash can no longer be matched.
	record.Set("token_hash", "revoked_"+record.Id)

	if err := app.Save(record); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to revoke token"})
	}

	return c.JSON(http.StatusOK, types.RevokeStaticTokenResponse{Success: true, Message: "token revoked"})
}

// ResolveStaticToken looks up a static token by plaintext value, validating
// expiry and revocation. On success it returns the owning user record.
// The token record is also updated with last_used_at.
func ResolveStaticToken(app core.App, token string) (*core.Record, error) {
	hash := HashToken(token)
	collection, err := app.FindCollectionByNameOrId("agent_static_tokens")
	if err != nil {
		return nil, err
	}

	records, err := app.FindRecordsByFilter(collection, "token_hash = {:hash}", "", 1, 0, map[string]any{"hash": hash})
	if err != nil || len(records) == 0 {
		return nil, errStaticTokenInvalid
	}
	r := records[0]
	if r.GetBool("revoked") {
		return nil, errStaticTokenInvalid
	}
	if t := r.GetDateTime("expires_at").Time(); !t.IsZero() && time.Now().After(t) {
		return nil, errStaticTokenInvalid
	}

	userID := r.GetString("user_id")
	usersColl, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return nil, err
	}
	user, err := app.FindRecordById(usersColl, userID)
	if err != nil {
		return nil, err
	}

	// Best-effort: update last_used_at. Ignore any error so a failure here
	// does not prevent authentication from succeeding.
	r.Set("last_used_at", time.Now())
	_ = app.Save(r)

	return user, nil
}

// HandleRegisterWithStaticToken registers a new agent using a static token.
// Unlike the device-code flow, this does NOT create a device_codes row and
// does NOT populate device_user_code, refresh_token_hash, or
// refresh_token_expires on the agent record.
func HandleRegisterWithStaticToken(app core.App, c *core.RequestEvent) error {
	authRec := c.Get("authRecord")
	if authRec == nil {
		return c.JSON(http.StatusUnauthorized, types.ErrorResponse{Error: "authentication required"})
	}

	// Only static-token auth is accepted for this action.
	if c.Get("authViaStaticToken") == nil || c.Get("authViaStaticToken") != true {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "this action requires a static token"})
	}

	user, ok := authRec.(*core.Record)
	if !ok || user.Collection().Name != "users" {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "users only"})
	}

	var req types.RegisterWithStaticTokenRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	if strings.TrimSpace(req.Hostname) == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "hostname required"})
	}

	if strings.TrimSpace(req.Version) == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "version required"})
	}

	agentsCollection, err := app.FindCollectionByNameOrId("agents")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "agents collection unavailable"})
	}

	agentRecord := core.NewRecord(agentsCollection)
	agentRecord.Set("user_id", user.Id)
	agentRecord.Set("hostname", req.Hostname)
	agentRecord.Set("os_type", req.OSType)
	agentRecord.Set("os_info", req.OSInfo)
	agentRecord.Set("os_version", req.OSVersion)
	agentRecord.Set("version", req.Version)
	agentRecord.Set("status", string(types.AgentStatusActive))
	agentRecord.Set("last_seen", time.Now())
	agentRecord.Set("kernel_version", req.KernelVersion)
	agentRecord.Set("arch", req.Arch)
	agentRecord.Set("auth_method", "static_token")

	// Resolve platform_family
	platformFamily := req.PlatformFamily
	if platformFamily == "" {
		osInfoLower := strings.ToLower(req.OSInfo)
		switch {
		case strings.Contains(osInfoLower, "debian") || strings.Contains(osInfoLower, "ubuntu") || strings.Contains(osInfoLower, "mint") || strings.Contains(osInfoLower, "pop"):
			platformFamily = "debian"
		case strings.Contains(osInfoLower, "red hat") || strings.Contains(osInfoLower, "rhel") || strings.Contains(osInfoLower, "centos") || strings.Contains(osInfoLower, "fedora") || strings.Contains(osInfoLower, "alma") || strings.Contains(osInfoLower, "rocky") || strings.Contains(osInfoLower, "amazon"):
			platformFamily = "rhel"
		case strings.Contains(osInfoLower, "suse") || strings.Contains(osInfoLower, "sles"):
			platformFamily = "suse"
		case strings.Contains(osInfoLower, "arch") || strings.Contains(osInfoLower, "manjaro"):
			platformFamily = "arch"
		case strings.Contains(osInfoLower, "alpine"):
			platformFamily = "alpine"
		case req.OSType == "darwin":
			platformFamily = "darwin"
		case req.OSType == "windows":
			platformFamily = "windows"
		default:
			platformFamily = "unknown"
		}
	}
	agentRecord.Set("platform_family", platformFamily)

	// Random password for Auth collection requirements.
	password, err := generateRandomPassword(32)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate password"})
	}
	agentRecord.SetPassword(password)

	if req.PrimaryIP != "" {
		agentRecord.Set("primary_ip", req.PrimaryIP)
	}
	if len(req.AllIPs) > 0 {
		agentRecord.Set("all_ips", req.AllIPs)
	}

	// Explicitly do NOT set: device_code_id, device_user_code,
	// refresh_token_hash, refresh_token_expires — those are for the
	// device-code OAuth flow only.

	if err := app.Save(agentRecord); err != nil {
		// Check if this is a pricing/rate-limit error from hooks
		var apiErr *router.ApiError
		if errors.As(err, &apiErr) {
			return c.JSON(apiErr.Status, types.ErrorResponse{Error: apiErr.Message})
		}
		app.Logger().Error("Failed to save agent via static token", "error", err)
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create agent"})
	}

	return c.JSON(http.StatusOK, types.RegisterWithStaticTokenResponse{
		AgentID: agentRecord.Id,
		Message: "agent registered via static token",
	})
}

type staticTokenError string

func (e staticTokenError) Error() string { return string(e) }

const errStaticTokenInvalid = staticTokenError("invalid static token")
