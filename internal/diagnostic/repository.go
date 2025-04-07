package diagnostic

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type DiagnosticRepository struct {
	collection *mongo.Collection
}

func NewDiagnosticRepository(db *mongo.Database) *DiagnosticRepository {
	collection := db.Collection("diagnostic_sessions")
	return &DiagnosticRepository{
		collection: collection,
	}
}

func (r *DiagnosticRepository) CreateSession(ctx context.Context, session *DiagnosticSession) (bson.ObjectID, error) {
	result, err := r.collection.InsertOne(ctx, session)
	if err != nil {
		return bson.ObjectID{}, fmt.Errorf("failed to create diagnostic session: %v", err)
	}
	return result.InsertedID.(bson.ObjectID), nil
}

func (r *DiagnosticRepository) UpdateSession(ctx context.Context, session *DiagnosticSession) error {
	filter := bson.M{"_id": session.ID}
	update := bson.M{"$set": session}
	_, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update diagnostic session: %v", err)
	}
	return nil
}

func (r *DiagnosticRepository) GetSession(ctx context.Context, sessionID bson.ObjectID) (*DiagnosticSession, error) {
	var session DiagnosticSession
	err := r.collection.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&session)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments
		}
		return nil, fmt.Errorf("failed to get diagnostic session: %v", err)
	}
	return &session, nil
}

func (r *DiagnosticRepository) ListSessions(ctx context.Context, filter bson.M) ([]*DiagnosticSession, error) {
	opts := options.Find().SetSort(bson.M{"created_at": -1})
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list diagnostic sessions: %v", err)
	}
	defer cursor.Close(ctx)

	var sessions []*DiagnosticSession
	if err = cursor.All(ctx, &sessions); err != nil {
		return nil, fmt.Errorf("failed to decode diagnostic sessions: %v", err)
	}
	return sessions, nil
}

func (r *DiagnosticRepository) DeleteSession(ctx context.Context, sessionID bson.ObjectID) error {
	filter := bson.M{"_id": sessionID}
	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete diagnostic session: %v", err)
	}
	if result.DeletedCount == 0 {
		return fmt.Errorf("session not found: %s", sessionID.Hex())
	}
	return nil
}
