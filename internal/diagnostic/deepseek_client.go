package diagnostic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"

	"github.com/harshavmb/nannyapi/internal/agent"
)

const (
	model            = "deepseek-chat"
	baseURL          = "https://api.deepseek.com/v1"
	initialMaxTokens = 500  // Increased from 100 to handle full command responses
	fullMaxTokens    = 2048 // For detailed analysis responses
	temparature      = 0.2
)

// DeepSeekClient handles interactions with the DeepSeek API.
type DeepSeekClient struct {
	client *openai.Client
	ctx    context.Context
}

// NewDeepSeekClient creates a new DeepSeek API client.
func NewDeepSeekClient(apiKey string) *DeepSeekClient {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL

	return &DeepSeekClient{
		client: openai.NewClientWithConfig(config),
		ctx:    context.Background(),
	}
}

// buildSystemPrompt creates the system prompt for Linux diagnostics.
func (c *DeepSeekClient) buildSystemPrompt() string {
	return `You are a Linux expert specializing in system diagnostics. Return ONLY JSON following this schema:
{
  "diagnosis_type": "thread_deadlock|memory_leak|inode_exhaustion|database|network|unsupported",
  "commands": [{"command": "safe_command", "timeout_seconds": 5}],
  "log_checks": [{"log_path": "/path", "grep_pattern": "pattern"}],
  "next_step": "detailed_guidance",
  "root_cause": "specific_technical_cause",
  "severity": "high|medium|low",
  "impact": "impact_description"
}

Special Cases - EXACT Response Requirements:

1. For ambiguous/insufficient information:
   diagnosis_type: "unsupported"
   next_step: MUST start with "Insufficient information to determine specific issue."
   commands: []

2. For hardware issues:
   diagnosis_type: "unsupported"
   next_step: MUST start with "This issue requires physical hardware inspection."
   commands: []

3. For non-Linux issues:
   diagnosis_type: "unsupported"
   next_step: MUST start with "This issue is outside the scope of Linux diagnostics."
   commands: []

4. For CPU thread issues:
   diagnosis_type: "thread_deadlock"
   next_step: MUST include ALL terms:
   - thread state
   - deadlock detection
   - process monitoring
   - lock analysis
   - contention patterns
   commands: [process investigation commands]

5. For filesystem issues:
   diagnosis_type: "inode_exhaustion"
   next_step: MUST include ALL terms:
   - inode analysis
   - log rotation
   - filesystem cleanup
   - disk space
   - file management
   commands: [filesystem analysis commands]

General Rules:
1. Never suggest destructive commands
2. Maximum 3 commands per iteration
3. Always include specific metrics
4. Reference exact PIDs when available
5. For unsupported cases, use EXACT phrases as specified above`
}

