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

type AuthTokenRepository struct {
	collection *mongo.Collection
}

func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{
		collection: db.Collection("users"),
	}
}

func NewAuthTokenRepository(db *mongo.Database) *AuthTokenRepository {
	return &AuthTokenRepository{
		collection: db.Collection("auth_tokens"),
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
		log.Fatalf("Failed to insert/update doc to collection: %v", err)
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

func (r *AuthTokenRepository) CreateAuthToken(ctx context.Context, encryptedToken, userEmail string) (*AuthToken, error) {
	authToken := &AuthToken{
		Email:     userEmail,
		Token:     encryptedToken,
		CreatedAt: time.Now(),
		Retrieved: false,
	}

	tokenResult, err := r.collection.InsertOne(ctx, authToken)
	if err != nil {
		return nil, err
	}

	if tokenResult.InsertedID != nil {
		log.Printf("Created auth token for user %s", userEmail)
	}

	return authToken, nil
}

func (r *AuthTokenRepository) GetAuthTokenByEmail(ctx context.Context, userEmail string) (*AuthToken, error) {
	filter := bson.M{"email": userEmail}
	var authToken AuthToken
	err := r.collection.FindOne(ctx, filter).Decode(&authToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No auth token found
		}
		return nil, err
	}
	return &authToken, nil
}

func (r *AuthTokenRepository) UpdateAuthToken(ctx context.Context, authToken *AuthToken) error {
	filter := bson.M{"email": authToken.Email}
	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": authToken})
	return err
}
