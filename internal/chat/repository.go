package chat

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type ChatRepository struct {
	collection *mongo.Collection
}

func NewChatRepository(db *mongo.Database) *ChatRepository {
	return &ChatRepository{
		collection: db.Collection("chats"),
	}
}

func (r *ChatRepository) InsertChat(ctx context.Context, chat *Chat) (*mongo.InsertOneResult, error) {
	chat.CreatedAt = time.Now()
	return r.collection.InsertOne(ctx, chat)
}

func (r *ChatRepository) UpdateChat(ctx context.Context, chatID bson.ObjectID, promptResponse PromptResponse) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": chatID}
	update := bson.M{
		"$push": bson.M{"history": promptResponse},
	}
	return r.collection.UpdateOne(ctx, filter, update)
}

func (r *ChatRepository) GetChatByID(ctx context.Context, chatID bson.ObjectID) (*Chat, error) {
	filter := bson.M{"_id": chatID}
	var chat Chat
	err := r.collection.FindOne(ctx, filter).Decode(&chat)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No chat found
		}
		return nil, err
	}
	return &chat, nil
}
