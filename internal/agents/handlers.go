package agents

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/nannyagent/nannyapi/internal/types"
	"github.com/pocketbase/pocketbase/core"
)

// generateRandomPassword creates a strong random password
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}

// HandleDeviceAuthStart - anonymous agent requests device code
func HandleDeviceAuthStart(app core.App, c *core.RequestEvent, frontendURL string) error {
	deviceCode := generateID()
	userCode := generateUserCode()

	collection, err := app.FindCollectionByNameOrId("device_codes")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "setup error"})
	}

	record := core.NewRecord(collection)
	record.Set("device_code", deviceCode)
	record.Set("user_code", userCode)
	record.Set("authorized", false)
	record.Set("consumed", false)
	record.Set("expires_at", time.Now().Add(10*time.Minute))

	if err := app.Save(record); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create device code"})
	}

	verificationURI := strings.TrimSuffix(frontendURL, "/") + "/agent/authorize?user_code=" + userCode

	return c.JSON(http.StatusOK, types.DeviceAuthResponse{
		DeviceCode:      deviceCode,
		UserCode:        userCode,
		VerificationURI: verificationURI,
		ExpiresIn:       600,
	})
}

// HandleAuthorize - user authorizes device code
func HandleAuthorize(app core.App, c *core.RequestEvent) error {
	var req types.AuthorizeRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	if req.UserCode == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "user_code required"})
	}

	// Get authenticated user
	user := c.Get("authRecord").(*core.Record)

	collection, _ := app.FindCollectionByNameOrId("device_codes")
	records, err := app.FindRecordsByFilter(collection, "user_code = {:code}", "", 1, 0, map[string]any{"code": req.UserCode})
	if err != nil || len(records) == 0 {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid user code"})
	}

	record := records[0]
	if time.Now().After(record.GetDateTime("expires_at").Time()) {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "code expired"})
	}

	record.Set("authorized", true)
	record.Set("user_id", user.Id)
	if err := app.Save(record); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to authorize"})
	}

	return c.JSON(http.StatusOK, types.AuthorizeResponse{
		Success: true,
		Message: "device authorized",
	})
}

