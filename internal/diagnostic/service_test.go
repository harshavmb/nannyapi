package diagnostic

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	// Initialize the diagnostic service
	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))
	assert.NotNil(t, service)
	assert.NotNil(t, service.client)
	assert.NotNil(t, service.sessions)
	assert.Equal(t, 3, service.maxIterations)
}

func TestStartDiagnosticSession(t *testing.T) {

	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))
	issue := "High CPU usage"
	systemInfo := map[string]string{
		"OS":     "Ubuntu 22.04",
		"Kernel": "5.15.0-91-generic",
		"CPU":    "Intel i7-1165G7",
		"Memory": "16GB",
	}

	session, err := service.StartDiagnosticSession(context.Background(), issue, systemInfo)
	if err != nil {
		t.Logf("Got error starting session: %v", err)
		// Even with an error, we should still create a session
		assert.NotNil(t, session)
		assert.Equal(t, issue, session.InitialIssue)
		return
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, issue, session.InitialIssue)
	assert.Equal(t, 0, session.CurrentIteration)
	assert.Equal(t, 3, session.MaxIterations)
	assert.Equal(t, "in_progress", session.Status)
	assert.NotEmpty(t, session.History)
}

func TestContinueDiagnosticSession(t *testing.T) {
	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))

	// Start a session first
	initialSession := &DiagnosticSession{
		ID:               "test_session_1",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	service.sessions[initialSession.ID] = initialSession

	// Continue the session with command results
	results := []string{
		"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
		"Tasks: 180 total, 2 running, 178 sleeping",
	}

	continuedSession, err := service.ContinueDiagnosticSession(context.Background(), initialSession.ID, results)
	if err != nil {
		t.Logf("Got error continuing session: %v", err)
		// Verify the session still exists and was updated
		assert.NotNil(t, continuedSession)
		assert.Equal(t, initialSession.ID, continuedSession.ID)
		return
	}

	assert.Equal(t, 1, continuedSession.CurrentIteration)
	assert.NotEmpty(t, continuedSession.History)
	assert.Equal(t, "in_progress", continuedSession.Status)
}

func TestGetDiagnosticSession(t *testing.T) {
	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))

	// Create a test session
	testSession := &DiagnosticSession{
		ID:               "test_session_2",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	service.sessions[testSession.ID] = testSession

	// Retrieve the session
	retrievedSession, err := service.GetDiagnosticSession(context.Background(), testSession.ID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedSession)
	assert.Equal(t, testSession.ID, retrievedSession.ID)
	assert.Equal(t, testSession.InitialIssue, retrievedSession.InitialIssue)
}

func TestSessionMaxIterations(t *testing.T) {
	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))

	// Create a test session
	session := &DiagnosticSession{
		ID:               "test_session_3",
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	service.sessions[session.ID] = session

	results := []string{"Sample command output"}

	// Run through all iterations
	for i := 0; i < 3; i++ {
		var err error
		session, err = service.ContinueDiagnosticSession(context.Background(), session.ID, results)
		if err != nil {
			t.Logf("Got error in iteration %d: %v", i, err)
			continue
		}
	}

	assert.Equal(t, "completed", session.Status)
	assert.Equal(t, 3, session.CurrentIteration)
	assert.NotEmpty(t, session.History)
}

func TestGetDiagnosticSummary(t *testing.T) {
	service := NewDiagnosticService(os.Getenv("DEEPSEEK_API_KEY"))

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

	service.sessions[session.ID] = session

	summary, err := service.GetDiagnosticSummary(context.Background(), session.ID)
	assert.NoError(t, err)
	assert.Contains(t, summary, "High CPU usage")
	assert.Contains(t, summary, "Diagnostic Summary")
	assert.Contains(t, summary, "cpu")         // diagnosis_type
	assert.Contains(t, summary, "top -b -n 1") // command
}
