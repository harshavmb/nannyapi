package diagnostic

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/harshavmb/nannyapi/internal/agent"
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
		SystemMetrics: &agent.SystemMetrics{
			CPUInfo:     []string{"Intel i7-1165G7"},
			CPUUsage:    85.5,
			MemoryTotal: 16 * 1024 * 1024 * 1024,
			MemoryUsed:  14 * 1024 * 1024 * 1024,
			MemoryFree:  2 * 1024 * 1024 * 1024,
			DiskUsage: map[string]int64{
				"/": 250 * 1024 * 1024 * 1024,
			},
			FSUsage: map[string]string{
				"/": "85.5%",
			},
		},
		Iteration: 0,
	}

	// Test initial request prompt
	prompt := client.buildUserPrompt(req)
	assert.Contains(t, prompt, "Suggest diagnostic commands")
	assert.Contains(t, prompt, "85.5%")
	assert.Contains(t, prompt, "High CPU usage")
	assert.Contains(t, prompt, "Intel i7")

	// Test analysis prompt with command results
	req.Iteration = 1
	req.CommandResults = []string{
		"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
	}
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
