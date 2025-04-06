package diagnostic

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/harshavmb/nannyapi/internal/agent"
)

func setupTestService(t *testing.T) (*DiagnosticService, func(), string, string) {
	client, cleanup := setupTestDB(t)
	repo := NewDiagnosticRepository(client.Database(testDBName))
	agentRepo := agent.NewAgentInfoRepository(client.Database(testDBName))
	agentService := agent.NewAgentInfoService(agentRepo)

	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Fatal("DEEPSEEK_API_KEY environment variable is required")
	}

	service := NewDiagnosticService(apiKey, repo, agentService)

	// Create a test agent
	testUserID := "test_user_123"
	testAgent := &agent.AgentInfo{
		ID:            bson.NewObjectID(),
		UserID:        testUserID,
		Hostname:      "test-host",
		IPAddress:     "192.168.1.1",
		KernelVersion: "5.10.0",
		OsVersion:     "Ubuntu 24.04",
		CreatedAt:     time.Now(),
		SystemMetrics: agent.SystemMetrics{
			CPUInfo:     []string{"Intel i7-1165G7"},
			CPUUsage:    45.5,
			MemoryTotal: 16 * 1024 * 1024 * 1024, // 16GB
			MemoryUsed:  8 * 1024 * 1024 * 1024,  // 8GB
			MemoryFree:  8 * 1024 * 1024 * 1024,  // 8GB
			DiskUsage: map[string]int64{
				"/": 250 * 1024 * 1024 * 1024, // 250GB
			},
			FSUsage: map[string]string{
				"/": "45%",
			},
		},
	}

	insertResult, err := agentService.SaveAgentInfo(context.Background(), *testAgent)
	if err != nil {
		t.Fatalf("Failed to create test agent: %v", err)
	}

	return service, cleanup, insertResult.InsertedID.(bson.ObjectID).Hex(), testUserID
}

// mockDiagnosticResponse creates a mock response for testing.
func mockDiagnosticResponse() *DiagnosticResponse {
	return &DiagnosticResponse{
		DiagnosisType: "cpu",
		Commands: []DiagnosticCommand{
			{Command: "top -b -n 1", TimeoutSeconds: 5},
			{Command: "vmstat 1 5", TimeoutSeconds: 5},
		},
		LogChecks: []LogCheck{
			{LogPath: "/var/log/syslog", GrepPattern: "oom-killer"},
		},
		NextStep: "Analyze CPU usage patterns",
		SystemSnapshot: &agent.SystemMetrics{
			CPUInfo:     []string{"Intel i7-1165G7"},
			CPUUsage:    45.5,
			MemoryTotal: 16 * 1024 * 1024 * 1024,
			MemoryUsed:  8 * 1024 * 1024 * 1024,
			MemoryFree:  8 * 1024 * 1024 * 1024,
		},
	}
}

func TestNewDiagnosticService(t *testing.T) {
	service, cleanup, _, _ := setupTestService(t)
	defer cleanup()

	assert.NotNil(t, service)
	assert.NotNil(t, service.client)
	assert.NotNil(t, service.repository)
	assert.Equal(t, 3, service.maxIterations)
}

func TestStartDiagnosticSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID, issue)
	if err != nil {
		t.Fatalf("Failed to start diagnostic session: %v", err)
	}

	assert.NotEmpty(t, session.ID)
	assert.Equal(t, agentID, session.AgentID)
	assert.Equal(t, userID, session.UserID)
	assert.Equal(t, issue, session.InitialIssue)
	assert.Equal(t, 0, session.CurrentIteration)
	assert.Equal(t, 3, session.MaxIterations)
	assert.Equal(t, "in_progress", session.Status)
	assert.NotEmpty(t, session.History)

	// Verify system metrics are present in the first diagnostic response
	firstResponse := session.History[0]
	assert.NotNil(t, firstResponse.SystemSnapshot)
	assert.NotEmpty(t, firstResponse.SystemSnapshot.CPUInfo)
	assert.Greater(t, firstResponse.SystemSnapshot.CPUUsage, float64(0))

	// Verify session was stored in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), session.ID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, session.ID, storedSession.ID)
	assert.Equal(t, session.InitialIssue, storedSession.InitialIssue)
	assert.NotNil(t, storedSession.History[0].SystemSnapshot)
}

func TestContinueDiagnosticSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create an initial session
	initialSession := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	// Add system metrics to the initial response
	initialSession.History[0].SystemSnapshot = &agent.SystemMetrics{
		CPUUsage:    45.5,
		MemoryTotal: 16 * 1024 * 1024 * 1024,
		MemoryUsed:  8 * 1024 * 1024 * 1024,
	}

	sessionID, err := service.repository.CreateSession(context.Background(), initialSession)
	assert.NoError(t, err)
	initialSession.ID = sessionID

	// Continue the session with command results
	results := []string{
		"top - 14:30:00 up 7 days, load average: 2.15, 1.92, 1.74",
		"Tasks: 180 total, 2 running, 178 sleeping",
	}

	continuedSession, err := service.ContinueDiagnosticSession(context.Background(), sessionID.Hex(), results)
	assert.NoError(t, err)
	assert.NotNil(t, continuedSession)
	assert.Equal(t, sessionID, continuedSession.ID)
	assert.Equal(t, 1, continuedSession.CurrentIteration)
	assert.Equal(t, "in_progress", continuedSession.Status)
	assert.Len(t, continuedSession.History, 2)

	// Verify system metrics are captured in the new response
	latestResponse := continuedSession.History[len(continuedSession.History)-1]
	assert.NotNil(t, latestResponse.SystemSnapshot)
	assert.NotEmpty(t, latestResponse.SystemSnapshot.CPUInfo)
	assert.Greater(t, latestResponse.SystemSnapshot.CPUUsage, float64(0))

	// Test system metrics change detection
	if latestResponse.NextStep != "" && continuedSession.History[0].SystemSnapshot.CPUUsage != latestResponse.SystemSnapshot.CPUUsage {
		assert.Contains(t, latestResponse.NextStep, "ALERT")
	}
}

func TestSessionMaxIterations(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	results := []string{"Sample command output"}

	// Run through all iterations
	for i := 0; i < 3; i++ {
		var err error
		session, err = service.ContinueDiagnosticSession(context.Background(), sessionID.Hex(), results)
		if err != nil {
			t.Fatalf("Failed in iteration %d: %v", i, err)
		}
	}

	assert.Equal(t, "completed", session.Status)
	assert.Equal(t, 3, session.CurrentIteration)
	assert.NotEmpty(t, session.History)

	// Verify final state in MongoDB
	storedSession, err := service.GetDiagnosticSession(context.Background(), sessionID.Hex())
	assert.NoError(t, err)
	assert.Equal(t, "completed", storedSession.Status)
	assert.Equal(t, 3, storedSession.CurrentIteration)
}

func TestGetDiagnosticSummary(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session with history
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 1,
		MaxIterations:    3,
		Status:           "in_progress",
		History: []DiagnosticResponse{
			*mockDiagnosticResponse(),
			*mockDiagnosticResponse(),
		},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	summary, err := service.GetDiagnosticSummary(context.Background(), sessionID.Hex())
	assert.NoError(t, err)
	assert.Contains(t, summary, "High CPU usage")
	assert.Contains(t, summary, "Diagnostic Summary")
	assert.Contains(t, summary, "cpu")         // diagnosis_type
	assert.Contains(t, summary, "top -b -n 1") // command
}

