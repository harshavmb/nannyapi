package chat

import (
	"context"
	"fmt"
	"log"

	"github.com/harshavmb/nannyapi/internal/agent"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type ChatService struct {
	repo             *ChatRepository
	agentInfoService *agent.AgentInfoService
}

func NewChatService(repo *ChatRepository, agentInfoService *agent.AgentInfoService) *ChatService {
	return &ChatService{repo: repo, agentInfoService: agentInfoService}
}

func (s *ChatService) StartChat(ctx context.Context, chat *Chat) (*mongo.InsertOneResult, error) {
	// validate whether agentId exists and is in the correct format
	agentIDFromInput, err := bson.ObjectIDFromHex(chat.AgentID)
	if err != nil {
		return nil, fmt.Errorf("agent_id isn't passed as an ObjectID: %v", err)
	}

	agentInfo, err := s.agentInfoService.GetAgentInfoByID(ctx, agentIDFromInput)
	if err != nil {
		return nil, err
	}

	if agentInfo == nil {
		return nil, nil
	}

	insertInfo, err := s.repo.InsertChat(ctx, chat)
	if err != nil {
		return nil, err
	}
	log.Printf("Agent: %s Started chat: %s", insertInfo.InsertedID, chat.AgentID)
	return insertInfo, nil
}

func (s *ChatService) AddPromptResponse(ctx context.Context, chatID bson.ObjectID, prompt, response string) (*Chat, error) {
	promptResponse := PromptResponse{
		Prompt:   prompt,
		Response: response,
	}
	_, err := s.repo.UpdateChat(ctx, chatID, promptResponse)
	if err != nil {
		return nil, err
	}
	return s.repo.GetChatByID(ctx, chatID)
}

func (s *ChatService) GetChatByID(ctx context.Context, chatID bson.ObjectID) (*Chat, error) {
	return s.repo.GetChatByID(ctx, chatID)
}
