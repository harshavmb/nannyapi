package agent

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// SystemMetrics represents the current system metrics.
type SystemMetrics struct {
	CPUInfo     []string          `json:"cpu_info" bson:"cpu_info"`         // CPU information from /proc/cpuinfo
	CPUUsage    float64           `json:"cpu_usage" bson:"cpu_usage"`       // Current CPU usage percentage
	MemoryTotal int64             `json:"memory_total" bson:"memory_total"` // Total memory in bytes
	MemoryUsed  int64             `json:"memory_used" bson:"memory_used"`   // Used memory in bytes
	MemoryFree  int64             `json:"memory_free" bson:"memory_free"`   // Free memory in bytes
	DiskUsage   map[string]int64  `json:"disk_usage" bson:"disk_usage"`     // Disk usage per mount point in bytes
	FSUsage     map[string]string `json:"fs_usage" bson:"fs_usage"`         // Filesystem usage percentages
}

// AgentInfo represents the information ingested by the agent.
type AgentInfo struct {
	ID            bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID        string        `json:"user_id" bson:"user_id"`
	Hostname      string        `json:"hostname" bson:"hostname"`
	IPAddress     string        `json:"ip_address" bson:"ip_address"`
	KernelVersion string        `json:"kernel_version" bson:"kernel_version"`
	OsVersion     string        `json:"os_version" bson:"os_version"`
	SystemMetrics SystemMetrics `json:"system_metrics" bson:"system_metrics"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at" bson:"updated_at"`
}