// HandleRegister - agent registers using authorized device code
func HandleRegister(app core.App, c *core.RequestEvent) error {
	var req types.RegisterRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	if req.DeviceCode == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "device_code required"})
	}

	deviceCodesCollection, _ := app.FindCollectionByNameOrId("device_codes")
	deviceRecords, err := app.FindRecordsByFilter(deviceCodesCollection, "device_code = {:code}", "", 1, 0, map[string]any{"code": req.DeviceCode})
	if err != nil || len(deviceRecords) == 0 {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid device code"})
	}

	deviceRecord := deviceRecords[0]

	if time.Now().After(deviceRecord.GetDateTime("expires_at").Time()) {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "device code expired"})
	}

	if !deviceRecord.GetBool("authorized") {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "device not authorized yet"})
	}

	if deviceRecord.GetBool("consumed") {
		existingAgentID := deviceRecord.GetString("agent_id")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "device code already used", "agent_id": existingAgentID})
	}

	userID := deviceRecord.GetString("user_id")
	if userID == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "user_id missing"})
	}

	// Create agent
	agentsCollection, _ := app.FindCollectionByNameOrId("agents")
	agentRecord := core.NewRecord(agentsCollection)
	agentRecord.Set("user_id", userID)
	agentRecord.Set("device_code_id", deviceRecord.Id)
	agentRecord.Set("device_user_code", deviceRecord.GetString("user_code"))
	agentRecord.Set("hostname", req.Hostname)
	agentRecord.Set("os_type", req.OSType)
	agentRecord.Set("os_info", req.OSInfo)
	agentRecord.Set("os_version", req.OSVersion)
	agentRecord.Set("version", req.Version)
	agentRecord.Set("status", string(types.AgentStatusActive))
	agentRecord.Set("last_seen", time.Now())
	agentRecord.Set("kernel_version", req.KernelVersion)
	agentRecord.Set("arch", req.Arch)
	agentRecord.Set("auth_method", "device_code")

	// Fallback for platform_family if missing
	platformFamily := req.PlatformFamily
	if platformFamily == "" {
		// Try to guess from OSInfo
		osInfoLower := strings.ToLower(req.OSInfo)
		if strings.Contains(osInfoLower, "debian") || strings.Contains(osInfoLower, "ubuntu") || strings.Contains(osInfoLower, "mint") || strings.Contains(osInfoLower, "pop") {
			platformFamily = "debian"
		} else if strings.Contains(osInfoLower, "red hat") || strings.Contains(osInfoLower, "rhel") || strings.Contains(osInfoLower, "centos") || strings.Contains(osInfoLower, "fedora") || strings.Contains(osInfoLower, "alma") || strings.Contains(osInfoLower, "rocky") || strings.Contains(osInfoLower, "amazon") {
			platformFamily = "rhel"
		} else if strings.Contains(osInfoLower, "suse") || strings.Contains(osInfoLower, "sles") {
			platformFamily = "suse"
		} else if strings.Contains(osInfoLower, "arch") || strings.Contains(osInfoLower, "manjaro") {
			platformFamily = "arch"
		} else if strings.Contains(osInfoLower, "alpine") {
			platformFamily = "alpine"
		} else if req.OSType == "darwin" {
			platformFamily = "darwin"
		} else if req.OSType == "windows" {
			platformFamily = "windows"
		} else {
			platformFamily = "unknown"
		}
	}
	agentRecord.Set("platform_family", platformFamily)

	// Set random password to satisfy Auth collection requirements
	password, err := generateRandomPassword(32)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate password"})
	}
	agentRecord.SetPassword(password)

	// Store IP addresses if provided
	if req.PrimaryIP != "" {
		agentRecord.Set("primary_ip", req.PrimaryIP)
	}
	if len(req.AllIPs) > 0 {
		ipsJSON, _ := json.Marshal(req.AllIPs)
		agentRecord.Set("all_ips", string(ipsJSON))
	}

	// Generate tokens
	refreshToken := generateID()
	refreshTokenHash := HashToken(refreshToken)

	agentRecord.Set("refresh_token_hash", refreshTokenHash)
	agentRecord.Set("refresh_token_expires", time.Now().Add(30*24*time.Hour))

	if err := app.Save(agentRecord); err != nil {
		app.Logger().Error("Failed to save agent", "error", err)
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to create agent: " + err.Error()})
	}

	// Mark device code as consumed
	deviceRecord.Set("consumed", true)
	deviceRecord.Set("agent_id", agentRecord.Id)
	if err := app.Save(deviceRecord); err != nil {
		log.Printf("Warning: failed to mark device code as consumed: %v", err)
	}

	// Generate PocketBase Auth Token
	accessToken, tokenErr := agentRecord.NewAuthToken()
	if tokenErr != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate token"})
	}

	return c.JSON(http.StatusOK, types.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    3600, // Default PB token expiry is usually 14 days or configurable, but we can just say 3600 or whatever
		AgentID:      agentRecord.Id,
	})
}

// resolveAgentByRefreshToken validates the supplied refresh token and returns
// the owning agent record. Shared between HandleRefreshToken (no rotation)
// and HandleRenewRefreshToken (rotation).
func resolveAgentByRefreshToken(app core.App, token string) (*core.Record, *types.ErrorResponse, int) {
	if token == "" {
		return nil, &types.ErrorResponse{Error: "refresh_token required"}, http.StatusBadRequest
	}
	hash := HashToken(token)
	collection, _ := app.FindCollectionByNameOrId("agents")
	records, err := app.FindRecordsByFilter(collection, "refresh_token_hash = {:hash}", "", 1, 0, map[string]any{"hash": hash})
	if err != nil || len(records) == 0 {
		return nil, &types.ErrorResponse{Error: "invalid refresh token"}, http.StatusUnauthorized
	}
	agentRecord := records[0]
	if agentRecord.GetString("status") == string(types.AgentStatusRevoked) {
		return nil, &types.ErrorResponse{Error: "agent revoked"}, http.StatusUnauthorized
	}
	if time.Now().After(agentRecord.GetDateTime("refresh_token_expires").Time()) {
		return nil, &types.ErrorResponse{Error: "refresh token expired"}, http.StatusUnauthorized
	}
	return agentRecord, nil, 0
}