// buildUserPrompt creates the user prompt with diagnostic context.
func (c *DeepSeekClient) buildUserPrompt(req *DiagnosticRequest) string {
	if req.Iteration > 0 && len(req.CommandResults) > 0 {
		var analysisGuidance string
		context := fmt.Sprintf("Original Issue: %s\n\nPrevious Context: %s\n\n",
			req.Issue,
			"Please maintain focus on the original issue. Ignore irrelevant inputs that do not contribute to diagnosis.")

		switch {
		case strings.Contains(strings.ToLower(req.Issue), "database"):
			analysisGuidance = context + "Analyze PostgreSQL Database Performance:\n" +
				"REQUIRED Response Elements:\n" +
				"1. Use diagnosis_type='database'\n" +
				"2. Include ALL terms in next_step:\n" +
				"   - Analysis of disk I/O patterns from iostat\n" +
				"   - PostgreSQL process and connections\n" +
				"   - Database connection states and pools\n" +
				"   - Query performance and execution time\n" +
				"   - Process monitoring for PID " + extractPID(req.CommandResults) + "\n" +
				"   - Database metrics and performance\n" +
				"   - Connection pool utilization\n" +
				"   - Query analysis and optimization\n" +
				"3. Reference specific metrics from results\n" +
				"4. Provide actionable performance insights"

		case strings.Contains(strings.ToLower(req.Issue), "network") ||
			strings.Contains(strings.ToLower(req.Issue), "connection"):
			analysisGuidance = context + "Analyze Network Performance:\n" +
				"REQUIRED Response Elements:\n" +
				"1. Use diagnosis_type='network'\n" +
				"2. Include ALL terms in next_step:\n" +
				"   - TCP flags and connection states\n" +
				"   - Network latency measurements\n" +
				"   - Socket buffer analysis\n" +
				"   - Packet monitoring results\n" +
				"   - Connection tracking details\n" +
				"   - Network performance metrics\n" +
				"   - Process " + extractPID(req.CommandResults) + " analysis\n" +
				"   - Port and connection statistics\n" +
				"3. Reference specific metrics from results\n" +
				"4. Provide clear next troubleshooting steps"

		case strings.Contains(strings.ToLower(req.Issue), "memory"):
			analysisGuidance = context + "Analyze Memory Usage:\n" +
				"REQUIRED Response Elements:\n" +
				"1. Use diagnosis_type='memory_leak'\n" +
				"2. Include ALL terms in next_step:\n" +
				"   - Memory leak detection analysis\n" +
				"   - Heap usage patterns\n" +
				"   - Cache utilization behavior\n" +
				"   - Buffer allocation tracking\n" +
				"   - Memory consumption trends\n" +
				"   - Process " + extractPID(req.CommandResults) + " monitoring\n" +
				"   - Growth pattern analysis\n" +
				"   - Garbage collection impact\n" +
				"   - Virtual memory utilization\n" +
				"3. Reference specific metrics from results\n" +
				"4. Provide memory optimization guidance"

		default:
			analysisGuidance = context + "System Analysis Requirements:\n" +
				"1. Use appropriate diagnosis_type\n" +
				"2. Include relevant system metrics\n" +
				"3. Reference process " + extractPID(req.CommandResults) + "\n" +
				"4. Provide specific next steps"
		}

		return fmt.Sprintf(
			"Analyze these Linux command results for issue '%s'.\n\nResponse Requirements:\n%s\n\nCommand Results:\n%s\n\n"+
				"Your response MUST include ALL required terms in the analysis guidance and stay focused on the original issue.",
			req.Issue,
			analysisGuidance,
			strings.Join(req.CommandResults, "\n"),
		)
	}

	// Initial request handling
	var systemInfo []string
	if req.SystemMetrics != nil {
		totalMemoryGiB := float64(req.SystemMetrics.MemoryTotal) / (1024 * 1024 * 1024)
		usedMemoryGiB := float64(req.SystemMetrics.MemoryUsed) / (1024 * 1024 * 1024)
		freeMemoryGiB := float64(req.SystemMetrics.MemoryFree) / (1024 * 1024 * 1024)
		memUsagePercent := (usedMemoryGiB / totalMemoryGiB) * 100

		systemInfo = append(systemInfo,
			fmt.Sprintf("Memory: Total: %.2f GiB, Used: %.2f GiB (%.1f%%), Free: %.2f GiB",
				totalMemoryGiB, usedMemoryGiB, memUsagePercent, freeMemoryGiB))

		if req.SystemMetrics.CPUUsage > 0 {
			systemInfo = append(systemInfo, fmt.Sprintf("CPU Usage: %.1f%%", req.SystemMetrics.CPUUsage))
		}

		for mountPoint, usage := range req.SystemMetrics.DiskUsage {
			usageGiB := float64(usage) / (1024 * 1024 * 1024)
			systemInfo = append(systemInfo, fmt.Sprintf("Disk (%s): %.2f GiB", mountPoint, usageGiB))
		}
	}

	var analysisType string
	var requiredTerms string
	switch {
	case strings.Contains(strings.ToLower(req.Issue), "database"):
		analysisType = "database"
		requiredTerms = "- disk i/o, postgresql, connections, query performance, process monitoring\n"
	case strings.Contains(strings.ToLower(req.Issue), "network"):
		analysisType = "network"
		requiredTerms = "- tcp flags, connection analysis, latency, socket buffers, packet monitoring\n"
	case strings.Contains(strings.ToLower(req.Issue), "memory"):
		analysisType = "memory_leak"
		requiredTerms = "- memory leak, heap, cache, buffer, memory consumption, process monitoring\n"
	}

	return fmt.Sprintf(
		"Analyze Linux system for issue '%s'.\n\nSystem State:\n%s\n\n"+
			"Analysis Type: %s\n%s\n"+
			"Suggest diagnostic commands to investigate this issue.\n"+
			"Your response MUST use the correct diagnosis_type and include ALL required terms.",
		req.Issue,
		strings.Join(systemInfo, "\n"),
		analysisType,
		requiredTerms,
	)
}

