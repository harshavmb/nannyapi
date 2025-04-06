package diagnostic

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/harshavmb/nannyapi/internal/agent"
)

// DiagnosticService manages diagnostic sessions and coordinates with DeepSeek API.
type DiagnosticService struct {
	client        *DeepSeekClient
	repository    *DiagnosticRepository
	agentService  *agent.AgentInfoService
	maxIterations int
}

// NewDiagnosticService creates a new diagnostic service.
func NewDiagnosticService(apiKey string, repository *DiagnosticRepository, agentService *agent.AgentInfoService) *DiagnosticService {
	return &DiagnosticService{
		client:        NewDeepSeekClient(apiKey),
		repository:    repository,
		agentService:  agentService,
		maxIterations: 3,
	}
}

// StartDiagnosticSession initiates a new diagnostic session.
func (s *DiagnosticService) StartDiagnosticSession(ctx context.Context, agentID string, userID string, issue string) (*DiagnosticSession, error) {
	// Validate agent exists
	agentObjectID, err := bson.ObjectIDFromHex(agentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent ID format")
	}

	agentInfo, err := s.agentService.GetAgentInfoByID(ctx, agentObjectID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("agent not found")
		}
		return nil, fmt.Errorf("failed to validate agent: %v", err)
	}

	if agentInfo == nil {
		return nil, fmt.Errorf("agent not found")
	}

	// Validate agent belongs to user
	if agentInfo.UserID != userID {
		return nil, fmt.Errorf("agent does not belong to user")
	}

	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     issue,
		CurrentIteration: 0,
		MaxIterations:    s.maxIterations,
		Status:           "in_progress",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		History:          make([]DiagnosticResponse, 0),
	}

	sessionID, err := s.repository.CreateSession(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("failed to create session in database: %v", err)
	}

	session.ID = sessionID

	// Use agent's current metrics for initial diagnosis
	req := &DiagnosticRequest{
		Issue:         issue,
		SystemMetrics: &agentInfo.SystemMetrics,
		Iteration:     0,
	}

	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		return session, fmt.Errorf("failed to diagnose issue: %v", err)
	}

	// Store current system metrics with the diagnostic response
	resp.SystemSnapshot = &agentInfo.SystemMetrics
	session.History = append(session.History, *resp)
	session.UpdatedAt = time.Now()

	if err := s.repository.UpdateSession(ctx, session); err != nil {
		return session, fmt.Errorf("failed to update session in database: %v", err)
	}

	return session, nil
}

// ContinueDiagnosticSession continues an existing diagnostic session with new results.
func (s *DiagnosticService) ContinueDiagnosticSession(ctx context.Context, sessionID string, results []string) (*DiagnosticSession, error) {
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session ID format")
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to retrieve session")
	}

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
		if err := s.repository.UpdateSession(ctx, session); err != nil {
			return session, fmt.Errorf("failed to update session in database: %v", err)
		}
		return session, nil
	}

	// Get current agent info to check for system changes
	agentObjectID, err := bson.ObjectIDFromHex(session.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent ID format")
	}

	agentInfo, err := s.agentService.GetAgentInfoByID(ctx, agentObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent info: %v", err)
	}

	req := &DiagnosticRequest{
		Issue:          session.InitialIssue,
		SystemMetrics:  &agentInfo.SystemMetrics,
		CommandResults: results,
		Iteration:      session.CurrentIteration + 1,
	}

	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		// On AI service error, still increment iteration but don't add response
		session.CurrentIteration++
		session.UpdatedAt = time.Now()

		if session.CurrentIteration >= session.MaxIterations {
			session.Status = "completed"
		}

		if err := s.repository.UpdateSession(ctx, session); err != nil {
			return session, fmt.Errorf("failed to update session in database: %v", err)
		}
		return session, nil
	}

	// Check if system metrics have changed significantly
	lastMetrics := session.History[len(session.History)-1].SystemSnapshot
	if s.agentService.HasSystemMetricsChanged(*lastMetrics, agentInfo.SystemMetrics) {
		resp.NextStep += "\n[ALERT] Significant system changes detected since last check."
	}

	// Store current system metrics with the diagnostic response
	resp.SystemSnapshot = &agentInfo.SystemMetrics
	session.History = append(session.History, *resp)
	session.CurrentIteration++
	session.UpdatedAt = time.Now()

	if session.CurrentIteration >= session.MaxIterations {
		session.Status = "completed"
	}

	if err := s.repository.UpdateSession(ctx, session); err != nil {
		return session, fmt.Errorf("failed to update session in database: %v", err)
	}

	return session, nil
}

// DeleteSession deletes a diagnostic session and all its associated data.
func (s *DiagnosticService) DeleteSession(ctx context.Context, sessionID string, userID string) error {
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		return fmt.Errorf("invalid session ID format: %v", err)
	}

	// First get the session to verify ownership
	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("session not found")
		}
		return err
	}

	// Verify ownership
	if session.UserID != userID {
		return fmt.Errorf("user does not own this session")
	}

	// Delete the session
	if err := s.repository.DeleteSession(ctx, id); err != nil {
		return fmt.Errorf("failed to delete session: %v", err)
	}

	return nil
}

// ListUserSessions returns all diagnostic sessions for a user.
func (s *DiagnosticService) ListUserSessions(ctx context.Context, userID string) ([]*DiagnosticSession, error) {
	filter := bson.M{"user_id": userID}
	return s.repository.ListSessions(ctx, filter)
}

// GetDiagnosticSession retrieves a diagnostic session by ID.
func (s *DiagnosticService) GetDiagnosticSession(ctx context.Context, sessionID string) (*DiagnosticSession, error) {
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		return nil, fmt.Errorf("invalid session ID format %v", err)
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("session not found")
		}
		return nil, fmt.Errorf("failed to retrieve session %v", err)
	}
	return session, nil
}

// GetDiagnosticSummary generates a summary of the diagnostic session.
func (s *DiagnosticService) GetDiagnosticSummary(ctx context.Context, sessionID string) (string, error) {
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		return "", fmt.Errorf("invalid session ID format %v", err)
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", fmt.Errorf("session not found")
		}
		return "", fmt.Errorf("failed to retrieve session %v", err)
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