// HandleRefreshToken - issues a NEW ACCESS TOKEN using a still-valid refresh
// token. The refresh token itself is NOT rotated here; clients keep using the
// same refresh token until it is close to expiry, at which point they must
// call the "renew-refresh-token" action to obtain a new one.
//
// This is the standard OAuth2-style refresh flow: short-lived access tokens
// are obtained often, long-lived refresh tokens rotate rarely.
func HandleRefreshToken(app core.App, c *core.RequestEvent) error {
	var req types.RefreshRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	agentRecord, errResp, status := resolveAgentByRefreshToken(app, req.RefreshToken)
	if errResp != nil {
		return c.JSON(status, *errResp)
	}

	accessToken, tokenErr := agentRecord.NewAuthToken()
	if tokenErr != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate token"})
	}

	// RefreshToken intentionally omitted from the response: the existing
	// refresh token remains valid and unchanged. Clients renew it
	// explicitly via the "renew-refresh-token" action.
	refreshExpiresIn := int(time.Until(agentRecord.GetDateTime("refresh_token_expires").Time()).Seconds())
	if refreshExpiresIn < 0 {
		refreshExpiresIn = 0
	}
	return c.JSON(http.StatusOK, types.RefreshTokenResponse{
		AccessToken:           accessToken,
		ExpiresIn:             3600,
		RefreshTokenExpiresIn: refreshExpiresIn,
		AgentID:               agentRecord.Id,
	})
}

// HandleRenewRefreshToken - rotates the agent's refresh token. The previous
// refresh token is invalidated immediately; the response contains the NEW
// refresh token which the client MUST persist before discarding the old one.
//
// This endpoint is intended to be called sparingly — typically when the
// current refresh token is nearing expiry — NOT on every access-token
// refresh. Rotating on every call would defeat the purpose of the
// short-lived access / long-lived refresh token split.
func HandleRenewRefreshToken(app core.App, c *core.RequestEvent) error {
	var req types.RefreshRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	agentRecord, errResp, status := resolveAgentByRefreshToken(app, req.RefreshToken)
	if errResp != nil {
		return c.JSON(status, *errResp)
	}

	newRefreshToken := generateID()
	newHash := HashToken(newRefreshToken)
	refreshLifetime := 30 * 24 * time.Hour
	newExpiry := time.Now().Add(refreshLifetime)

	agentRecord.Set("refresh_token_hash", newHash)
	agentRecord.Set("refresh_token_expires", newExpiry)
	if err := app.Save(agentRecord); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to rotate refresh token"})
	}

	accessToken, tokenErr := agentRecord.NewAuthToken()
	if tokenErr != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to generate token"})
	}

	return c.JSON(http.StatusOK, types.RefreshTokenResponse{
		AccessToken:           accessToken,
		RefreshToken:          newRefreshToken,
		ExpiresIn:             3600,
		RefreshTokenExpiresIn: int(refreshLifetime.Seconds()),
		AgentID:               agentRecord.Id,
	})
}

