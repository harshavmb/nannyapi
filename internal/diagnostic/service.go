package diagnostic

import (
	"context"
	"fmt"
	"log"
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
	log.Printf("Initializing diagnostic service with max iterations: %d", 3)
	return &DiagnosticService{
		client:        NewDeepSeekClient(apiKey),
		repository:    repository,
		agentService:  agentService,
		maxIterations: 3,
	}
}

// StartDiagnosticSession initiates a new diagnostic session.
func (s *DiagnosticService) StartDiagnosticSession(ctx context.Context, agentID string, userID string, issue string) (*DiagnosticSession, error) {
	log.Printf("Starting new diagnostic session - User: %s, Agent: %s, Issue: %s", userID, agentID, issue)

	// Validate agent exists
	agentObjectID, err := bson.ObjectIDFromHex(agentID)
	if err != nil {
		log.Printf("Invalid agent ID format - User: %s, Agent: %s", userID, agentID)
		return nil, fmt.Errorf("invalid agent ID format")
	}

	agentInfo, err := s.agentService.GetAgentInfoByID(ctx, agentObjectID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Agent not found - User: %s, Agent: %s", userID, agentID)
			return nil, fmt.Errorf("agent not found")
		}
		log.Printf("Error validating agent - User: %s, Agent: %s, Error: %v", userID, agentID, err)
		return nil, fmt.Errorf("failed to validate agent: %v", err)
	}

	if agentInfo == nil {
		log.Printf("Agent not found - User: %s, Agent: %s", userID, agentID)
		return nil, fmt.Errorf("agent not found")
	}

	// Validate agent belongs to user
	if agentInfo.UserID != userID {
		log.Printf("Agent ownership mismatch - User: %s, Agent: %s, Owner: %s", userID, agentID, agentInfo.UserID)
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

	log.Printf("Creating diagnostic session in database - User: %s, Agent: %s", userID, agentID)
	sessionID, err := s.repository.CreateSession(ctx, session)
	if err != nil {
		log.Printf("Error creating session in database - User: %s, Agent: %s, Error: %v", userID, agentID, err)
		return nil, fmt.Errorf("failed to create session in database: %v", err)
	}

	session.ID = sessionID
	log.Printf("Session created successfully - ID: %s, User: %s, Agent: %s", sessionID.Hex(), userID, agentID)

	// Use agent's current metrics for initial diagnosis
	req := &DiagnosticRequest{
		Issue:         issue,
		SystemMetrics: &agentInfo.SystemMetrics,
		Iteration:     0,
	}

	log.Printf("Initiating initial diagnosis - Session: %s", sessionID.Hex())
	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		log.Printf("Error during initial diagnosis - Session: %s, Error: %v", sessionID.Hex(), err)
		return session, fmt.Errorf("failed to diagnose issue: %v", err)
	}
	log.Printf("Initial diagnosis completed - Session: %s, Type: %s", sessionID.Hex(), resp.DiagnosisType)

	// Store current system metrics with the diagnostic response
	resp.SystemSnapshot = &agentInfo.SystemMetrics
	session.History = append(session.History, *resp)
	session.UpdatedAt = time.Now()

	log.Printf("Updating session with initial diagnosis - Session: %s", sessionID.Hex())
	if err := s.repository.UpdateSession(ctx, session); err != nil {
		log.Printf("Error updating session with initial diagnosis - Session: %s, Error: %v", sessionID.Hex(), err)
		return session, fmt.Errorf("failed to update session in database: %v", err)
	}

	return session, nil
}

