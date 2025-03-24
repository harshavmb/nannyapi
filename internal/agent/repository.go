package agent

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type AgentInfoRepository struct {
	collection *mongo.Collection
}

func NewAgentInfoRepository(db *mongo.Database) *AgentInfoRepository {
	return &AgentInfoRepository{
		collection: db.Collection("agent_info"),
	}
}

func (r *AgentInfoRepository) InsertAgentInfo(ctx context.Context, agentInfo *AgentInfo) (*mongo.InsertOneResult, error) {
	agentInfo.CreatedAt = time.Now()
	return r.collection.InsertOne(ctx, agentInfo)
}

func (r *AgentInfoRepository) GetAgentInfoByID(ctx context.Context, id bson.ObjectID) (*AgentInfo, error) {
	filter := bson.M{"_id": id}

	var agentInfo *AgentInfo
	err := r.collection.FindOne(ctx, filter).Decode(&agentInfo)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve agent: %v", err)
	}
	return agentInfo, nil
}

func (r *AgentInfoRepository) GetAgents(ctx context.Context, userID string) ([]*AgentInfo, error) {
	filter := bson.M{"user_id": userID}
	var agents []*AgentInfo
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments
		}
		return nil, fmt.Errorf("failed to retrieve agents: %v", err)
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var agent *AgentInfo
		if err := cursor.Decode(&agent); err != nil {
			return nil, err
		}
		agents = append(agents, agent)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}
	return agents, nil
}
