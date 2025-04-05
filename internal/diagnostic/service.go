package diagnostic

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DiagnosticService manages diagnostic sessions and coordinates with DeepSeek API
type DiagnosticService struct {
	client        *DeepSeekClient
	sessions      map[string]*DiagnosticSession
	sessionsLock  sync.RWMutex
	maxIterations int
}

// NewDiagnosticService creates a new diagnostic service
func NewDiagnosticService(apiKey string) *DiagnosticService {
	return &DiagnosticService{
		client:        NewDeepSeekClient(apiKey),
		sessions:      make(map[string]*DiagnosticSession),
		maxIterations: 3, // Default max iterations
	}
}

// StartDiagnosticSession initiates a new diagnostic session
func (s *DiagnosticService) StartDiagnosticSession(ctx context.Context, issue string, systemInfo map[string]string) (*DiagnosticSession, error) {
	session := &DiagnosticSession{
		ID:               fmt.Sprintf("diag_%d", time.Now().UnixNano()),
		InitialIssue:     issue,
		CurrentIteration: 0,
		MaxIterations:    s.maxIterations,
		Status:           "in_progress",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		History:          make([]DiagnosticResponse, 0), // Initialize empty history
	}

	// Store the session before making the API call
	s.sessionsLock.Lock()
	s.sessions[session.ID] = session
	s.sessionsLock.Unlock()

	req := &DiagnosticRequest{
		Issue:      issue,
		SystemInfo: systemInfo,
		Iteration:  0,
	}

	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		// Return the session even if API call fails
		return session, fmt.Errorf("failed to start diagnostic session: %v", err)
	}

	session.History = append(session.History, *resp)
	session.UpdatedAt = time.Now()

	return session, nil
}

// ContinueDiagnosticSession continues an existing diagnostic session with new results
func (s *DiagnosticService) ContinueDiagnosticSession(ctx context.Context, sessionID string, results []string) (*DiagnosticSession, error) {
	s.sessionsLock.RLock()
	session, exists := s.sessions[sessionID]
	s.sessionsLock.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
		return session, nil
	}

	req := &DiagnosticRequest{
		Issue:          session.InitialIssue,
		SystemInfo:     make(map[string]string),
		CommandResults: results,
		Iteration:      session.CurrentIteration + 1,
	}

	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		// Even if the API call fails, we should update the iteration count
		// and potentially mark the session as completed
		session.CurrentIteration++
		session.UpdatedAt = time.Now()

		if session.CurrentIteration >= session.MaxIterations {
			session.Status = "completed"
		}

		return session, fmt.Errorf("failed to continue diagnostic session: %v", err)
	}

	session.History = append(session.History, *resp)
	session.CurrentIteration++
	session.UpdatedAt = time.Now()

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
	}

	return session, nil
}

// GetDiagnosticSession retrieves a diagnostic session by ID
func (s *DiagnosticService) GetDiagnosticSession(ctx context.Context, sessionID string) (*DiagnosticSession, error) {
	s.sessionsLock.RLock()
	defer s.sessionsLock.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	return session, nil
}

// GetDiagnosticSummary generates a summary of the diagnostic session
func (s *DiagnosticService) GetDiagnosticSummary(ctx context.Context, sessionID string) (string, error) {
	session, err := s.GetDiagnosticSession(ctx, sessionID)
	if err != nil {
		return "", err
	}

	summary := fmt.Sprintf("Diagnostic Summary for Issue: %s\n\n", session.InitialIssue)
	summary += fmt.Sprintf("Session Status: %s\n", session.Status)
	summary += fmt.Sprintf("Total Iterations: %d\n\n", len(session.History))

	for i, resp := range session.History {
		summary += fmt.Sprintf("Iteration %d:\n", i+1)
		summary += fmt.Sprintf("Diagnosis Type: %s\n", resp.DiagnosisType)

		if len(resp.Commands) > 0 {
			summary += "Commands:\n"
			for _, cmd := range resp.Commands {
				summary += fmt.Sprintf("- %s (timeout: %ds)\n", cmd.Command, cmd.TimeoutSeconds)
			}
		}

		if len(resp.LogChecks) > 0 {
			summary += "Log Checks:\n"
			for _, check := range resp.LogChecks {
				summary += fmt.Sprintf("- Check %s for pattern: %s\n", check.LogPath, check.GrepPattern)
			}
		}

		if resp.NextStep != "" {
			summary += fmt.Sprintf("Next Step: %s\n", resp.NextStep)
		}
		summary += "\n"
	}

	return summary, nil
}
