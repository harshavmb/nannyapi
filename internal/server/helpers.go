package server

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt"

	"github.com/harshavmb/nannyapi/internal/token"
)

const (
	Issuer = "https://nannyai.dev"
)

// parseRequestJSON populates the target with the fields of the JSON-encoded value in the request
// body. It expects the request to have the Content-Type header set to JSON and a body with a
// JSON-encoded value complying with the underlying type of target.
func parseRequestJSON(r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid Content-Type header: %v", err)
	}
	if mediaType != "application/json" {
		return fmt.Errorf("Content-Type header is not application/json: %v", mediaType)
	}

	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		return fmt.Errorf("failed to decode request body: %v", err)
	}

	return nil
}

// IsValidEmail checks if a string is a valid email address.
func IsValidEmail(email string) bool {
	// Updated regular expression to handle IP addresses in square brackets
	emailRegex := `^(?i)[a-zA-Z0-9._%+-]+@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}|(\[[0-9]{1,3}(\.[0-9]{1,3}){3}\]))$`
	re := regexp.MustCompile(emailRegex)

	// Check if the email matches the regex
	if !re.MatchString(email) {
		return false
	}

	// Additional checks for edge cases
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	// Validate the domain part
	domain := parts[1]
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return false
	}
	if strings.Contains(domain, "..") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	return true
}

func generateRefreshToken(userID, jwtSecret string) (string, error) {
	expirationTime := time.Now().Add(30 * 24 * time.Hour) // 30 days

	claims := &token.Claims{
		UserID: userID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    Issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

func generateAccessToken(userID, jwtSecret string) (string, error) {
	expirationTime := time.Now().Add(15 * time.Minute) // 15 minutes

	claims := &token.Claims{
		UserID: userID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
			IssuedAt:  time.Now().Unix(),
			Issuer:    Issuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

func (s *Server) validateRefreshToken(ctx context.Context, tokenString, jwtSecret string) (bool, *token.Claims, error) {
	hashedToken := token.HashToken(tokenString)

	// Validate the refresh token
	claims, err := token.ValidateJWTToken(tokenString, jwtSecret)
	if err != nil {
		return false, nil, err
	}
	// Check if the token exists in the database and is not revoked
	refreshToken, err := s.refreshTokenservice.GetRefreshTokenByHashedToken(ctx, hashedToken)
	if err != nil {
		return false, claims, err
	}
	if refreshToken != nil {
		// Check if the token is revoked
		if refreshToken.Revoked {
			return false, claims, fmt.Errorf("refresh token revoked")
		}

		// Check if the token has expired
		if time.Now().After(refreshToken.ExpiresAt) {
			return false, claims, fmt.Errorf("refresh token expired")
		}

		return true, claims, nil
	}
	return false, nil, nil
}