// HandleIngestMetrics - agent sends metrics every 30s
func HandleIngestMetrics(app core.App, c *core.RequestEvent) error {
	var req types.IngestMetricsRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	// Get authenticated agent from context (set by middleware)
	authRecord := c.Get("authRecord")
	if authRecord == nil {
		return c.JSON(http.StatusUnauthorized, types.ErrorResponse{Error: "authentication required"})
	}
	agentRecord := authRecord.(*core.Record)

	if agentRecord.GetString("status") == string(types.AgentStatusRevoked) {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "agent revoked"})
	}

	// Update last_seen and other metadata
	agentRecord.Set("last_seen", time.Now())
	if req.OSInfo != "" {
		agentRecord.Set("os_info", req.OSInfo)
	}
	if req.OSVersion != "" {
		agentRecord.Set("os_version", req.OSVersion)
	}
	if req.Version != "" {
		agentRecord.Set("version", req.Version)
	}
	if req.PrimaryIP != "" {
		agentRecord.Set("primary_ip", req.PrimaryIP)
	}
	if req.KernelVersion != "" {
		agentRecord.Set("kernel_version", req.KernelVersion)
	}
	if req.Arch != "" {
		agentRecord.Set("arch", req.Arch)
	}
	if len(req.AllIPs) > 0 {
		ipsJSON, _ := json.Marshal(req.AllIPs)
		agentRecord.Set("all_ips", string(ipsJSON))
	}
	if req.KernelFamily == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "platform_family required"})
	}
	agentRecord.Set("platform_family", req.KernelFamily)

	if err := app.Save(agentRecord); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to update agent"})
	}

	// Store metrics
	metricsCollection, err := app.FindCollectionByNameOrId("agent_metrics")
	if err != nil {
		log.Printf("[ERROR] Error finding agent_metrics: %v", err)
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "metrics collection not found"})
	}

	// Try to find existing metrics for this agent
	records, err := app.FindRecordsByFilter(metricsCollection, "agent_id = {:agentId}", "", 1, 0, map[string]any{"agentId": agentRecord.Id})

	var metricsRecord *core.Record
	if err == nil && len(records) > 0 {
		metricsRecord = records[0]
	} else {
		metricsRecord = core.NewRecord(metricsCollection)
		metricsRecord.Set("agent_id", agentRecord.Id)
	}

	// Set individual metric fields from SystemMetrics struct
	metrics := req.SystemMetrics

	metricsRecord.Set("cpu_percent", metrics.CPUPercent)
	metricsRecord.Set("cpu_cores", metrics.CPUCores)
	metricsRecord.Set("memory_used_gb", metrics.MemoryUsedGB)
	metricsRecord.Set("memory_total_gb", metrics.MemoryTotalGB)
	metricsRecord.Set("memory_percent", metrics.MemoryPercent)
	metricsRecord.Set("disk_used_gb", metrics.DiskUsedGB)
	metricsRecord.Set("disk_total_gb", metrics.DiskTotalGB)
	metricsRecord.Set("disk_usage_percent", metrics.DiskUsagePercent)
	metricsRecord.Set("network_in_gb", metrics.NetworkStats.InGB)
	metricsRecord.Set("network_out_gb", metrics.NetworkStats.OutGB)
	metricsRecord.Set("load_avg_1min", metrics.LoadAverage.OneMin)
	metricsRecord.Set("load_avg_5min", metrics.LoadAverage.FiveMin)
	metricsRecord.Set("load_avg_15min", metrics.LoadAverage.FifteenMin)

	// Filesystems - store as JSON array (TextField requires JSON string)
	if len(metrics.Filesystems) > 0 {
		filesystemsJSON, _ := json.Marshal(metrics.Filesystems)
		metricsRecord.Set("filesystems", string(filesystemsJSON))
	}

	metricsRecord.Set("recorded_at", time.Now())

	if err := app.Save(metricsRecord); err != nil {
		log.Printf("[ERROR] Error saving metrics: %v", err)
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to save metrics"})
	}

	return c.JSON(http.StatusOK, types.IngestMetricsResponse{
		Success: true,
		Message: "metrics recorded",
	})
}

