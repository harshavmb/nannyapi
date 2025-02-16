package user

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type UserRepository struct {
	collection *mongo.Collection
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{
		collection: db.Collection("users"),
	}
}

func (r *UserRepository) UpsertUser(ctx context.Context, user *User) (*mongo.UpdateResult, error) {
	if user.ID.IsZero() {
		user.ID = bson.NewObjectID()
	}

	filter := bson.M{"_id": user.ID}
	update := bson.M{
		"$set": user,
		"$setOnInsert": bson.M{
			"created_at": time.Now(),
		},
	}
	opts := options.UpdateOne().SetUpsert(true)
	updateResult, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		log.Fatalf("Failed to insert/update doc to collection %s: %v", r.collection.Name(), err)
		return nil, err
	}

	if updateResult.UpsertedID != nil {
		log.Printf("New user inserted with ID: %v", updateResult.UpsertedID)
	} else {
		log.Printf("User updated with ID: %v", user.ID)
	}
	return updateResult, nil
}

func (r *UserRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	filter := bson.M{"email": email}
	var user User
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No user found
		}
		return nil, err
	}
	return &user, nil
}
