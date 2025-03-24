package token

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Token struct for static tokens (already defined)
type Token struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID      string        `json:"user_id" bson:"user_id"`
	Token       string        `bson:"token" json:"token"`
	HashedToken string        `bson:"hashed_token" json:"hashed_token"`
	CreatedAt   time.Time     `bson:"created_at" json:"created_at"`
	Retrieved   bool          `bson:"retrieved" json:"retrieved"`
}

// RefreshToken struct for refresh tokens (store in database)
type RefreshToken struct {
	ID          bson.ObjectID `json:"id" bson:"_id,omitempty"`
	UserID      string        `json:"user_id" bson:"user_id"` // Link to user
	Token       string        `bson:"token" json:"token"`     // The refresh token itself (encrypted or hashed)
	HashedToken string        `bson:"hashed_token" json:"hashed_token"`
	ExpiresAt   time.Time     `bson:"expires_at" json:"expires_at"`
	CreatedAt   time.Time     `bson:"created_at" json:"created_at"`
	Revoked     bool          `bson:"revoked" json:"revoked"`
	UserAgent   string        `bson:"user_agent,omitempty" json:"user_agent,omitempty"` //Optional user agent
	IPAddress   string        `bson:"ip_address,omitempty" json:"ip_address,omitempty"` //Optional IP
}

// AccessToken struct (not stored in database)
// This struct is not stored in a database, but it is useful for representing
// the data that is inside the access token when it is parsed.
type AccessTokenClaims struct {
	UserID    bson.ObjectID `json:"user_id"`
	IsAgent   bool          `json:"is_agent,omitempty"`
	ExpiresAt time.Time     `json:"exp"`
	IssuedAt  time.Time     `json:"iat"`
	NotBefore time.Time     `json:"nbf"`
	Issuer    string        `json:"iss"`
}
