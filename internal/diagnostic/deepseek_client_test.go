package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSystemPrompt(t *testing.T) {
	client := &DeepSeekClient{}
	prompt := client.buildSystemPrompt()
	assert.Contains(t, prompt, "You are a Linux expert")
	assert.Contains(t, prompt, "Return ONLY JSON")
	assert.Contains(t, prompt, "diagnosis_type")
	assert.Contains(t, prompt, "commands")
	assert.Contains(t, prompt, "log_checks")
}

func TestBuildUserPrompt(t *testing.T) {
	client := &DeepSeekClient{}

	req := &DiagnosticRequest{
		Issue: "High CPU usage",
		SystemInfo: map[string]string{
			"OS":     "Ubuntu 22.04",
			"Kernel": "5.15.0",
			"CPU":    "Intel i7",
		},
		Iteration: 1,
		CommandResults: []string{
			"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
		},
	}

	// Test initial request prompt
	initialReq := *req
	initialReq.Iteration = 0
	initialReq.CommandResults = nil
	prompt := client.buildUserPrompt(&initialReq)
	assert.Contains(t, prompt, "Suggest diagnostic commands")
	assert.Contains(t, prompt, "High CPU usage")
	assert.Contains(t, prompt, "Ubuntu 22.04")

	// Test analysis prompt with command results
	analysisPrompt := client.buildUserPrompt(req)
	assert.Contains(t, analysisPrompt, "Analyze these Linux command results")
	assert.Contains(t, analysisPrompt, "load average: 2.15")
}

func TestNewDeepSeekClient(t *testing.T) {
	client := NewDeepSeekClient("test-api-key")
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.NotNil(t, client.ctx)
}
