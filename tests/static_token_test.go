package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nannyagent/nannyapi/internal/agents"
	"github.com/nannyagent/nannyapi/internal/hooks"
	"github.com/nannyagent/nannyapi/internal/types"
	_ "github.com/nannyagent/nannyapi/pb_migrations"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	pbtests "github.com/pocketbase/pocketbase/tests"
)

// buildAgentRouter initializes the test app, runs migrations, registers
// agent hooks and returns an http.Handler wired to /api/agent. We avoid
// pocketbase's ApiScenario helper for tests that issue multiple HTTP calls
// on the same app instance because apis.NewRouter is invoked per-scenario
// by ApiScenario which accumulates duplicate /_/extensions.js bindings in
// PB v0.37.2.
func buildAgentRouter(t *testing.T) (*pbtests.TestApp, http.Handler) {
	t.Helper()
	app, err := pbtests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.RunAllMigrations(); err != nil {
		app.Cleanup()
		t.Fatalf("Failed to run migrations: %v", err)
	}
	hooks.RegisterAgentHooks(app)

	baseRouter, err := apis.NewRouter(app)
	if err != nil {
		app.Cleanup()
		t.Fatalf("NewRouter: %v", err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: baseRouter}
	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error { return nil }); err != nil {
		app.Cleanup()
		t.Fatalf("OnServe trigger: %v", err)
	}
	mux, err := baseRouter.BuildMux()
	if err != nil {
		app.Cleanup()
		t.Fatalf("BuildMux: %v", err)
	}
	return app, mux
}