// HandleListAgents - list user's agents
func HandleListAgents(app core.App, c *core.RequestEvent) error {
	user := c.Get("authRecord").(*core.Record)

	collection, _ := app.FindCollectionByNameOrId("agents")
	records, err := app.FindRecordsByFilter(collection, "user_id = {:userId}", "", 100, 0, map[string]any{"userId": user.Id})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to fetch agents"})
	}

	result := make([]types.AgentListItem, 0, len(records))
	for _, agent := range records {
		lastSeen := agent.GetDateTime("last_seen")
		var lastSeenPtr *time.Time
		if !lastSeen.Time().IsZero() {
			t := lastSeen.Time()
			lastSeenPtr = &t
		}

		health := CalculateHealth(lastSeenPtr, types.AgentStatus(agent.GetString("status")))

		result = append(result, types.AgentListItem{
			ID:            agent.Id,
			Hostname:      agent.GetString("hostname"),
			OSType:        agent.GetString("os_type"),
			OSInfo:        agent.GetString("os_info"),
			OSVersion:     agent.GetString("os_version"),
			Version:       agent.GetString("version"),
			Status:        types.AgentStatus(agent.GetString("status")),
			Health:        health,
			LastSeen:      lastSeenPtr,
			Created:       agent.GetDateTime("created").Time(),
			KernelVersion: agent.GetString("kernel_version"),
			Arch:          agent.GetString("arch"),
		})
	}

	return c.JSON(http.StatusOK, types.ListAgentsResponse{Agents: result})
}

// HandleRevokeAgent - revoke agent access
func HandleRevokeAgent(app core.App, c *core.RequestEvent) error {
	var req types.RevokeAgentRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	if req.AgentID == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "agent_id required"})
	}

	user := c.Get("authRecord").(*core.Record)

	collection, _ := app.FindCollectionByNameOrId("agents")
	agentRecord, err := app.FindRecordById(collection, req.AgentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, types.ErrorResponse{Error: "agent not found"})
	}

	if agentRecord.GetString("user_id") != user.Id {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "not your agent"})
	}

	agentRecord.Set("status", string(types.AgentStatusRevoked))
	agentRecord.Set("refresh_token_hash", "")
	agentRecord.Set("refresh_token_expires", nil)

	if err := app.Save(agentRecord); err != nil {
		return c.JSON(http.StatusInternalServerError, types.ErrorResponse{Error: "failed to revoke agent"})
	}

	return c.JSON(http.StatusOK, types.RevokeAgentResponse{
		Success: true,
		Message: "agent revoked",
	})
}

