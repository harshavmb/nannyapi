package diagnostic

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/harshavmb/nannyapi/internal/agent"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func setupTestService(t *testing.T) (*DiagnosticService, func(), string, string) {
	client, cleanup := setupTestDB(t)
	repo := NewDiagnosticRepository(client.Database(testDBName))
	agentRepo := agent.NewAgentInfoRepository(client.Database(testDBName))
	agentService := agent.NewAgentInfoService(agentRepo)

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	service := NewDiagnosticService(apiKey, repo, agentService)

	// Create a test agent
	testUserID := "test_user_123"
	testAgent := &agent.AgentInfo{
		ID:            bson.NewObjectID(),
		UserID:        testUserID,
		Hostname:      "test-host",
		IPAddress:     "192.168.1.1",
		KernelVersion: "5.10.0",
		OsVersion:     "Ubuntu 24.04",
		CreatedAt:     time.Now(),
	}

	insertResult, err := agentService.SaveAgentInfo(context.Background(), *testAgent)
	if err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return service, cleanup, insertResult.InsertedID.(bson.ObjectID).Hex(), testUserID
}

// mockDiagnosticResponse creates a mock response for testing
func mockDiagnosticResponse() *DiagnosticResponse {
	return &DiagnosticResponse{
		DiagnosisType: "cpu",
		Commands: []DiagnosticCommand{
			{Command: "top -b -n 1", TimeoutSeconds: 5},
			{Command: "vmstat 1 5", TimeoutSeconds: 5},
		},
		LogChecks: []LogCheck{
			{LogPath: "/var/log/syslog", GrepPattern: "oom-killer"},
		},
		NextStep: "Analyze CPU usage patterns",
	}
}

func TestNewDiagnosticService(t *testing.T) {
	service, cleanup, _, _ := setupTestService(t)
	defer cleanup()

	assert.NotNil(t, service)
	assert.NotNil(t, service.client)
	assert.NotNil(t, service.repository)
	assert.Equal(t, 3, service.maxIterations)
}

func TestStartDiagnosticSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"
	systemInfo := map[string]string{
		"OS":     "Ubuntu 22.04",
		"Kernel": "5.15.0-91-generic",
		"CPU":    "Intel i7-1165G7",
		"Memory": "16GB",
	}

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID, issue, systemInfo)
	if err != nil {
		t.Fatalf("Failed to start diagnostic session: %v", err)
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, agentID, session.AgentID)
	assert.Equal(t, userID, session.UserID)
	assert.Equal(t, issue, session.InitialIssue)
	assert.Equal(t, 0, session.CurrentIteration)
	assert.Equal(t, 3, session.MaxIterations)
	assert.Equal(t, "in_progress", session.Status)
	assert.NotEmpty(t, session.History)

	// Verify session was stored in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), session.ID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, session.ID, storedSession.ID)
	assert.Equal(t, session.InitialIssue, storedSession.InitialIssue)
}

func TestContinueDiagnosticSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create an initial session
	initialSession := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), initialSession)
	assert.NoError(t, err)
	initialSession.ID = sessionID

	// Continue the session with command results
	results := []string{
		"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
		"Tasks: 180 total, 2 running, 178 sleeping",
	}

	continuedSession, err := service.ContinueDiagnosticSession(context.Background(), sessionID.Hex(), results)
	if err != nil {
		t.Fatalf("Failed to continue diagnostic session: %v", err)
	}

	assert.Equal(t, 1, continuedSession.CurrentIteration)
	assert.NotEmpty(t, continuedSession.History)
	assert.Equal(t, "in_progress", continuedSession.Status)

	// Verify session was updated in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), sessionID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, continuedSession.CurrentIteration, storedSession.CurrentIteration)
	assert.Equal(t, len(continuedSession.History), len(storedSession.History))
}

func TestSessionMaxIterations(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	results := []string{"Sample command output"}

	// Run through all iterations
	for i := 0; i < 3; i++ {
		var err error
		session, err = service.ContinueDiagnosticSession(context.Background(), sessionID.Hex(), results)
		if err != nil {
			t.Fatalf("Failed in iteration %d: %v", i, err)
		}
	}

	assert.Equal(t, "completed", session.Status)
	assert.Equal(t, 3, session.CurrentIteration)
	assert.NotEmpty(t, session.History)

	// Verify final state in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), sessionID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, "completed", storedSession.Status)
	assert.Equal(t, 3, storedSession.CurrentIteration)
}

func TestGetDiagnosticSummary(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session with history
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 1,
		MaxIterations:    3,
		Status:           "in_progress",
		History: []DiagnosticResponse{
			*mockDiagnosticResponse(),
			*mockDiagnosticResponse(),
		},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	summary, err := service.GetDiagnosticSummary(context.Background(), sessionID.Hex())
	assert.NoError(t, err)
	assert.Contains(t, summary, "High CPU usage")
	assert.Contains(t, summary, "Diagnostic Summary")
	assert.Contains(t, summary, "cpu")         // diagnosis_type
	assert.Contains(t, summary, "top -b -n 1") // command
}

func TestStartDiagnosticSessionWithInvalidAgent(t *testing.T) {
	service, cleanup, _, userID := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"
	systemInfo := map[string]string{
		"OS":     "Ubuntu 22.04",
		"Kernel": "5.15.0-91-generic",
	}

	// Test with non-existent agent ID
	nonExistentAgentID := bson.NewObjectID().Hex()
	_, err := service.StartDiagnosticSession(context.Background(), nonExistentAgentID, userID, issue, systemInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")

	// Test with invalid agent ID format
	_, err = service.StartDiagnosticSession(context.Background(), "invalid-id", userID, issue, systemInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid agent ID format")
}

func TestStartDiagnosticSessionWithWrongUser(t *testing.T) {
	service, cleanup, agentID, _ := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"
	systemInfo := map[string]string{
		"OS":     "Ubuntu 22.04",
		"Kernel": "5.15.0-91-generic",
	}

	// Test with wrong user ID
	wrongUserID := "wrong_user_123"
	_, err := service.StartDiagnosticSession(context.Background(), agentID, wrongUserID, issue, systemInfo)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent does not belong to user")
}

func TestDeleteSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	// Test successful deletion
	err = service.DeleteSession(context.Background(), sessionID.Hex(), userID)
	assert.NoError(t, err)

	// Verify session was deleted
	_, err = service.GetDiagnosticSession(context.Background(), sessionID.Hex())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// Test deletion with wrong user
	wrongUserID := "wrong_user_123"
	err = service.DeleteSession(context.Background(), sessionID.Hex(), wrongUserID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// Test deletion of non-existent session
	err = service.DeleteSession(context.Background(), bson.NewObjectID().Hex(), userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}
