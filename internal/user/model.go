package user

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type User struct {
	ID           bson.ObjectID `json:"id" bson:"_id,omitempty"`
	Email        string        `json:"email" bson:"email"`
	Name         string        `json:"name" bson:"name"`
	AvatarURL    string        `json:"avatar_url" bson:"avatar_url"`
	HTMLURL      string        `json:"html_url" bson:"html_url"`
	LastLoggedIn time.Time     `json:"last_logged_in" bson:"last_logged_in"`
}

type GitHubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

type AuthToken struct {
	Email     string    `json:"email" bson:"email"`
	Token     string    `bson:"token" json:"token"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
	Retrieved bool      `bson:"retrieved" json:"retrieved"`
}
