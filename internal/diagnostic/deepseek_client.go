package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

const (
	model            = "deepseek-chat"
	baseURL          = "https://api.deepseek.com/v1"
	initialMaxTokens = 500  // Increased from 100 to handle full command responses
	fullMaxTokens    = 2048 // For detailed analysis responses
)

// DeepSeekClient handles interactions with the DeepSeek API
type DeepSeekClient struct {
	client *openai.Client
	ctx    context.Context
}

// NewDeepSeekClient creates a new DeepSeek API client
func NewDeepSeekClient(apiKey string) *DeepSeekClient {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	return &DeepSeekClient{
		client: openai.NewClientWithConfig(config),
		ctx:    context.Background(),
	}
}

// buildSystemPrompt creates the system prompt for Linux diagnostics
func (c *DeepSeekClient) buildSystemPrompt() string {
	return `You are a Linux expert. Only respond to Linux/Bash queries. Reject others with: '[ERROR] Non-Linux input.'. Return ONLY JSON. Follow this schema:
{
  "diagnosis_type": "cpu|memory|disk|etc",
  "commands": [{"command": "safe_command", "timeout_seconds": 5}],
  "log_checks": [{"log_path": "/path", "grep_pattern": "pattern"}],
  "next_step": "string"
}
Rules:
1. Only suggest safe, read-only commands.
2. Add timeouts to infinite commands (e.g., vmstat 1 5).
3. Never suggest rm, dd, mkfs, or modifying commands.`
}

// buildUserPrompt creates the user prompt with diagnostic context
func (c *DeepSeekClient) buildUserPrompt(req *DiagnosticRequest) string {
	systemInfo := make([]string, 0, len(req.SystemInfo))
	for k, v := range req.SystemInfo {
		systemInfo = append(systemInfo, fmt.Sprintf("%s: %s", k, v))
	}

	// For subsequent iterations, include command results and previous findings
	if req.Iteration > 0 && len(req.CommandResults) > 0 {
		return fmt.Sprintf(
			"Analyze these Linux command results for the issue '%s':\n\n%s",
			req.Issue,
			strings.Join(req.CommandResults, "\n"),
		)
	}

	// Initial request
	return fmt.Sprintf(
		"Suggest diagnostic commands for this Linux issue:\nIssue: %s\nSystem Info:\n%s",
		req.Issue,
		strings.Join(systemInfo, "\n"),
	)
}

// DiagnoseIssue sends a diagnostic request to DeepSeek API
func (c *DeepSeekClient) DiagnoseIssue(req *DiagnosticRequest) (*DiagnosticResponse, error) {
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: c.buildSystemPrompt(),
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: c.buildUserPrompt(req),
		},
	}

	maxTokens := initialMaxTokens
	if req.Iteration > 0 {
		maxTokens = fullMaxTokens
	}

	resp, err := c.client.CreateChatCompletion(
		c.ctx,
		openai.ChatCompletionRequest{
			Model:     model,
			Messages:  messages,
			MaxTokens: maxTokens,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get DeepSeek response: %v", err)
	}

	content := resp.Choices[0].Message.Content

	// Find the JSON content within markdown blocks if present
	if idx := strings.Index(content, "{"); idx >= 0 {
		if endIdx := strings.LastIndex(content, "}"); endIdx > idx {
			content = content[idx : endIdx+1]
		}
	}

	// Clean up any remaining markdown or whitespace
	content = strings.TrimSpace(content)

	var diagnosticResp DiagnosticResponse
	if err := json.Unmarshal([]byte(content), &diagnosticResp); err != nil {
		return nil, fmt.Errorf("failed to parse DeepSeek response: %v\nResponse content: %s", err, resp.Choices[0].Message.Content)
	}

	diagnosticResp.IterationCount = req.Iteration
	diagnosticResp.Timestamp = time.Now()
	return &diagnosticResp, nil
}
