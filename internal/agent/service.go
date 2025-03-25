package agent

import (
	"context"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type AgentInfoService struct {
	repo *AgentInfoRepository
}

func NewAgentInfoService(repo *AgentInfoRepository) *AgentInfoService {
	return &AgentInfoService{repo: repo}
}

func (s *AgentInfoService) SaveAgentInfo(ctx context.Context, agentInfo AgentInfo) (*mongo.InsertOneResult, error) {
	insertInfo, err := s.repo.InsertAgentInfo(ctx, &agentInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to save agent info: %v", err)
	}
	log.Printf("Saved agent info: %s", insertInfo.InsertedID)
	return insertInfo, nil
}

func (s *AgentInfoService) GetAgentInfoByID(ctx context.Context, id bson.ObjectID) (*AgentInfo, error) {
	agentInfo, err := s.repo.GetAgentInfoByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return agentInfo, nil
}

func (s *AgentInfoService) GetAgents(ctx context.Context, userID string) ([]*AgentInfo, error) {
	agents, err := s.repo.GetAgents(ctx, userID)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return []*AgentInfo{}, nil // Return an empty slice instead of nil
		}
		return nil, err
	}
	return agents, nil
}
