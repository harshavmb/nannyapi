package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/harshavmb/nannyapi/internal/token"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type contextKey string

const (
	userContextKey contextKey = "userID"
)

// AuthMiddleware authenticates requests using the Authorization header.
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for the Authorization header
		authHeader := r.Header.Get("Authorization")
		apiKeyHeader := r.Header.Get("X-NANNYAPI-Key")
		if authHeader == "" && apiKeyHeader == "" {
			http.Error(w, "One of Authorization/X-NANNYAPI-Key headers is required", http.StatusUnauthorized)
			return
		}

		if authHeader != "" && apiKeyHeader != "" {
			http.Error(w, "Only one of Authorization/X-NANNYAPI-Key headers is required", http.StatusUnauthorized)
			return
		}

		var userID string

		// Extract the key from the header (assuming an API key)
		if apiKeyHeader != "" {
			// Validate the static token against the database
			userToken, err := s.validateStaticToken(r.Context(), apiKeyHeader)
			if err != nil {
				log.Printf("API key validation failed: %v", err)
				http.Error(w, "Invalid API key passed", http.StatusUnauthorized)
				return
			}
			userID = userToken.UserID
		}

		// Check whether valid accessToken is passed
		if authHeader != "" {
			// Check if the accessToken has Bearer prefix & strip that before validation
			var tokenString string
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				http.Error(w, "Invalid Authorization header format, it should start with Bearer ", http.StatusUnauthorized)
				return
			}

			// Validate the accessToken
			userToken, err := token.ValidateJWTToken(tokenString, s.jwtSecret)
			if err != nil {
				log.Printf("Invalid access token: %v", err)
				http.Error(w, "Invalid access token", http.StatusUnauthorized)
				return
			}
			userID = userToken.UserID
		}

		if userID != "" {
			// Add the user information to the request context
			ctx := context.WithValue(r.Context(), userContextKey, userID)
			r = r.WithContext(ctx)
		}

		// Call the next handler in the chain
		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext retrieves the token information from the request context.
func GetUserFromContext(r *http.Request) (string, bool) {
	userID, ok := r.Context().Value(userContextKey).(string)
	if !ok {
		return "", false
	}
	return userID, ok
}

func (s *Server) validateStaticToken(ctx context.Context, tokenString string) (*token.Token, error) {
	// Hash the token
	hashedToken := token.HashToken(tokenString)

	// Retrieve the auth token from the database
	token, err := s.tokenService.GetTokenByHashedToken(ctx, hashedToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("auth token not found")
		}
		return nil, fmt.Errorf("failed to find auth token: %w", err)
	}

	return token, nil
}
