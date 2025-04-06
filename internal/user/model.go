package user

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID           bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Email        string        `json:"email" bson:"email"`
	Name         string        `json:"name" bson:"name"`
	AvatarURL    string        `json:"avatar_url" bson:"avatar_url"`
	HTMLURL      string        `json:"html_url" bson:"html_url"`
	LastLoggedIn time.Time     `json:"last_logged_in" bson:"last_logged_in"`
}

type GitHubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

// AgentInfo represents the information ingested by the agent.
type AgentInfo struct {
	ID            bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Email         string        `json:"email" bson:"email"`
	Hostname      string        `json:"hostname" bson:"hostname"`
	IPAddress     string        `json:"ip_address" bson:"ip_address"`
	KernelVersion string        `json:"kernel_version" bson:"kernel_version"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
}
