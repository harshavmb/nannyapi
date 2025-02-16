package user

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserService struct {
	repo *UserRepository
}

func NewUserService(repo *UserRepository) *UserService {
	return &UserService{
		repo: repo,
	}
}

func (s *UserService) SaveUser(ctx context.Context, userInfo map[string]interface{}) error {
	// Check if a user with the given email already exists
	existingUser, err := s.repo.FindUserByEmail(ctx, userInfo["email"].(string))
	if err != nil {
		log.Fatalf("Failed to find user by email: %v", err)
		return err
	}

	var userID bson.ObjectID
	if existingUser != nil {
		// Use the existing user's ID
		userID = existingUser.ID
	} else {
		// Create a new ID for the new user
		userID = bson.NewObjectID()
	}

	user := &User{
		ID:           userID,
		Email:        userInfo["email"].(string),
		Name:         userInfo["name"].(string),
		AvatarURL:    userInfo["avatar_url"].(string),
		HTMLURL:      userInfo["html_url"].(string),
		LastLoggedIn: time.Now(),
	}
	log.Printf("Saving user: %v", user.Email)
	_, err = s.repo.UpsertUser(ctx, user)
	if err != nil {
		log.Fatalf("Failed to save user: %v", err)
		return err
	}
	return nil
}

func (r *UserRepository) CreateAuthToken(ctx context.Context, userEmail string) (*AuthToken, error) {
	token, err := generateRandomToken(32)
	if err != nil {
		return nil, err
	}

	encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
	if encryptionKey == "" {
		return nil, fmt.Errorf("NANNY_ENCRYPTION_KEY not set")
	}

	encryptedToken, err := encrypt(token, encryptionKey)
	if err != nil {
		return nil, err
	}

	authToken := &AuthToken{
		Email:     userEmail,
		Token:     encryptedToken,
		CreatedAt: time.Now(),
	}

	collection := r.collection.Database().Collection("auth_tokens")
	tokenResult, err := collection.InsertOne(ctx, authToken)
	if err != nil {
		return nil, err
	}

	if tokenResult.Acknowledged {
		log.Printf("Created auth token for user %s", userEmail)
	}

	return authToken, nil
}

func (r *UserRepository) GetAuthToken(ctx context.Context, userEmail string) (*AuthToken, error) {
	collection := r.collection.Database().Collection("auth_tokens")
	if collection == nil {
		return nil, nil // Collections itself is nil
	}
	filter := bson.M{"email": userEmail}

	var authToken AuthToken
	err := collection.FindOne(ctx, filter).Decode(&authToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No auth token found
		}
		return nil, err
	}

	return &authToken, nil
}
