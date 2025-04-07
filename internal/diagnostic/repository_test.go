package diagnostic

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db"
	testCollectionName = "diagnostic_sessions"
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

func TestDiagnosticRepository(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewDiagnosticRepository(client.Database(testDBName))

	// Create a test session
	session := &DiagnosticSession{
		InitialIssue:     "High CPU usage",
		CurrentIteration: 0,
		MaxIterations:    3,
		Status:           "in_progress",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
		History:          []DiagnosticResponse{*mockDiagnosticResponse()},
	}

	// Test CreateSession
	sessionID, err := repo.CreateSession(context.Background(), session)
	assert.NoError(t, err)

	// Test GetSession
	retrievedSession, err := repo.GetSession(context.Background(), sessionID)
	assert.NoError(t, err)
	assert.Equal(t, sessionID, retrievedSession.ID)
	assert.Equal(t, session.InitialIssue, retrievedSession.InitialIssue)
	assert.Equal(t, len(session.History), len(retrievedSession.History))

	// Test UpdateSession
	session.CurrentIteration = 1
	session.Status = "completed"
	session.ID = sessionID
	err = repo.UpdateSession(context.Background(), session)
	assert.NoError(t, err)

	// Verify update
	updatedSession, err := repo.GetSession(context.Background(), sessionID)
	assert.NoError(t, err)
	assert.Equal(t, 1, updatedSession.CurrentIteration)
	assert.Equal(t, "completed", updatedSession.Status)

	// Test non-existent session
	_, err = repo.GetSession(context.Background(), bson.NewObjectID())
	assert.Error(t, err)
	assert.Equal(t, err.Error(), mongo.ErrNoDocuments.Error())
}
