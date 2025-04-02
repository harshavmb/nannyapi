package token

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type TokenRepository struct {
	collection *mongo.Collection
}

type RefreshTokenRepository struct {
	collection *mongo.Collection
}

func NewTokenRepository(db *mongo.Database) *TokenRepository {
	return &TokenRepository{
		collection: db.Collection("auth_tokens"),
	}
}

func NewRefreshTokenRepository(db *mongo.Database) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		collection: db.Collection("refresh_tokens"),
	}
}

// static tokens
func (r *TokenRepository) CreateToken(ctx context.Context, token Token) (*Token, error) {
	tokenResult, err := r.collection.InsertOne(ctx, token)
	if err != nil {
		return nil, err
	}

	if tokenResult.InsertedID != nil {
		token.ID = tokenResult.InsertedID.(bson.ObjectID)
	}

	return &token, nil
}

func (r *TokenRepository) GetTokensByUser(ctx context.Context, userID string) ([]*Token, error) {
	filter := bson.M{"user_id": userID}
	var tokens []*Token
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No Refresh tokens found
		}
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var token *Token
		if err := cursor.Decode(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}
	return tokens, nil
}

func (r *TokenRepository) GetToken(ctx context.Context, tokenId bson.ObjectID) (*Token, error) {
	filter := bson.M{"_id": tokenId}
	var token Token
	err := r.collection.FindOne(ctx, filter).Decode(&token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No auth token found
		}
		return nil, err
	}
	return &token, nil
}

func (r *TokenRepository) GetTokenByHashedToken(ctx context.Context, hashedToken string) (*Token, error) {
	filter := bson.M{"hashed_token": hashedToken}
	var token Token
	err := r.collection.FindOne(ctx, filter).Decode(&token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No auth token found
		}
		return nil, err
	}
	return &token, nil
}

func (r *TokenRepository) DeleteToken(ctx context.Context, tokenID bson.ObjectID) error {
	filter := bson.M{"_id": tokenID}

	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

func (r *TokenRepository) UpdateToken(ctx context.Context, token *Token) error {
	filter := bson.M{"token": token.Token}
	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": token})
	return err
}

// refresh tokens
func (r *RefreshTokenRepository) CreateRefreshToken(ctx context.Context, token RefreshToken) (*RefreshToken, error) {
	tokenResult, err := r.collection.InsertOne(ctx, token)
	if err != nil {
		return nil, err
	}

	if tokenResult.InsertedID != nil {
		token.ID = tokenResult.InsertedID.(bson.ObjectID)
	}

	return &token, nil
}

func (r *RefreshTokenRepository) UpdateRefreshToken(ctx context.Context, token *RefreshToken) error {
	filter := bson.M{"token": token.Token}
	_, err := r.collection.UpdateOne(ctx, filter, bson.M{"$set": token})
	return err
}

func (r *RefreshTokenRepository) GetRefreshToken(ctx context.Context, hashedToken string) (*RefreshToken, error) {
	filter := bson.M{"hashed_token": hashedToken}
	var token RefreshToken
	err := r.collection.FindOne(ctx, filter).Decode(&token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No refresh token found
		}
		return nil, err
	}
	return &token, nil
}

func (r *RefreshTokenRepository) DeleteRefreshToken(ctx context.Context, hashedToken string) error {
	filter := bson.M{"hashed_token": hashedToken}

	_, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	return nil
}

// Retrieves all refresh tokens associated with a given user ID.
// This is useful for user-initiated token revocation (e.g., "log out from all devices").
func (r *RefreshTokenRepository) GetRefreshTokensByUser(ctx context.Context, userID string) ([]*RefreshToken, error) {
	filter := bson.M{"user_id": userID}
	var tokens []*RefreshToken
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No Refresh tokens found
		}
		return nil, err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var token *RefreshToken
		if err := cursor.Decode(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}
	return tokens, nil
}
