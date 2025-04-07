package chat

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Chat represents a chat document with prompts and responses.
type Chat struct {
	ID        bson.ObjectID    `json:"id" bson:"_id,omitempty"`
	AgentID   string           `json:"agent_id" bson:"agent_id"`
	History   []PromptResponse `json:"history" bson:"history"`
	CreatedAt time.Time        `json:"created_at" bson:"created_at"`
}

// PromptResponse represents a single prompt-response pair.
type PromptResponse struct {
	Prompt   string `json:"prompt" bson:"prompt"`
	Response string `json:"response" bson:"response"`
	Type     string `json:"type" bson:"type"` // "commands" or "text"
}

type AllowedCommand struct {
	Command string
	Args    []string
}
