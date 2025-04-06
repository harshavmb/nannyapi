package token

import (
	"github.com/golang-jwt/jwt"
)

// Define claims for JWT.
type Claims struct {
	UserID string `json:"userID"`
	jwt.StandardClaims
}
