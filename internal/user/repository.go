package user

import (
	"context"
	"fmt"
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

func (r *UserRepository) FindUserByID(ctx context.Context, userId bson.ObjectID) (*User, error) {
	filter := bson.M{"_id": userId}
	var user User
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No user found
		}
		return nil, err
	}
	return &user, nil
}

func (r *AuthTokenRepository) CreateAuthToken(ctx context.Context, encryptedToken, userEmail, hashedToken string) (*AuthToken, error) {
	authToken := &AuthToken{
		Email:       userEmail,
		Token:       encryptedToken,
		CreatedAt:   time.Now(),
		HashedToken: hashedToken,
		Retrieved:   false,
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
	filter := bson.M{"token": authToken.Token}
	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": authToken})
	return err
}

func (r *AuthTokenRepository) GetAuthTokensByEmail(ctx context.Context, userEmail string) ([]AuthToken, error) {
	filter := bson.M{"email": userEmail}
	var authTokens []AuthToken
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No auth token found
		}
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var authToken AuthToken
		if err := cursor.Decode(&authToken); err != nil {
			return nil, err
		}
		authTokens = append(authTokens, authToken)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}
	return authTokens, nil
}

func (r *AuthTokenRepository) DeleteAuthToken(ctx context.Context, tokenID bson.ObjectID) error {
	filter := bson.M{"_id": tokenID}

	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return fmt.Errorf("error while deleting token %s : %w", tokenID, err)
	}

	return nil
}

// GetAuthTokenByToken retrieves an auth token by its token value.
func (r *AuthTokenRepository) GetAuthTokenByToken(ctx context.Context, token string) (*AuthToken, error) {

	filter := bson.M{"token": token}

	var authToken AuthToken
	err := r.collection.FindOne(ctx, filter).Decode(&authToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("auth token not found")
		}
		return nil, fmt.Errorf("failed to find auth token: %w", err)
	}

	return &authToken, nil
}

// GetUserByEmail retrieves a user by their email address.
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	filter := bson.M{"email": email}

	var user User
	err := r.collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return &user, nil
}

func (r *AuthTokenRepository) GetAuthTokenByHashedToken(ctx context.Context, hashedToken string) (*AuthToken, error) {
	filter := bson.M{"hashed_token": hashedToken}

	var authToken AuthToken
	err := r.collection.FindOne(ctx, filter).Decode(&authToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("auth token not found")
		}
		return nil, fmt.Errorf("failed to retrieve auth token: %v", err)
	}
	return &authToken, nil
}

// SHOULDN'T be used in this project as GitHub OAuth is used
func (r *UserRepository) CreateUser(ctx context.Context, user *User) (*mongo.InsertOneResult, error) {
	user.LastLoggedIn = time.Now()

	return r.collection.InsertOne(ctx, user)
}
