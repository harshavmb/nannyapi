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

func (r *AgentInfoRepository) GetAgents(ctx context.Context, email string) ([]*AgentInfo, error) {
	cursor, err := r.collection.Find(ctx, bson.M{
		"email": email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve agents: %v", err)
	}
	defer cursor.Close(ctx)

	var agents []*AgentInfo
	if err := cursor.All(ctx, &agents); err != nil {
		return nil, fmt.Errorf("failed to decode agents: %v", err)
	}
	return agents, nil
}
