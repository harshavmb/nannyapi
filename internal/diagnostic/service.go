package diagnostic

import (
	"context"
	"fmt"
	"time"
)

// DiagnosticService manages diagnostic sessions and coordinates with DeepSeek API
type DiagnosticService struct {
	client        *DeepSeekClient
	repository    *DiagnosticRepository
	maxIterations int
}

// NewDiagnosticService creates a new diagnostic service
func NewDiagnosticService(apiKey string, repository *DiagnosticRepository) *DiagnosticService {
	return &DiagnosticService{
		client:        NewDeepSeekClient(apiKey),
		repository:    repository,
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
		History:          make([]DiagnosticResponse, 0),
	}

	// Store the session before making the API call
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return session, fmt.Errorf("failed to create session in database: %v", err)
	}

	req := &DiagnosticRequest{
		Issue:      issue,
		SystemInfo: systemInfo,
		Iteration:  0,
	}

	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		return session, fmt.Errorf("failed to start diagnostic session: %v", err)
	}

	session.History = append(session.History, *resp)
	session.UpdatedAt = time.Now()

	if err := s.repository.UpdateSession(ctx, session); err != nil {
		return session, fmt.Errorf("failed to update session in database: %v", err)
	}

	return session, nil
}

// ContinueDiagnosticSession continues an existing diagnostic session with new results
func (s *DiagnosticService) ContinueDiagnosticSession(ctx context.Context, sessionID string, results []string) (*DiagnosticSession, error) {
	session, err := s.repository.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %s", err)
	}

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
		if err := s.repository.UpdateSession(ctx, session); err != nil {
			return session, fmt.Errorf("failed to update session status: %v", err)
		}
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

		if err := s.repository.UpdateSession(ctx, session); err != nil {
			return session, fmt.Errorf("failed to update session: %v", err)
		}
		return session, fmt.Errorf("failed to continue diagnostic session: %v", err)
	}

	session.History = append(session.History, *resp)
	session.CurrentIteration++
	session.UpdatedAt = time.Now()

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
	}

	if err := s.repository.UpdateSession(ctx, session); err != nil {
		return session, fmt.Errorf("failed to update session: %v", err)
	}

	return session, nil
}

// GetDiagnosticSession retrieves a diagnostic session by ID
func (s *DiagnosticService) GetDiagnosticSession(ctx context.Context, sessionID string) (*DiagnosticSession, error) {
	return s.repository.GetSession(ctx, sessionID)
}

// GetDiagnosticSummary generates a summary of the diagnostic session
func (s *DiagnosticService) GetDiagnosticSummary(ctx context.Context, sessionID string) (string, error) {
	session, err := s.repository.GetSession(ctx, sessionID)
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
