package agent

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// AgentInfo represents the information ingested by the agent
type AgentInfo struct {
	ID            bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID        string        `json:"user_id" bson:"user_id"`
	Hostname      string        `json:"hostname" bson:"hostname"`
	IPAddress     string        `json:"ip_address" bson:"ip_address"`
	KernelVersion string        `json:"kernel_version" bson:"kernel_version"`
	OsVersion     string        `json:"os_version" bson:"os_version"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
}
