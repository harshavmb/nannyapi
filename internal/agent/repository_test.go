package agent

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db"
	testCollectionName = "agent_info"
)

func setupTestDB(t *testing.T) (*mongo.Client, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Cleanup function to drop the test database after tests
	cleanup := func() {
		err := client.Database(testDBName).Collection(testCollectionName).Drop(context.Background())
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
		err = client.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("Failed to disconnect from MongoDB: %v", err)
		}
	}

	return client, cleanup
}

func TestAgentInfoRepository(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAgentInfoRepository(client.Database(testDBName))

	t.Run("InsertAgentInfo", func(t *testing.T) {
		agentInfo := &AgentInfo{
			UserID:        "123456",
			Hostname:      "test-host",
			IPAddress:     "192.168.1.1",
			KernelVersion: "5.10.0",
			OsVersion:     "Ubuntu 24.04",
		}

		result, err := repo.InsertAgentInfo(context.Background(), agentInfo)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.InsertedID)

		// Verify the agent info was inserted
		insertedAgentInfo, err := repo.GetAgentInfoByID(context.Background(), result.InsertedID.(bson.ObjectID))
		assert.NoError(t, err)
		assert.NotNil(t, insertedAgentInfo)
		assert.Equal(t, agentInfo.UserID, insertedAgentInfo.UserID)
	})

	t.Run("GetAgentInfoByID", func(t *testing.T) {
		// Insert agent info
		agentInfo := &AgentInfo{
			UserID:        "123456",
			Hostname:      "findbyid-host",
			IPAddress:     "192.168.1.3",
			KernelVersion: "5.10.2",
			OsVersion:     "Ubuntu 24.04",
		}
		insertResult, err := repo.InsertAgentInfo(context.Background(), agentInfo)
		assert.NoError(t, err)

		// Fetch the inserted ID
		agentInfoID := insertResult.InsertedID.(bson.ObjectID)

		// Find the agent info by ID
		foundAgentInfo, err := repo.GetAgentInfoByID(context.Background(), agentInfoID)
		assert.NoError(t, err)
		assert.NotNil(t, foundAgentInfo)
		assert.Equal(t, agentInfo.UserID, foundAgentInfo.UserID)
	})

	t.Run("AgentInfoNotFoundByID", func(t *testing.T) {
		// Try to find agent info by non-existent ID
		nonExistentID := bson.NewObjectID()
		agentInfo, err := repo.GetAgentInfoByID(context.Background(), nonExistentID)
		assert.NoError(t, err)
		assert.Nil(t, agentInfo)
	})
}

func TestGetAgents(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAgentInfoRepository(client.Database(testDBName))

	t.Run("ValidAgents", func(t *testing.T) {
		// Insert test agents into the database
		agents := []AgentInfo{
			{
				UserID:        "123456",
				Hostname:      "host1",
				IPAddress:     "192.168.1.1",
				KernelVersion: "5.10.0",
				OsVersion:     "Ubuntu 24.04",
			},
			{
				UserID:        "123456",
				Hostname:      "host2",
				IPAddress:     "192.168.1.2",
				KernelVersion: "3.10.0",
				OsVersion:     "Ubuntu 18.04",
			},
			{
				UserID:        "654321",
				Hostname:      "host3",
				IPAddress:     "192.168.1.3",
				KernelVersion: "5.11.0",
				OsVersion:     "Ubuntu 22.04",
			},
		}

		for _, agent := range agents {
			_, err := repo.InsertAgentInfo(context.Background(), &agent)
			assert.NoError(t, err)
		}

		// Fetch agents
		result, err := repo.GetAgents(context.Background(), "123456")
		assert.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, "123456", result[0].UserID)
	})

	t.Run("NoAgents", func(t *testing.T) {
		// Ensure no agents exist in the database
		result, err := repo.GetAgents(context.Background(), "000000")
		assert.NoError(t, err)
		assert.Empty(t, result)
	})
}
