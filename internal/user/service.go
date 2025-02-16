package user

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
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