// HandleAgentHealth - get agent health & latest metrics
func HandleAgentHealth(app core.App, c *core.RequestEvent) error {
	var req types.HealthRequest
	if err := c.BindBody(&req); err != nil {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "invalid request"})
	}

	if req.AgentID == "" {
		return c.JSON(http.StatusBadRequest, types.ErrorResponse{Error: "agent_id required"})
	}

	user := c.Get("authRecord").(*core.Record)

	collection, _ := app.FindCollectionByNameOrId("agents")
	agentRecord, err := app.FindRecordById(collection, req.AgentID)
	if err != nil {
		return c.JSON(http.StatusNotFound, types.ErrorResponse{Error: "agent not found"})
	}

	if agentRecord.GetString("user_id") != user.Id {
		return c.JSON(http.StatusForbidden, types.ErrorResponse{Error: "not your agent"})
	}

	lastSeen := agentRecord.GetDateTime("last_seen")
	var lastSeenPtr *time.Time
	if !lastSeen.Time().IsZero() {
		t := lastSeen.Time()
		lastSeenPtr = &t
	}

	status := types.AgentStatus(agentRecord.GetString("status"))
	health := CalculateHealth(lastSeenPtr, status)

	// Get latest metrics
	var latestMetrics *types.SystemMetrics
	metricsCollection, _ := app.FindCollectionByNameOrId("agent_metrics")
	metricsRecords, err := app.FindRecordsByFilter(metricsCollection, "agent_id = {:agentId}", "-recorded_at", 1, 0, map[string]any{"agentId": req.AgentID})
	if err == nil && len(metricsRecords) > 0 {
		metricsRecord := metricsRecords[0]

		// Reconstruct SystemMetrics from individual fields
		metrics := types.SystemMetrics{
			CPUPercent:       metricsRecord.GetFloat("cpu_percent"),
			CPUCores:         metricsRecord.GetInt("cpu_cores"),
			MemoryUsedGB:     metricsRecord.GetFloat("memory_used_gb"),
			MemoryTotalGB:    metricsRecord.GetFloat("memory_total_gb"),
			MemoryPercent:    metricsRecord.GetFloat("memory_percent"),
			DiskUsedGB:       metricsRecord.GetFloat("disk_used_gb"),
			DiskTotalGB:      metricsRecord.GetFloat("disk_total_gb"),
			DiskUsagePercent: metricsRecord.GetFloat("disk_usage_percent"),
			NetworkStats: types.NetworkStats{
				InGB:  metricsRecord.GetFloat("network_in_gb"),
				OutGB: metricsRecord.GetFloat("network_out_gb"),
			},
			LoadAverage: types.LoadAverage{
				OneMin:     metricsRecord.GetFloat("load_avg_1min"),
				FiveMin:    metricsRecord.GetFloat("load_avg_5min"),
				FifteenMin: metricsRecord.GetFloat("load_avg_15min"),
			},
		}

		// Parse filesystems JSON if present
		filesystemsJSON := metricsRecord.GetString("filesystems")
		if filesystemsJSON != "" {
			var filesystems []types.FilesystemStats
			if err := json.Unmarshal([]byte(filesystemsJSON), &filesystems); err == nil {
				metrics.Filesystems = filesystems
			}
		}

		latestMetrics = &metrics
	}

	return c.JSON(http.StatusOK, types.HealthResponse{
		AgentID:       agentRecord.Id,
		Status:        status,
		Health:        health,
		LastSeen:      lastSeenPtr,
		LatestMetrics: latestMetrics,
	})
}

// CalculateHealth determines agent health status
func CalculateHealth(lastSeen *time.Time, status types.AgentStatus) types.AgentHealthStatus {
	if status == types.AgentStatusRevoked || status == types.AgentStatusInactive {
		return types.HealthStatusInactive
	}

	if lastSeen == nil {
		return types.HealthStatusInactive
	}

	timeSince := time.Since(*lastSeen)
	if timeSince < 5*time.Minute {
		return types.HealthStatusHealthy
	} else if timeSince < 15*time.Minute {
		return types.HealthStatusStale
	}
	return types.HealthStatusInactive
}

// CleanupExpiredDeviceCodes removes old device codes
func CleanupExpiredDeviceCodes(app core.App) {
	collection, err := app.FindCollectionByNameOrId("device_codes")
	if err != nil {
		log.Printf("Cleanup error: %v", err)
		return
	}

	oneDayAgo := time.Now().Add(-24 * time.Hour)
	filter := "expires_at < {:now} || (consumed = true)"
	records, err := app.FindRecordsByFilter(collection, filter, "", 100, 0, map[string]any{
		"now":       time.Now().UTC(),
		"oneDayAgo": oneDayAgo,
	})
	if err != nil {
		log.Printf("Cleanup query error: %v", err)
		return
	}

	for _, record := range records {
		if err := app.Delete(record); err != nil {
			log.Printf("Failed to delete device code %s: %v", record.Id, err)
		}
	}

	if len(records) > 0 {
		log.Printf("Cleaned up %d device codes", len(records))
	}
}

// Utility functions

func generateID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// handle error appropriately, for example:
		panic("failed to generate random ID")
	}
	return hex.EncodeToString(b)
}

func generateUserCode() string {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	result := make([]byte, 8)
	for i := range result {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		result[i] = charset[n.Int64()]
	}
	return string(result)
}

// HashToken creates SHA-256 hash of token
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
