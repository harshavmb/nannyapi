package diagnostic

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/harshavmb/nannyapi/internal/agent"
)

// DiagnosticCommand represents a Linux command with timeout.
type DiagnosticCommand struct {
	Command        string `json:"command" bson:"command"`
	TimeoutSeconds int    `json:"timeout_seconds" bson:"timeout_seconds"`
}

// LogCheck represents a log file check with grep pattern.
type LogCheck struct {
	LogPath     string `json:"log_path" bson:"log_path"`
	GrepPattern string `json:"grep_pattern" bson:"grep_pattern"`
}

// DiagnosticResponse represents the response from DeepSeek API.
type DiagnosticResponse struct {
	DiagnosisType  string               `json:"diagnosis_type" bson:"diagnosis_type"`
	Commands       []DiagnosticCommand  `json:"commands" bson:"commands"`
	LogChecks      []LogCheck           `json:"log_checks" bson:"log_checks"`
	NextStep       string               `json:"next_step" bson:"next_step"`
	Timestamp      time.Time            `json:"-" bson:"timestamp"`
	IterationCount int                  `json:"-" bson:"iteration_count"`
	SystemSnapshot *agent.SystemMetrics `json:"system_snapshot" bson:"system_snapshot"`
	RootCause      string               `json:"root_cause,omitempty" bson:"root_cause,omitempty"`
	Severity       string               `json:"severity,omitempty" bson:"severity,omitempty"`
	Impact         string               `json:"impact,omitempty" bson:"impact,omitempty"`
}

// DiagnosticRequest represents a Linux system diagnostic request.
type DiagnosticRequest struct {
	Issue           string               `json:"issue" bson:"issue"`
	SystemMetrics   *agent.SystemMetrics `json:"system_metrics" bson:"system_metrics"`
	LogFiles        []string             `json:"log_files,omitempty" bson:"log_files,omitempty"`
	CommandResults  []string             `json:"command_results,omitempty" bson:"command_results,omitempty"`
	Iteration       int                  `json:"iteration" bson:"iteration"`
	PreviousResults []string             `json:"previous_results,omitempty" bson:"previous_results,omitempty"`
}

// DiagnosticSession tracks the state of a diagnostic session.
type DiagnosticSession struct {
	ID               bson.ObjectID        `json:"id" bson:"_id,omitempty"`
	AgentID          string               `json:"agent_id" bson:"agent_id"`
	UserID           string               `json:"user_id" bson:"user_id"`
	InitialIssue     string               `json:"initial_issue" bson:"initial_issue"`
	CurrentIteration int                  `json:"current_iteration" bson:"current_iteration"`
	MaxIterations    int                  `json:"max_iterations" bson:"max_iterations"`
	Status           string               `json:"status" bson:"status"`
	CreatedAt        time.Time            `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at" bson:"updated_at"`
	History          []DiagnosticResponse `json:"history" bson:"history"`
}
