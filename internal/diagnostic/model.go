package diagnostic

import (
	"time"
)

// DiagnosticCommand represents a Linux command with timeout
type DiagnosticCommand struct {
	Command        string `json:"command" bson:"command"`
	TimeoutSeconds int    `json:"timeout_seconds" bson:"timeout_seconds"`
}

// LogCheck represents a log file check with grep pattern
type LogCheck struct {
	LogPath     string `json:"log_path" bson:"log_path"`
	GrepPattern string `json:"grep_pattern" bson:"grep_pattern"`
}

// DiagnosticResponse represents the response from DeepSeek API
type DiagnosticResponse struct {
	DiagnosisType  string              `json:"diagnosis_type" bson:"diagnosis_type"`
	Commands       []DiagnosticCommand `json:"commands" bson:"commands"`
	LogChecks      []LogCheck          `json:"log_checks" bson:"log_checks"`
	NextStep       string              `json:"next_step" bson:"next_step"`
	Timestamp      time.Time           `json:"-" bson:"timestamp"`       // Internal use only
	IterationCount int                 `json:"-" bson:"iteration_count"` // Internal use only
}

// DiagnosticRequest represents a Linux system diagnostic request
type DiagnosticRequest struct {
	Issue           string            `json:"issue" bson:"issue"`
	SystemInfo      map[string]string `json:"system_info" bson:"system_info"`
	LogFiles        []string          `json:"log_files,omitempty" bson:"log_files,omitempty"`
	CommandResults  []string          `json:"command_results,omitempty" bson:"command_results,omitempty"`
	Iteration       int               `json:"iteration" bson:"iteration"`
	PreviousResults []string          `json:"previous_results,omitempty" bson:"previous_results,omitempty"`
}

// DiagnosticSession tracks the state of a diagnostic session
type DiagnosticSession struct {
	ID               string               `json:"id" bson:"_id"`
	InitialIssue     string               `json:"initial_issue" bson:"initial_issue"`
	CurrentIteration int                  `json:"current_iteration" bson:"current_iteration"`
	MaxIterations    int                  `json:"max_iterations" bson:"max_iterations"`
	History          []DiagnosticResponse `json:"history" bson:"history"`
	Status           string               `json:"status" bson:"status"`
	CreatedAt        time.Time            `json:"created_at" bson:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at" bson:"updated_at"`
}
