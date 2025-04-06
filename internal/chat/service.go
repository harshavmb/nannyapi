package chat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/harshavmb/nannyapi/internal/agent"
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

	// If the chat has history, process the response
	if chat.History != nil || len(chat.History) > 0 {
		if err := s.processPromptResponse(&chat.History[0]); err != nil {
			return nil, err
		}
	}

	insertInfo, err := s.repo.InsertChat(ctx, chat)
	if err != nil {
		return nil, err
	}
	log.Printf("Agent: %s Started chat: %s", insertInfo.InsertedID, chat.AgentID)
	return insertInfo, nil
}

func (s *ChatService) AddPromptResponse(ctx context.Context, chatID bson.ObjectID, promptResponse PromptResponse) (*Chat, error) {
	// Process the response
	if err := s.processPromptResponse(&promptResponse); err != nil {
		return nil, err
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

func (s *ChatService) processPromptResponse(promptResponse *PromptResponse) error {
	switch promptResponse.Type {
	case "commands":
		// Send commands to the agent and receive outputs
		commands := SendHealthCheckCommands()
		promptResponse.Response = strings.Join(commands, "\n")
	case "text":
		// Process the text response
		// (This is a placeholder for the actual implementation)
		commandOutput := promptResponse.Prompt
		log.Printf("command output recieved from agent: %s", commandOutput)
		promptResponse.Response = randomString(20)
	default:
		return fmt.Errorf("invalid response type: %s", promptResponse.Type)
	}
	return nil
}

// will be removed
// just faking an API response.
func randomString(length int) string {
	buff := make([]byte, int(math.Ceil(float64(length)/2)))
	if _, err := rand.Read(buff); err != nil {
		// G104 (CWE-703): Errors unhandled (Confidence: HIGH, Severity: LOW)
		// If we can't generate random bytes, log the error and return an empty string
		// This is a critical error that should be extremely rare
		log.Printf("Failed to generate random string: %v", err)
		return ""
	}
	str := hex.EncodeToString(buff)
	return str[:length] // strip 1 extra character we get from odd length results
}
