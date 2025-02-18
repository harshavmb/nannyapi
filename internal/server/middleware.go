package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/harshavmb/nannyapi/internal/user"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type contextKey string

const (
	userContextKey contextKey = "user"
)

// AuthMiddleware authenticates requests using the Authorization header.
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header is required", http.StatusUnauthorized)
			return
		}

		// Extract the token from the header (assuming a "Bearer" token)
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, "Invalid Authorization header format", http.StatusBadRequest)
			return
		}

		encryptionKey := os.Getenv("NANNY_ENCRYPTION_KEY")
		if encryptionKey == "" {
			log.Println("NANNY_ENCRYPTION_KEY not set, cannot decrypt token")
			http.Error(w, "ENCRYPTION KEY NOT SET", http.StatusInternalServerError)
			return
		}

		// Validate the token against the database
		userInfo, err := s.validateAuthToken(r.Context(), token, encryptionKey)
		if err != nil {
			log.Printf("Failed to validate auth token: %v", err)
			http.Error(w, "Invalid auth token", http.StatusUnauthorized)
			return
		}

		// Add the user information to the request context
		ctx := context.WithValue(r.Context(), userContextKey, userInfo)
		r = r.WithContext(ctx)

		// Call the next handler in the chain
		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext retrieves the user information from the request context.
func GetUserFromContext(r *http.Request) (*user.User, bool) {
	userInfo, ok := r.Context().Value(userContextKey).(*user.User)
	if ok {
		return userInfo, ok
	}

	// If not found in the context, attempt to retrieve the user information from the userinfo cookie
	// Check if user info is already in the cookie
	userInfo, err := GetUserInfoFromCookie(r)
	if err != nil {
		return nil, false
	}

	return userInfo, ok
}

func (s *Server) validateAuthToken(ctx context.Context, token string, encryptionKey string) (*user.User, error) {
	// Hash the token
	hashedToken := user.HashToken(token)

	// Retrieve the auth token from the database
	authToken, err := s.userService.GetAuthTokenByHashedToken(ctx, hashedToken)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("auth token not found")
		}
		return nil, fmt.Errorf("failed to find auth token: %w", err)
	}

	// Decrypt the token
	decryptedToken, err := user.Decrypt(authToken.Token, encryptionKey)
	if err != nil {
		return nil, err
	}

	if decryptedToken != token {
		return nil, fmt.Errorf("token mismatch")
	}

	// Retrieve the user from the database
	user, err := s.userService.GetUserByEmail(ctx, authToken.Email)
	if err != nil {
		return nil, err
	}

	return user, nil
}
