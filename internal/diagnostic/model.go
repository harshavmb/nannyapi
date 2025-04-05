package diagnostic

import (
	"time"
)

// DiagnosticCommand represents a Linux command with timeout
type DiagnosticCommand struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// LogCheck represents a log file check with grep pattern
type LogCheck struct {
	LogPath     string `json:"log_path"`
	GrepPattern string `json:"grep_pattern"`
}

// DiagnosticResponse represents the response from DeepSeek API
type DiagnosticResponse struct {
	DiagnosisType  string              `json:"diagnosis_type"`
	Commands       []DiagnosticCommand `json:"commands"`
	LogChecks      []LogCheck          `json:"log_checks"`
	NextStep       string              `json:"next_step"`
	Timestamp      time.Time           `json:"-"` // Internal use only
	IterationCount int                 `json:"-"` // Internal use only
}

// DiagnosticRequest represents a Linux system diagnostic request
type DiagnosticRequest struct {
	Issue           string            `json:"issue"`
	SystemInfo      map[string]string `json:"system_info"`
	LogFiles        []string          `json:"log_files,omitempty"`
	CommandResults  []string          `json:"command_results,omitempty"`
	Iteration       int               `json:"iteration"`
	PreviousResults []string          `json:"previous_results,omitempty"`
}

// DiagnosticSession tracks the state of a diagnostic session
type DiagnosticSession struct {
	ID               string               `json:"id"`
	InitialIssue     string               `json:"initial_issue"`
	CurrentIteration int                  `json:"current_iteration"`
	MaxIterations    int                  `json:"max_iterations"`
	History          []DiagnosticResponse `json:"history"`
	Status           string               `json:"status"`
	CreatedAt        time.Time            `json:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at"`
}