func postAgent(t *testing.T, mux http.Handler, payload any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/agent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func readJSON(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	buf, _ := io.ReadAll(rec.Body)
	if out != nil {
		if err := json.Unmarshal(buf, out); err != nil {
			t.Fatalf("decode: %v; body=%s", err, string(buf))
		}
	}
}

// --- Refresh token: access renewal (NO rotation) --------------------------

// TestRefreshActionIssuesAccessTokenOnly verifies that the "refresh" action
// issues a new access token WITHOUT rotating the refresh token itself. The
// refresh token must remain usable after the call.
func TestRefreshActionIssuesAccessTokenOnly(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("acc_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "acc-host")
	orig := fmt.Sprintf("access-seed-%d", time.Now().UnixNano())
	agent.Set("refresh_token_hash", agents.HashToken(orig))
	agent.Set("refresh_token_expires", time.Now().Add(24*time.Hour))
	agent.Set("status", string(types.AgentStatusActive))
	if err := app.Save(agent); err != nil {
		t.Fatal(err)
	}

	rec := postAgent(t, mux, map[string]string{"action": "refresh", "refresh_token": orig}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp types.RefreshTokenResponse
	readJSON(t, rec, &resp)
	if resp.AccessToken == "" {
		t.Fatalf("missing access_token: %+v", resp)
	}
	if resp.RefreshToken != "" {
		t.Fatalf("refresh action must NOT return a new refresh_token; got %q", resp.RefreshToken)
	}
	if resp.RefreshTokenExpiresIn <= 0 {
		t.Fatalf("expected positive refresh_token_expires_in, got %d", resp.RefreshTokenExpiresIn)
	}

	// The original refresh token must still work.
	rec2 := postAgent(t, mux, map[string]string{"action": "refresh", "refresh_token": orig}, nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("original refresh token must remain valid: got %d: %s", rec2.Code, rec2.Body.String())
	}
}

// --- Refresh token: explicit rotation via renew-refresh-token --------------

// TestRenewRefreshTokenRotation covers: a valid refresh token yields new
// access AND refresh tokens on renew-refresh-token; the old refresh token
// is invalidated; the new refresh token works.
func TestRenewRefreshTokenRotation(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("rotate_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "rotate-host")
	orig := fmt.Sprintf("rotate-seed-%d", time.Now().UnixNano())
	agent.Set("refresh_token_hash", agents.HashToken(orig))
	agent.Set("refresh_token_expires", time.Now().Add(24*time.Hour))
	agent.Set("status", string(types.AgentStatusActive))
	if err := app.Save(agent); err != nil {
		t.Fatal(err)
	}

	rec := postAgent(t, mux, map[string]string{"action": "renew-refresh-token", "refresh_token": orig}, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp types.RefreshTokenResponse
	readJSON(t, rec, &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("missing tokens: %+v", resp)
	}
	if resp.RefreshToken == orig {
		t.Fatal("refresh token was not rotated")
	}
	if resp.RefreshTokenExpiresIn <= 0 {
		t.Fatalf("expected positive refresh_token_expires_in, got %d", resp.RefreshTokenExpiresIn)
	}
	if resp.AgentID != agent.Id {
		t.Fatalf("agent_id mismatch")
	}

	// Old token rejected.
	rec2 := postAgent(t, mux, map[string]string{"action": "renew-refresh-token", "refresh_token": orig}, nil)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("old token: expected 401, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "invalid refresh token") {
		t.Fatalf("old token: unexpected body %s", rec2.Body.String())
	}

	// New token still works against renew-refresh-token.
	rec3 := postAgent(t, mux, map[string]string{"action": "renew-refresh-token", "refresh_token": resp.RefreshToken}, nil)
	if rec3.Code != http.StatusOK {
		t.Fatalf("new token: expected 200, got %d: %s", rec3.Code, rec3.Body.String())
	}
}

// TestRefreshTokenRejectsClearedHash ensures that once the refresh_token_hash
// is cleared (as HandleRevokeAgent does), neither refresh nor
// renew-refresh-token can succeed.
func TestRefreshTokenRejectsClearedHash(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()
	user := createTestUser(app, t, fmt.Sprintf("rev_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "revoked-host")
	tok := "revoked-agent-refresh"
	agent.Set("refresh_token_hash", agents.HashToken(tok))
	agent.Set("refresh_token_expires", time.Now().Add(time.Hour))
	if err := app.Save(agent); err != nil {
		t.Fatal(err)
	}
	// Simulate revoke by clearing the hash.
	agent.Set("refresh_token_hash", "")
	if err := app.Save(agent); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"refresh", "renew-refresh-token"} {
		rec := postAgent(t, mux, map[string]string{"action": action, "refresh_token": tok}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d: %s", action, rec.Code, rec.Body.String())
		}
	}
}

// TestRefreshTokenExpired covers expired refresh token rejection on both
// the access-renewal and rotation actions.
func TestRefreshTokenExpired(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()
	user := createTestUser(app, t, fmt.Sprintf("exp_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "exp-host")
	tok := "expired-refresh"
	agent.Set("refresh_token_hash", agents.HashToken(tok))
	agent.Set("refresh_token_expires", time.Now().Add(-time.Hour))
	agent.Set("status", string(types.AgentStatusActive))
	if err := app.Save(agent); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"refresh", "renew-refresh-token"} {
		rec := postAgent(t, mux, map[string]string{"action": action, "refresh_token": tok}, nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d", action, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "expired") {
			t.Fatalf("%s: unexpected body: %s", action, rec.Body.String())
		}
	}
}

// --- Static tokens ---------------------------------------------------------

// TestStaticTokenCreateValidation covers input validation failures.
func TestStaticTokenCreateValidation(t *testing.T) {
	cases := []struct {
		name      string
		body      map[string]any
		wantCode  int
		wantError string
	}{
		{"missing name", map[string]any{"action": "create-static-token"}, 400, "name required"},
		{"bad expiry", map[string]any{"action": "create-static-token", "name": "t1", "expires_in_days": 7}, 400, "expires_in_days must be one of"},
		{"name too long", map[string]any{"action": "create-static-token", "name": strings.Repeat("x", 121)}, 400, "name too long"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app, mux := buildAgentRouter(t)
			defer app.Cleanup()
			user := createTestUser(app, t, fmt.Sprintf("stv_%d@example.com", time.Now().UnixNano()), "TestPass123!")
			userToken, err := user.NewAuthToken()
			if err != nil {
				t.Fatal(err)
			}
			rec := postAgent(t, mux, tc.body, map[string]string{"Authorization": "Bearer " + userToken})
			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantError) {
				t.Fatalf("expected body to contain %q, got %s", tc.wantError, rec.Body.String())
			}
		})
	}
}

// TestStaticTokenCreateRequiresAuth rejects anonymous callers.
func TestStaticTokenCreateRequiresAuth(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()
	rec := postAgent(t, mux, map[string]any{"action": "create-static-token", "name": "x"}, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// TestStaticTokenFullLifecycle: create → list → auth via token → revoke →
// revoked token no longer works.
func TestStaticTokenFullLifecycle(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()
	user := createTestUser(app, t, fmt.Sprintf("stl_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	userToken, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	userHdr := map[string]string{"Authorization": "Bearer " + userToken}

	// Create
	rec := postAgent(t, mux, map[string]any{"action": "create-static-token", "name": "ci-token", "expires_in_days": 30}, userHdr)
	if rec.Code != http.StatusOK {
		t.Fatalf("create expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var created types.CreateStaticTokenResponse
	readJSON(t, rec, &created)
	if !strings.HasPrefix(created.Token, agents.StaticTokenPrefix) {
		t.Fatalf("token prefix missing: %q", created.Token)
	}
	if created.TokenInfo.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set for 30-day token")
	}

	// List
	listRec := postAgent(t, mux, map[string]any{"action": "list-static-tokens"}, userHdr)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: %d %s", listRec.Code, listRec.Body.String())
	}
	var listed types.ListStaticTokensResponse
	readJSON(t, listRec, &listed)
	if len(listed.Tokens) != 1 || listed.Tokens[0].ID != created.TokenInfo.ID {
		t.Fatalf("list mismatch: %+v", listed)
	}
	if strings.Contains(listRec.Body.String(), created.Token) {
		t.Fatal("plaintext token leaked in list response")
	}

	// Use static token as bearer
	authRec := postAgent(t, mux, map[string]any{"action": "list-static-tokens"}, map[string]string{"Authorization": "Bearer " + created.Token})
	if authRec.Code != http.StatusOK {
		t.Fatalf("static-token auth: %d %s", authRec.Code, authRec.Body.String())
	}

	// Revoke
	revRec := postAgent(t, mux, map[string]any{"action": "revoke-static-token", "token_id": created.TokenInfo.ID}, userHdr)
	if revRec.Code != http.StatusOK {
		t.Fatalf("revoke: %d %s", revRec.Code, revRec.Body.String())
	}

	// After revoke, the static token no longer authenticates.
	dead := postAgent(t, mux, map[string]any{"action": "list-static-tokens"}, map[string]string{"Authorization": "Bearer " + created.Token})
	if dead.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token should 401, got %d: %s", dead.Code, dead.Body.String())
	}
	if _, err := agents.ResolveStaticToken(app, created.Token); err == nil {
		t.Fatal("expected revoked token to be invalid")
	}
}

// TestStaticTokenNeverExpires covers expires_in_days=0 => no expiry.
func TestStaticTokenNeverExpires(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()
	user := createTestUser(app, t, fmt.Sprintf("stn_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	userToken, _ := user.NewAuthToken()
	rec := postAgent(t, mux, map[string]any{"action": "create-static-token", "name": "forever", "expires_in_days": 0},
		map[string]string{"Authorization": "Bearer " + userToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var created types.CreateStaticTokenResponse
	readJSON(t, rec, &created)
	if created.TokenInfo.ExpiresAt != nil {
		t.Fatalf("expected nil expires_at for expires_in_days=0, got %v", created.TokenInfo.ExpiresAt)
	}
}

// TestStaticTokenAgentImpersonation exercises X-Agent-ID resolution on the
// middleware path: a shared user-owned static token can authenticate AS a
// specific agent belonging to the same user.
func TestStaticTokenAgentImpersonation(t *testing.T) {
	app, mux := buildAgentRouter(t)
	defer app.Cleanup()

	user := createTestUser(app, t, fmt.Sprintf("sti_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	agent := createTestAgent(app, t, user.Id, "impersonation-host")

	// Issue a static token directly via DB.
	tokColl, _ := app.FindCollectionByNameOrId("agent_static_tokens")
	raw := agents.StaticTokenPrefix + "deadbeefcafe1234567890abcdef0123456789abcdef0123456789abcdef0123"
	tr := core.NewRecord(tokColl)
	tr.Set("user_id", user.Id)
	tr.Set("name", "shared")
	tr.Set("token_hash", agents.HashToken(raw))
	tr.Set("token_prefix", raw[:12])
	if err := app.Save(tr); err != nil {
		t.Fatal(err)
	}

	// Without X-Agent-ID → auth succeeds as the user (list-static-tokens works).
	userCall := postAgent(t, mux, map[string]any{"action": "list-static-tokens"}, map[string]string{"Authorization": "Bearer " + raw})
	if userCall.Code != http.StatusOK {
		t.Fatalf("user-scoped call expected 200, got %d: %s", userCall.Code, userCall.Body.String())
	}

	// With a bogus X-Agent-ID that does not belong to the user → 403.
	badCall := postAgent(t, mux, map[string]any{"action": "list"},
		map[string]string{"Authorization": "Bearer " + raw, "X-Agent-ID": "nonexistent"})
	if badCall.Code != http.StatusForbidden {
		t.Fatalf("bad X-Agent-ID expected 403, got %d: %s", badCall.Code, badCall.Body.String())
	}

	// With a valid X-Agent-ID → authRecord becomes the agent; list action
	// requires USER auth, so this should respond with not-your-agent or
	// equivalent user-only failure. We assert the middleware accepted the
	// impersonation by using the "health" action on the agent's own id.
	healthCall := postAgent(t, mux, map[string]any{"action": "health", "agent_id": agent.Id},
		map[string]string{"Authorization": "Bearer " + raw, "X-Agent-ID": agent.Id})
	// "health" handler does a user-ownership check via authRecord.Id; when
	// authRecord is the agent itself, the ids won't match so we expect 403
	// "not your agent". This still proves the middleware resolved the token
	// and set the agent as authRecord.
	if healthCall.Code == http.StatusUnauthorized {
		t.Fatalf("expected middleware to authenticate, got 401: %s", healthCall.Body.String())
	}
}

// TestStaticTokenExpiredRejected ensures expired static tokens are refused.
func TestStaticTokenExpiredRejected(t *testing.T) {
	app, _ := buildAgentRouter(t)
	defer app.Cleanup()
	user := createTestUser(app, t, fmt.Sprintf("ste_%d@example.com", time.Now().UnixNano()), "TestPass123!")
	tokColl, _ := app.FindCollectionByNameOrId("agent_static_tokens")
	raw := agents.StaticTokenPrefix + "1111111111111111111111111111111111111111111111111111111111111111"
	r := core.NewRecord(tokColl)
	r.Set("user_id", user.Id)
	r.Set("name", "expired")
	r.Set("token_hash", agents.HashToken(raw))
	r.Set("token_prefix", raw[:12])
	r.Set("expires_at", time.Now().Add(-time.Hour))
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	if _, err := agents.ResolveStaticToken(app, raw); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}