// ContinueDiagnosticSession continues an existing diagnostic session with new results.
func (s *DiagnosticService) ContinueDiagnosticSession(ctx context.Context, sessionID string, results []string) (*DiagnosticSession, error) {
	log.Printf("Continuing diagnostic session - Session: %s", sessionID)

	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		log.Printf("Invalid session ID format - Session: %s", sessionID)
		return nil, fmt.Errorf("invalid session ID format")
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Session not found - Session: %s", sessionID)
			return nil, fmt.Errorf("session not found")
		}
		log.Printf("Error retrieving session - Session: %s, Error: %v", sessionID, err)
		return nil, fmt.Errorf("failed to retrieve session")
	}

	// Check if we've already reached the maximum iterations
	if session.CurrentIteration >= s.maxIterations {
		log.Printf("Maximum iterations reached, marking as completed - Session: %s, Iterations: %d", sessionID, session.CurrentIteration)
		session.Status = "completed"
		if err := s.repository.UpdateSession(ctx, session); err != nil {
			log.Printf("Error updating completed session - Session: %s, Error: %v", sessionID, err)
			return session, fmt.Errorf("failed to update completed session: %v", err)
		}
		return session, nil
	}

	// Get current agent info to check for system changes
	agentObjectID, err := bson.ObjectIDFromHex(session.AgentID)
	if err != nil {
		log.Printf("Invalid agent ID format in session - Session: %s, Agent: %s", sessionID, session.AgentID)
		return nil, fmt.Errorf("invalid agent ID format")
	}

	agentInfo, err := s.agentService.GetAgentInfoByID(ctx, agentObjectID)
	if err != nil {
		log.Printf("Error getting agent info - Session: %s, Agent: %s, Error: %v", sessionID, session.AgentID, err)
		return nil, fmt.Errorf("failed to get agent info: %v", err)
	}

	req := &DiagnosticRequest{
		Issue:          session.InitialIssue,
		SystemMetrics:  &agentInfo.SystemMetrics,
		CommandResults: results,
		Iteration:      session.CurrentIteration + 1,
	}

	log.Printf("Diagnosing next iteration - Session: %s, Iteration: %d", sessionID, req.Iteration)
	resp, err := s.client.DiagnoseIssue(req)
	if err != nil {
		log.Printf("Error during diagnosis - Session: %s, Iteration: %d, Error: %v", sessionID, req.Iteration, err)
		// On AI service error, still increment iteration but don't add response
		session.CurrentIteration++
		session.UpdatedAt = time.Now()

		if session.CurrentIteration >= s.maxIterations {
			log.Printf("Maximum iterations reached after error - Session: %s", sessionID)
			session.Status = "completed"
		}

		if err := s.repository.UpdateSession(ctx, session); err != nil {
			log.Printf("Error updating session after diagnosis error - Session: %s, Error: %v", sessionID, err)
			return session, fmt.Errorf("failed to update session in database: %v", err)
		}
		return session, err
	}

	// Check if system metrics have changed significantly
	lastMetrics := session.History[len(session.History)-1].SystemSnapshot
	if s.agentService.HasSystemMetricsChanged(*lastMetrics, agentInfo.SystemMetrics) {
		log.Printf("Significant system metrics changes detected - Session: %s", sessionID)
		resp.NextStep += "\n[ALERT] Significant system changes detected since last check."
	}

	// Store current system metrics with the diagnostic response
	resp.SystemSnapshot = &agentInfo.SystemMetrics
	resp.IterationCount = session.CurrentIteration + 1
	session.History = append(session.History, *resp)
	session.CurrentIteration++
	session.UpdatedAt = time.Now()

	// Mark as completed if we've reached max iterations
	if session.CurrentIteration >= s.maxIterations {
		log.Printf("Maximum iterations reached, marking as completed - Session: %s", sessionID)
		session.Status = "completed"
	}

	log.Printf("Updating session with new diagnosis - Session: %s, Iteration: %d, Type: %s",
		sessionID, session.CurrentIteration, resp.DiagnosisType)
	if err := s.repository.UpdateSession(ctx, session); err != nil {
		log.Printf("Error updating session with new diagnosis - Session: %s, Error: %v", sessionID, err)
		return session, fmt.Errorf("failed to update session in database: %v", err)
	}

	return session, nil
}

// DeleteSession deletes a diagnostic session and all its associated data.
func (s *DiagnosticService) DeleteSession(ctx context.Context, sessionID string, userID string) error {
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		log.Printf("Invalid session ID format - Session: %s", sessionID)
		return fmt.Errorf("invalid session ID format: %v", err)
	}

	// First get the session to verify ownership
	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Session not found - Session: %s", sessionID)
			return fmt.Errorf("session not found")
		}
		log.Printf("Error retrieving session for deletion - Session: %s, Error: %v", sessionID, err)
		return err
	}

	// Verify ownership
	if session.UserID != userID {
		log.Printf("User does not own session - User: %s, Session: %s", userID, sessionID)
		return fmt.Errorf("user does not own this session")
	}

	// Delete the session
	log.Printf("Deleting session - Session: %s", sessionID)
	if err := s.repository.DeleteSession(ctx, id); err != nil {
		log.Printf("Error deleting session - Session: %s, Error: %v", sessionID, err)
		return fmt.Errorf("failed to delete session: %v", err)
	}

	log.Printf("Session deleted successfully - Session: %s", sessionID)
	return nil
}

// ListUserSessions returns all diagnostic sessions for a user.
func (s *DiagnosticService) ListUserSessions(ctx context.Context, userID string) ([]*DiagnosticSession, error) {
	log.Printf("Listing sessions for user - User: %s", userID)
	filter := bson.M{"user_id": userID}
	return s.repository.ListSessions(ctx, filter)
}

// GetDiagnosticSession retrieves a diagnostic session by ID.
func (s *DiagnosticService) GetDiagnosticSession(ctx context.Context, sessionID string) (*DiagnosticSession, error) {
	log.Printf("Retrieving diagnostic session - Session: %s", sessionID)
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		log.Printf("Invalid session ID format - Session: %s", sessionID)
		return nil, fmt.Errorf("invalid session ID format %v", err)
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Session not found - Session: %s", sessionID)
			return nil, fmt.Errorf("session not found")
		}
		log.Printf("Error retrieving session - Session: %s, Error: %v", sessionID, err)
		return nil, fmt.Errorf("failed to retrieve session %v", err)
	}
	log.Printf("Session retrieved successfully - Session: %s", sessionID)
	return session, nil
}

// GetDiagnosticSummary generates a summary of the diagnostic session.
func (s *DiagnosticService) GetDiagnosticSummary(ctx context.Context, sessionID string) (string, error) {
	log.Printf("Generating diagnostic summary - Session: %s", sessionID)
	id, err := bson.ObjectIDFromHex(sessionID)
	if err != nil {
		log.Printf("Invalid session ID format - Session: %s", sessionID)
		return "", fmt.Errorf("invalid session ID format %v", err)
	}

	session, err := s.repository.GetSession(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("Session not found - Session: %s", sessionID)
			return "", fmt.Errorf("session not found")
		}
		log.Printf("Error retrieving session for summary - Session: %s, Error: %v", sessionID, err)
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

	log.Printf("Diagnostic summary generated successfully - Session: %s", sessionID)
	return summary, nil
}
