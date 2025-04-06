package agent

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type AgentInfoService struct {
	repository *AgentInfoRepository
}

func NewAgentInfoService(repository *AgentInfoRepository) *AgentInfoService {
	return &AgentInfoService{
		repository: repository,
	}
}

// SaveAgentInfo saves or updates agent information.
func (s *AgentInfoService) SaveAgentInfo(ctx context.Context, info AgentInfo) (*mongo.InsertOneResult, error) {
	// Check if agent exists
	var existingAgent *AgentInfo
	if !info.ID.IsZero() {
		var err error
		existingAgent, err = s.GetAgentInfoByID(ctx, info.ID)
		if err != nil && err != mongo.ErrNoDocuments {
			return nil, fmt.Errorf("failed to check existing agent: %v", err)
		}
	}

	info.UpdatedAt = time.Now()
	if existingAgent == nil {
		info.CreatedAt = info.UpdatedAt
		return s.repository.InsertAgentInfo(ctx, &info)
	}

	// Update existing agent
	err := s.repository.UpdateAgentInfo(ctx, &info)
	if err != nil {
		return nil, err
	}

	return &mongo.InsertOneResult{InsertedID: info.ID}, nil
}

// GetAgentInfoByID retrieves agent information by ID.
func (s *AgentInfoService) GetAgentInfoByID(ctx context.Context, id bson.ObjectID) (*AgentInfo, error) {
	return s.repository.GetAgentInfoByID(ctx, id)
}

// GetAgents retrieves agents by user ID.
func (s *AgentInfoService) GetAgents(ctx context.Context, userID string) ([]*AgentInfo, error) {
	agents, err := s.repository.GetAgents(ctx, userID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []*AgentInfo{}, nil // Return an empty slice instead of nil
		}
		return nil, err
	}
	return agents, nil
}

// HasSystemMetricsChanged checks if there are significant changes in system metrics.
func (s *AgentInfoService) HasSystemMetricsChanged(old, new SystemMetrics) bool {
	// CPU usage change threshold (5%)
	if abs(new.CPUUsage-old.CPUUsage) > 5.0 {
		return true
	}

	// Memory change threshold (10%)
	memoryThreshold := float64(old.MemoryTotal) * 0.10
	if abs(float64(new.MemoryUsed-old.MemoryUsed)) > memoryThreshold {
		return true
	}

	// Check disk usage changes (10% threshold)
	for mountPoint, newUsage := range new.DiskUsage {
		if oldUsage, exists := old.DiskUsage[mountPoint]; exists {
			threshold := float64(oldUsage) * 0.10
			if abs(float64(newUsage-oldUsage)) > threshold {
				return true
			}
		} else {
			// New mount point appeared
			return true
		}
	}

	// Check if any mount points disappeared
	for mountPoint := range old.DiskUsage {
		if _, exists := new.DiskUsage[mountPoint]; !exists {
			return true
		}
	}

	return false
}

// Helper function for absolute value of float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