func TestStartDiagnosticSessionWithInvalidAgent(t *testing.T) {
	service, cleanup, _, userID := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"

	// Test with non-existent agent ID
	nonExistentAgentID := bson.NewObjectID().Hex()
	_, err := service.StartDiagnosticSession(context.Background(), nonExistentAgentID, userID, issue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")

	// Test with invalid agent ID format
	_, err = service.StartDiagnosticSession(context.Background(), "invalid-id", userID, issue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid agent ID format")
}

func TestStartDiagnosticSessionWithWrongUser(t *testing.T) {
	service, cleanup, agentID, _ := setupTestService(t)
	defer cleanup()

	issue := "High CPU usage"

	// Test with wrong user ID
	wrongUserID := "wrong_user_123"
	_, err := service.StartDiagnosticSession(context.Background(), agentID, wrongUserID, issue)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "agent does not belong to user")
}

func TestDeleteSession(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Create a test session
	session := &DiagnosticSession{
		AgentID:          agentID,
		UserID:           userID,
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	sessionID, err := service.repository.CreateSession(context.Background(), session)
	assert.NoError(t, err)
	session.ID = sessionID

	// Test successful deletion
	err = service.DeleteSession(context.Background(), sessionID.Hex(), userID)
	assert.NoError(t, err)

	// Verify session was deleted
	_, err = service.GetDiagnosticSession(context.Background(), sessionID.Hex())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// Test deletion with wrong user
	wrongUserID := "wrong_user_123"
	err = service.DeleteSession(context.Background(), sessionID.Hex(), wrongUserID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")

	// Test deletion of non-existent session
	err = service.DeleteSession(context.Background(), bson.NewObjectID().Hex(), userID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "session not found")
}

// getTermVariations returns common variations of diagnostic terms.
func getTermVariations(term string) []string {
	variations := []string{term}
	switch term {
	case "disk i/o":
		variations = append(variations, "disk io", "iostat", "disk performance", "disk utilization")
	case "connection":
		variations = append(variations, "connections", "connected", "connection pool", "connection state")
	case "query performance":
		variations = append(variations, "query execution", "query analysis", "query plan", "slow query")
	case "process monitoring":
		variations = append(variations, "process status", "process analysis", "process stats", "monitor process")
	case "tcp flags":
		variations = append(variations, "tcp state", "tcp status", "connection flags", "tcp analysis")
	case "latency":
		variations = append(variations, "delay", "response time", "slow response", "rtt")
	case "socket buffer":
		variations = append(variations, "socket", "buffer size", "tcp buffer", "network buffer")
	case "packet monitoring":
		variations = append(variations, "packet analysis", "packet capture", "network packets", "tcpdump")
	case "memory leak":
		variations = append(variations, "memory growth", "increasing memory", "memory consumption", "memory trend")
	case "heap":
		variations = append(variations, "heap usage", "heap size", "heap analysis", "memory heap")
	case "cache":
		variations = append(variations, "cache usage", "cached memory", "buffer cache", "page cache")
	}
	return variations
}

// matchDiagnosticTerms checks if a sufficient percentage of relevant terms are present.
func matchDiagnosticTerms(t *testing.T, text string, expectedTerms []string, requiredMatchPercentage float64) bool {
	text = strings.ToLower(text)
	var matchedTerms []string

	for _, term := range expectedTerms {
		termVariations := getTermVariations(term)
		for _, variation := range termVariations {
			if strings.Contains(text, strings.ToLower(variation)) {
				matchedTerms = append(matchedTerms, term)
				break
			}
		}
	}

	matchPercentage := float64(len(matchedTerms)) / float64(len(expectedTerms)) * 100
	t.Logf("Matched terms (%d/%d - %.1f%%):", len(matchedTerms), len(expectedTerms), matchPercentage)
	for _, term := range matchedTerms {
		t.Logf("  - %s", term)
	}
	return matchPercentage >= requiredMatchPercentage
}

func TestComplexCPUDiagnosticScenario(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// CPU metrics indicating high usage and thread issues
	agentObjectID, err := bson.ObjectIDFromHex(agentID)
	assert.NoError(t, err)

	agentInfo, err := service.agentService.GetAgentInfoByID(context.Background(), agentObjectID)
	assert.NoError(t, err)

	agentInfo.SystemMetrics = agent.SystemMetrics{
		CPUInfo:  []string{"Intel i7-1165G7"},
		CPUUsage: 92.5,
	}

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID, "High CPU usage with thread contention issues")
	assert.NoError(t, err)

	// Thread-related diagnostic terms that must be present
	threadTerms := []string{
		"thread state",
		"deadlock detection",
		"process monitoring",
		"lock analysis",
		"contention patterns",
		"pid",
		"1234",
		"thread",
		"deadlock",
	}

	lastResponse := session.History[len(session.History)-1]
	assert.True(t, matchDiagnosticTerms(t, lastResponse.NextStep, threadTerms, 60.0),
		"Response should contain sufficient thread/deadlock-related terms")
	assert.Equal(t, "thread_deadlock", lastResponse.DiagnosisType)
}

func TestComplexFilesystemDiagnosticScenario(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID, "High inode usage on filesystem")
	assert.NoError(t, err)

	// Filesystem-related terms that must be present
	fsTerms := []string{
		"inode analysis",
		"log rotation",
		"filesystem cleanup",
		"disk space",
		"file management",
		"filesystem",
		"rotation",
	}

	lastResponse := session.History[len(session.History)-1]
	assert.True(t, matchDiagnosticTerms(t, lastResponse.NextStep, fsTerms, 60.0),
		"Response should contain sufficient filesystem/rotation-related terms")
	assert.Equal(t, "inode_exhaustion", lastResponse.DiagnosisType)
}

func TestComplexMemoryLeakDiagnosticScenario(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID,
		"Memory usage steadily increasing, possible memory leak")
	assert.NoError(t, err)

	results := []string{
		"free -m",
		"              total        used        free      shared  buff/cache   available",
		"Mem:          32768       28672        4096         546        8192        3550",
		"ps aux --sort=-%mem | head -n 5",
		"USER       PID  %CPU %MEM    VSZ   RSS TTY      STAT START   TIME COMMAND",
		"app      12345  25.5 75.5 16.2g 14.8g ?        Ssl  Apr05 132:12 /app/myapp",
	}

	session, err = service.ContinueDiagnosticSession(context.Background(), session.ID.Hex(), results)
	assert.NoError(t, err)

	// Core memory diagnostic terms
	memoryTerms := []string{
		"memory leak", "heap", "cache", "buffer",
		"memory consumption", "process monitoring", "12345",
		"growth pattern",
	}

	lastResponse := session.History[len(session.History)-1]
	assert.True(t, matchDiagnosticTerms(t, lastResponse.NextStep, memoryTerms, 60.0),
		"Response should contain sufficient memory-related terms")
	assert.Equal(t, "memory_leak", lastResponse.DiagnosisType)
}