// extractPID extracts process ID from command results.
func extractPID(results []string) string {
	for _, line := range results {
		if strings.Contains(line, "PID") {
			fields := strings.Fields(line)
			for i, field := range fields {
				if field == "PID" && i+1 < len(fields) {
					return fields[i+1]
				}
			}
		}
	}
	return "N/A"
}

// DiagnoseIssue sends a diagnostic request to DeepSeek API.
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
			Model:       model,
			Messages:    messages,
			MaxTokens:   maxTokens,
			Temperature: temparature,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get DeepSeek response: %v", err)
	}

	content := resp.Choices[0].Message.Content

	// Extract JSON content, handling potential markdown formatting
	content = extractJSONContent(content)

	var diagnosticResp DiagnosticResponse
	if err := json.Unmarshal([]byte(content), &diagnosticResp); err != nil {
		return nil, fmt.Errorf("failed to parse DeepSeek response: %v\nResponse content: %s", err, content)
	}

	// Enrich response with metadata and context
	diagnosticResp.IterationCount = req.Iteration
	diagnosticResp.Timestamp = time.Now()
	diagnosticResp.SystemSnapshot = req.SystemMetrics

	// Set severity if not provided based on metrics
	if diagnosticResp.Severity == "" {
		diagnosticResp.Severity = determineSeverity(req.SystemMetrics, diagnosticResp.DiagnosisType)
	}

	return &diagnosticResp, nil
}

// extractJSONContent extracts JSON content from potential markdown formatting.
func extractJSONContent(content string) string {
	// Find content between triple backticks if present
	if start := strings.Index(content, "```json"); start != -1 {
		content = content[start+len("```json"):]
		if end := strings.Index(content, "```"); end != -1 {
			content = content[:end]
		}
	}

	// Find the actual JSON content by finding the first { and last }
	startBrace := strings.Index(content, "{")
	if startBrace != -1 {
		endBrace := strings.LastIndex(content, "}")
		if endBrace > startBrace {
			content = content[startBrace : endBrace+1]
		}
	}

	return strings.TrimSpace(content)
}

// determineSeverity determines the severity based on system metrics and diagnosis type.
func determineSeverity(metrics *agent.SystemMetrics, diagnosisType string) string {
	if metrics == nil {
		return "medium" // Default if no metrics available
	}

	switch diagnosisType {
	case "thread_deadlock":
		return "high"
	case "memory_leak":
		memUsage := float64(metrics.MemoryUsed) / float64(metrics.MemoryTotal)
		if memUsage > 0.9 {
			return "high"
		} else if memUsage > 0.8 {
			return "medium"
		}
	case "inode_exhaustion":
		for _, usage := range metrics.FSUsage {
			if strings.HasPrefix(usage, "9") {
				return "high"
			}
		}
	case "cpu":
		if metrics.CPUUsage > 90 {
			return "high"
		} else if metrics.CPUUsage > 80 {
			return "medium"
		}
	}

	return "low"
}
