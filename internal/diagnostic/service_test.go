package diagnostic

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupTestService(t *testing.T) (*DiagnosticService, func()) {
	client, cleanup := setupTestDB(t)
	repo := NewDiagnosticRepository(client.Database(testDBName))

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	service := NewDiagnosticService(apiKey, repo)
	return service, cleanup
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
	service, cleanup := setupTestService(t)
	defer cleanup()

	assert.NotNil(t, service)
	assert.NotNil(t, service.client)
	assert.NotNil(t, service.repository)
	assert.Equal(t, 3, service.maxIterations)
}

func TestStartDiagnosticSession(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"
	systemInfo := map[string]string{
		"OS":     "Ubuntu 22.04",
		"Kernel": "5.15.0-91-generic",
		"CPU":    "Intel i7-1165G7",
		"Memory": "16GB",
	}

	session, err := service.StartDiagnosticSession(context.Background(), issue, systemInfo)
	if err != nil {
		t.Fatalf("Failed to start diagnostic session: %v", err)
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, issue, session.InitialIssue)
	assert.Equal(t, 0, session.CurrentIteration)
	assert.Equal(t, 3, session.MaxIterations)
	assert.Equal(t, "in_progress", session.Status)
	assert.NotEmpty(t, session.History)

	// Verify session was stored in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), session.ID)
	assert.NoError(t, err)
	assert.Equal(t, session.ID, storedSession.ID)
	assert.Equal(t, session.InitialIssue, storedSession.InitialIssue)
}

func TestContinueDiagnosticSession(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	// Create an initial session
	initialSession := &DiagnosticSession{
		ID:               "test_session_1",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	err := service.repository.CreateSession(context.Background(), initialSession)
	assert.NoError(t, err)

	// Continue the session with command results
	results := []string{
		"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
		"Tasks: 180 total, 2 running, 178 sleeping",
	}

	continuedSession, err := service.ContinueDiagnosticSession(context.Background(), initialSession.ID, results)
	if err != nil {
		t.Fatalf("Failed to continue diagnostic session: %v", err)
	}

	assert.Equal(t, 1, continuedSession.CurrentIteration)
	assert.NotEmpty(t, continuedSession.History)
	assert.Equal(t, "in_progress", continuedSession.Status)

	// Verify session was updated in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), initialSession.ID)
	assert.NoError(t, err)
	assert.Equal(t, continuedSession.CurrentIteration, storedSession.CurrentIteration)
	assert.Equal(t, len(continuedSession.History), len(storedSession.History))
}

func TestSessionMaxIterations(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	// Create a test session
	session := &DiagnosticSession{
		ID:               "test_session_3",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)

	results := []string{"Sample command output"}

	// Run through all iterations
	for i := 0; i < 3; i++ {
		var err error
		session, err = service.ContinueDiagnosticSession(context.Background(), session.ID, results)
		if err != nil {
			t.Fatalf("Failed in iteration %d: %v", i, err)
		}
	}

	assert.Equal(t, "completed", session.Status)
	assert.Equal(t, 3, session.CurrentIteration)
	assert.NotEmpty(t, session.History)

	// Verify final state in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), session.ID)
	assert.NoError(t, err)
	assert.Equal(t, "completed", storedSession.Status)
	assert.Equal(t, 3, storedSession.CurrentIteration)
}

func TestGetDiagnosticSummary(t *testing.T) {
	service, cleanup := setupTestService(t)
	defer cleanup()

	// Create a test session with history
	session := &DiagnosticSession{
		ID:               "test_session_4",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 1,
		MaxIterations:    3,
		Status:           "in_progress",
		History: []DiagnosticResponse{
			*mockDiagnosticResponse(),
			*mockDiagnosticResponse(),
		},
	}

	err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)

	summary, err := service.GetDiagnosticSummary(context.Background(), session.ID)
	assert.NoError(t, err)
	assert.Contains(t, summary, "High CPU usage")
	assert.Contains(t, summary, "Diagnostic Summary")
	assert.Contains(t, summary, "cpu")         // diagnosis_type
	assert.Contains(t, summary, "top -b -n 1") // command
}