func TestComplexDatabaseDiagnosticScenario(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID,
		"PostgreSQL database showing high I/O, connection timeouts and slow queries")
	assert.NoError(t, err)

	// First iteration: System and database stats
	results := []string{
		"iostat -x 1 5",
		"Device   rrqm/s  wrqm/s  r/s    w/s    rkB/s   wkB/s  avgqu-sz  await  r_await  w_await  svctm  %util",
		"sda      0.00    15.20   85.60  45.20  2850.40 1024.00  2.50     25.40  15.20   35.60    8.50   95.20",
		"ps aux | grep postgres",
		"postgres  1234   95.5  5.0  5962404 839892 ?   Ssl  Apr05 125:30 /usr/lib/postgresql/14/bin/postgres",
	}

	session, err = service.ContinueDiagnosticSession(context.Background(), session.ID.Hex(), results)
	assert.NoError(t, err)

	// Core database diagnostic terms
	dbTerms := []string{
		"disk i/o", "postgresql", "connection", "query performance",
		"process monitoring", "1234", "iostat", "disk utilization",
	}

	lastResponse := session.History[len(session.History)-1]
	assert.True(t, matchDiagnosticTerms(t, lastResponse.NextStep, dbTerms, 60.0),
		"Response should contain sufficient database-related terms")
	assert.Equal(t, "database", lastResponse.DiagnosisType)
}

func TestComplexTCPNetworkDiagnosticScenario(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID,
		"Web service showing TCP connection timeouts and high latency")
	assert.NoError(t, err)

	// First iteration: Network state
	results := []string{
		"netstat -s | grep -i retransmit",
		"    12385 segments retransmitted",
		"    1238 fast retransmits",
		"netstat -ntp | grep ESTABLISHED",
		"tcp 0 0 10.0.0.5:8080 10.0.0.100:36002 ESTABLISHED 3456/nginx",
		"ss -tin sport = :8080",
		"State    Recv-Q   Send-Q     Local Address:Port     Peer Address:Port",
		"ESTAB    0        456        10.0.0.5:8080          10.0.0.101:40001",
	}

	session, err = service.ContinueDiagnosticSession(context.Background(), session.ID.Hex(), results)
	assert.NoError(t, err)

	// Core network diagnostic terms
	networkTerms := []string{
		"tcp flags", "connection", "latency", "socket buffer",
		"packet monitoring", "3456", "network", "port 8080",
	}

	lastResponse := session.History[len(session.History)-1]
	assert.True(t, matchDiagnosticTerms(t, lastResponse.NextStep, networkTerms, 60.0),
		"Response should contain sufficient network-related terms")
	assert.Equal(t, "network", lastResponse.DiagnosisType)
}

// TestNegativeDiagnosticScenarios tests cases where the system should recognize limitations.
func TestNegativeDiagnosticScenarios(t *testing.T) {
	service, cleanup, agentID, userID := setupTestService(t)
	defer cleanup()

	// Test 1: Ambiguous issue description
	session, err := service.StartDiagnosticSession(context.Background(), agentID, userID, "System not working properly")
	assert.NoError(t, err)
	lastResponse := session.History[len(session.History)-1]
	assert.Equal(t, "unsupported", lastResponse.DiagnosisType)
	assert.True(t, strings.HasPrefix(lastResponse.NextStep, "Insufficient information to determine specific issue"),
		"Response should indicate insufficient information")

	// Test 2: Hardware issue
	session, err = service.StartDiagnosticSession(context.Background(), agentID, userID, "CPU fan making strange noise")
	assert.NoError(t, err)
	lastResponse = session.History[len(session.History)-1]
	assert.Equal(t, "unsupported", lastResponse.DiagnosisType)
	assert.True(t, strings.HasPrefix(lastResponse.NextStep, "This issue requires physical hardware inspection"),
		"Response should indicate physical inspection requirement")

	// Test 3: Non-Linux issue
	session, err = service.StartDiagnosticSession(context.Background(), agentID, userID, "Windows blue screen error")
	assert.NoError(t, err)
	lastResponse = session.History[len(session.History)-1]
	assert.Equal(t, "unsupported", lastResponse.DiagnosisType)
	assert.True(t, strings.HasPrefix(lastResponse.NextStep, "This issue is outside the scope of Linux diagnostics"),
		"Response should indicate non-Linux scope")
}
