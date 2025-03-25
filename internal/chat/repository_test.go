package chat

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
	testCollectionName = "chat"
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

func TestChatRepository(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewChatRepository(client.Database(testDBName))

	t.Run("InsertChat", func(t *testing.T) {
		agentId := bson.NewObjectID().Hex()
		chat := &Chat{
			AgentID: agentId,
			History: []PromptResponse{},
		}

		result, err := repo.InsertChat(context.Background(), chat)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.InsertedID)

		// Verify the chat was inserted
		insertedChat, err := repo.GetChatByID(context.Background(), result.InsertedID.(bson.ObjectID))
		assert.NoError(t, err)
		assert.NotNil(t, insertedChat)
		assert.Equal(t, agentId, insertedChat.AgentID)
	})

	t.Run("UpdateChat", func(t *testing.T) {
		// Insert a chat
		chat := &Chat{
			AgentID: bson.NewObjectID().Hex(),
			History: []PromptResponse{},
		}
		insertResult, err := repo.InsertChat(context.Background(), chat)
		assert.NoError(t, err)

		// Update the chat with a prompt-response pair
		chatID := insertResult.InsertedID.(bson.ObjectID)
		promptResponse := PromptResponse{
			Prompt:   "Hello",
			Response: "Hi there!",
		}
		updateResult, err := repo.UpdateChat(context.Background(), chatID, promptResponse)
		assert.NoError(t, err)
		assert.NotNil(t, updateResult)
		assert.Equal(t, int64(1), updateResult.ModifiedCount)

		// Verify the chat was updated
		updatedChat, err := repo.GetChatByID(context.Background(), chatID)
		assert.NoError(t, err)
		assert.NotNil(t, updatedChat)
		assert.Len(t, updatedChat.History, 1)
		assert.Equal(t, "Hello", updatedChat.History[0].Prompt)
		assert.Equal(t, "Hi there!", updatedChat.History[0].Response)
	})

	t.Run("GetChatByID", func(t *testing.T) {
		// Insert a chat
		agentId := bson.NewObjectID().Hex()
		chat := &Chat{
			AgentID: agentId,
			History: []PromptResponse{},
		}
		insertResult, err := repo.InsertChat(context.Background(), chat)
		assert.NoError(t, err)

		// Fetch the inserted ID
		chatID := insertResult.InsertedID.(bson.ObjectID)

		// Find the chat by ID
		foundChat, err := repo.GetChatByID(context.Background(), chatID)
		assert.NoError(t, err)
		assert.NotNil(t, foundChat)
		assert.Equal(t, agentId, foundChat.AgentID)
	})

	t.Run("ChatNotFoundByID", func(t *testing.T) {
		// Try to find chat by non-existent ID
		nonExistentID := bson.NewObjectID()
		chat, _ := repo.GetChatByID(context.Background(), nonExistentID)
		assert.Error(t, mongo.ErrNoDocuments)
		assert.Nil(t, chat)
	})
}
