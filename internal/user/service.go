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

	// Hash the token
	hashedToken := HashToken(token)

	encryptedToken, err := Encrypt(token, encryptionKey)
	if err != nil {
		return nil, err
	}

	return s.authTokenRepo.CreateAuthToken(ctx, encryptedToken, userEmail, hashedToken)
}

func (s *UserService) GetAuthToken(ctx context.Context, userEmail, encryptionKey string) (*AuthToken, error) {
	authToken, err := s.authTokenRepo.GetAuthTokenByEmail(ctx, userEmail)
	if err != nil {
		return nil, err
	}

	if authToken == nil {
		return nil, mongo.ErrNoDocuments // No auth token found
	}

	return authToken, nil
}

func (s *UserService) GetAllAuthTokens(context context.Context, email string) ([]AuthToken, error) {
	authTokens, err := s.authTokenRepo.GetAuthTokensByEmail(context, email)
	if err != nil {
		return nil, err
	}

	if len(authTokens) == 0 {
		return nil, nil
	}

	if len(authTokens) > 0 {
		return authTokens, nil
	}
	return nil, nil
}

func (s *UserService) DeleteAuthToken(context context.Context, objID bson.ObjectID) error {
	err := s.authTokenRepo.DeleteAuthToken(context, objID)
	if err != nil {
		return err
	}
	log.Println("Deleted auth token with ID:", objID)
	return nil
}

// GetAuthTokenByToken retrieves an auth token by its token value.
func (s *UserService) GetAuthTokenByToken(ctx context.Context, token string) (*AuthToken, error) {
	authToken, err := s.authTokenRepo.GetAuthTokenByToken(ctx, token)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("auth token not found")
		}
		return nil, fmt.Errorf("failed to find auth token: %w", err)
	}

	var decryptedToken string

	if !authToken.Retrieved {
		// First time retrieval, return plain-text token
		decryptedToken, err = Decrypt(authToken.Token, os.Getenv("NANNY_ENCRYPTION_KEY"))
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
		decryptedToken, err = Decrypt(authToken.Token, os.Getenv("NANNY_ENCRYPTION_KEY"))
		if err != nil {
			return nil, err
		}
		if len(decryptedToken) <= 6 {
			authToken.Token = decryptedToken
			return authToken, nil // Return the whole token if it's too short
		}

		authToken.Token = decryptedToken
		return authToken, nil
	}

	return authToken, nil
}

// GetUserByEmail retrieves a user by their email address.
func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	return user, nil
}

// GetAuthTokenByHashedToken retrieves an auth token by hashed token
func (s *UserService) GetAuthTokenByHashedToken(ctx context.Context, hashedToken string) (*AuthToken, error) {
	authToken, err := s.authTokenRepo.GetAuthTokenByHashedToken(ctx, hashedToken)

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve auth token: %v", err)
	}
	return authToken, nil
}
