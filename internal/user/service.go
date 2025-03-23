package user

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserService struct {
	userRepo *UserRepository
}

func NewUserService(userRepo *UserRepository) *UserService {
	return &UserService{
		userRepo: userRepo,
	}
}

func (s *UserService) SaveUser(ctx context.Context, userInfo map[string]interface{}) error {
	// Check if a user with the given email already exists
	existingUser, err := s.userRepo.FindUserByEmail(ctx, userInfo["email"].(string))
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
	_, err = s.userRepo.UpsertUser(ctx, user)
	if err != nil {
		log.Fatalf("Failed to save user: %v", err)
		return err
	}
	return nil
}

// GetUserByEmail retrieves a user by their email address.
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, err
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by their id.
func (s *UserService) GetUserByID(ctx context.Context, id bson.ObjectID) (*User, error) {
	user, err := s.userRepo.FindUserByID(ctx, id)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return user, nil
}

func (s *UserService) CreateUser(ctx context.Context, user User) (*mongo.InsertOneResult, error) {
	insertInfo, err := s.userRepo.CreateUser(ctx, &user)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info: %v", err)
	}
	log.Printf("Created user info: %s", insertInfo.InsertedID)
	return insertInfo, nil
}
