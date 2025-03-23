package token

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

type TokenService struct {
	tokenRepo *TokenRepository
}

type RefreshTokenService struct {
	refreshTokenRepo *RefreshTokenRepository
}

func NewTokenService(tokenRepo *TokenRepository) *TokenService {
	return &TokenService{
		tokenRepo: tokenRepo,
	}
}

func NewRefreshTokenService(refreshTokenRepo *RefreshTokenRepository) *RefreshTokenService {
	return &RefreshTokenService{
		refreshTokenRepo: refreshTokenRepo,
	}
}

// CreateToken creates a static token
func (s *TokenService) CreateToken(ctx context.Context, token Token, encryptionKey string) (*Token, error) {
	tokenString, err := generateRandomToken(32)
	if err != nil {
		return nil, err
	}

	// Hash the token
	hashedToken := HashToken(tokenString)

	encryptedToken, err := Encrypt(tokenString, encryptionKey)
	if err != nil {
		return nil, err
	}

	// set Token object
	token.Token = encryptedToken
	token.HashedToken = hashedToken
	token.CreatedAt = time.Now()
	token.Retrieved = false

	log.Printf("Static token created by user %s", token.Email)

	return s.tokenRepo.CreateToken(ctx, token)
}

// GetTokenByHashedToken retrieves a static token by hashed token
func (s *TokenService) GetTokenByHashedToken(ctx context.Context, hashedToken string) (*Token, error) {
	token, err := s.tokenRepo.GetTokenByHashedToken(ctx, hashedToken)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No static token found
		}
		return nil, fmt.Errorf("failed to retrieve auth token: %v", err)
	}

	if token == nil {
		return nil, nil // No static token found
	}

	log.Printf("Static token fetched by user %s", token.Email)

	return token, nil
}

// DeleteToken deletes a static token
func (s *TokenService) DeleteToken(ctx context.Context, hashedToken string) error {
	err := s.tokenRepo.DeleteToken(ctx, hashedToken)

	if err != nil {
		return fmt.Errorf("failed to delete static token with hash %s: %v", hashedToken, err)
	}

	log.Printf("Static token deleted by user %s", hashedToken)
	return nil
}

// GetAllTokens gets all static tokens by a user
func (s *TokenService) GetAllTokens(context context.Context, email string) ([]*Token, error) {
	tokens, err := s.tokenRepo.GetTokensByEmail(context, email)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No static token found
		}
		return nil, err
	}

	if len(tokens) == 0 {
		return nil, nil
	}

	if len(tokens) > 0 {
		log.Printf("Static tokens fetched by user %s", email)
		return tokens, nil
	}
	return nil, nil
}

// CreateRefreshToken creates a refresh token
func (s *RefreshTokenService) CreateRefreshToken(ctx context.Context, token RefreshToken, encryptionKey string) (*RefreshToken, error) {
	tokenString, err := generateRandomToken(32)
	if err != nil {
		return nil, err
	}

	// Hash the token
	hashedToken := HashToken(tokenString)

	encryptedToken, err := Encrypt(tokenString, encryptionKey)
	if err != nil {
		return nil, err
	}

	// set Token object
	token.Token = encryptedToken
	token.HashedToken = hashedToken
	token.CreatedAt = time.Now()
	token.ExpiresAt = time.Now().AddDate(0, 0, 7) // 7 days expiry

	log.Printf("Created refresh token for user %s using user agent %s from %s", token.Email, token.UserAgent, token.IPAddress)

	return s.refreshTokenRepo.CreateRefreshToken(ctx, token)
}

// GetRefreshTokenByHashedToken retrieves a refresh token by hashed token
func (s *RefreshTokenService) GetRefreshTokenByHashedToken(ctx context.Context, hashedToken string) (*RefreshToken, error) {
	token, err := s.refreshTokenRepo.GetRefreshToken(ctx, hashedToken)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, mongo.ErrNoDocuments // No static token found
		}
		return nil, fmt.Errorf("failed to retrieve refresh token: %v", err)
	}

	if token == nil {
		return nil, nil // No static token found
	}

	log.Printf("Refresh token fetched for user %s", token.Email)

	return token, nil
}

// DeleteRefreshToken deletes a static token
func (s *RefreshTokenService) DeleteRefreshToken(ctx context.Context, hashedToken string) error {
	err := s.refreshTokenRepo.DeleteRefreshToken(ctx, hashedToken)

	if err != nil {
		return fmt.Errorf("failed to delete static token with hash %s: %v", hashedToken, err)
	}

	log.Printf("Refresh token deleted for user %s", hashedToken)
	return nil
}

// RevokeAllRefreshTokens gets all static tokens by a user
func (s *RefreshTokenService) RevokeAllRefreshTokens(context context.Context, email string) error {
	tokens, err := s.refreshTokenRepo.GetRefreshTokensByUser(context, email)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil // No refresh token found, nothing to do
		}
		return err
	}

	if len(tokens) == 0 {
		return nil
	}

	// now we delete all of them to revoke all refresh tokens
	// across all devices
	if len(tokens) > 0 {
		for _, token := range tokens {
			err := s.refreshTokenRepo.DeleteRefreshToken(context, token.HashedToken)
			if err != nil {
				return fmt.Errorf("failed to revoke refresh tokens for %s: %v", token.Email, err)
			}
		}
		log.Printf("Refresh tokens deleted for user %s", email)
		return nil
	}
	return nil
}
