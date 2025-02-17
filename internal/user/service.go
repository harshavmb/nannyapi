package user

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type UserService struct {
	userRepo      *UserRepository
	authTokenRepo *AuthTokenRepository
}

func NewUserService(userRepo *UserRepository, authTokenRepo *AuthTokenRepository) *UserService {
	return &UserService{
		userRepo:      userRepo,
		authTokenRepo: authTokenRepo,
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

func (s *UserService) CreateAuthToken(ctx context.Context, userEmail, encryptionKey string) (*AuthToken, error) {
	token, err := generateRandomToken(32)
	if err != nil {
		return nil, err
	}

	encryptedToken, err := encrypt(token, encryptionKey)
	if err != nil {
		return nil, err
	}

	return s.authTokenRepo.CreateAuthToken(ctx, encryptedToken, userEmail)
}

func (s *UserService) GetAuthToken(ctx context.Context, userEmail, encryptionKey string) (*AuthToken, error) {
	authToken, err := s.authTokenRepo.GetAuthTokenByEmail(ctx, userEmail)
	if err != nil {
		return nil, err
	}

	if authToken == nil {
		return nil, mongo.ErrNoDocuments // No auth token found
	}

	var decryptedToken string

	if !authToken.Retrieved {
		// First time retrieval, return plain-text token
		decryptedToken, err = decrypt(authToken.Token, encryptionKey)
		if err != nil {
			return nil, err
		}
		// Update the retrieved flag
		authToken.Retrieved = true
		err = s.authTokenRepo.UpdateAuthToken(ctx, authToken)
		if err != nil {
			return nil, err
		}
		authToken.Token = decryptedToken
		return authToken, nil
	}

	// Mask the token if already retrieved
	if authToken.Retrieved {
		decryptedToken, err = decrypt(authToken.Token, encryptionKey)
		if err != nil {
			return nil, err
		}
		if len(decryptedToken) <= 6 {
			authToken.Token = decryptedToken
			return authToken, nil // Return the whole token if it's too short
		}

		maskedToken := decryptedToken[:4] + "..." + decryptedToken[len(decryptedToken)-2:]

		authToken.Token = maskedToken
		return authToken, nil
	}

	return nil, nil
}
