package user

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db2"
	testCollectionName = "users"
)

func setupTestDB(t *testing.T) (*mongo.Client, func()) {
	mongoURI := os.Getenv("MONGODB_URI")
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		t.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Cleanup function to drop the test database after tests
	cleanup := func() {
		err := client.Database(testDBName).Drop(context.Background())
		if err != nil {
			t.Fatalf("Failed to drop test database: %v", err)
		}
		err = client.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("Failed to disconnect from MongoDB: %v", err)
		}
	}

	return client, cleanup
}

func TestUserRepository(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewUserRepository(client.Database(testDBName))

	t.Run("UpsertUser", func(t *testing.T) {
		user := &User{
			Email:        "test@example.com",
			Name:         "Test User",
			AvatarURL:    "http://example.com/avatar.png",
			HTMLURL:      "http://example.com",
			LastLoggedIn: time.Now(),
		}

		result, err := repo.UpsertUser(context.Background(), user)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.UpsertedID)

		// Verify the user was inserted
		insertedUser, err := repo.FindUserByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, insertedUser)
		assert.Equal(t, "test@example.com", insertedUser.Email)
	})

	t.Run("FindUserByEmail", func(t *testing.T) {
		// Insert a user
		user := &User{
			Email:        "findme@example.com",
			Name:         "Find Me",
			AvatarURL:    "http://example.com/avatar.png",
			HTMLURL:      "http://example.com",
			LastLoggedIn: time.Now(),
		}
		_, err := repo.UpsertUser(context.Background(), user)
		assert.NoError(t, err)

		// Find the user by email
		foundUser, err := repo.FindUserByEmail(context.Background(), "findme@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, foundUser)
		assert.Equal(t, "findme@example.com", foundUser.Email)
	})

	t.Run("UserNotFound", func(t *testing.T) {
		// Try to find a non-existent user
		user, err := repo.FindUserByEmail(context.Background(), "nonexistent@example.com")
		assert.NoError(t, err)
		assert.Nil(t, user)
	})
}

func TestAuthTokenRepository(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewAuthTokenRepository(client.Database(testDBName))

	t.Run("CreateAuthToken", func(t *testing.T) {
		authToken := &AuthToken{
			Email: "test@example.com",
			Token: "some-token",
		}

		// Hash the token
		hashedToken := HashToken(authToken.Token)

		result, err := repo.CreateAuthToken(context.Background(), authToken.Token, authToken.Email, hashedToken)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify the auth token was inserted
		insertedAuthToken, err := repo.GetAuthTokenByEmail(context.Background(), "test@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, insertedAuthToken)
		assert.Equal(t, "test@example.com", insertedAuthToken.Email)
	})

	t.Run("GetAuthTokenByEmail", func(t *testing.T) {
		// Insert an auth token
		authToken := &AuthToken{
			Email: "findme@example.com",
			Token: "some-token",
		}

		// Hash the token
		hashedToken := HashToken(authToken.Token)

		_, err := repo.CreateAuthToken(context.Background(), authToken.Token, authToken.Email, hashedToken)
		assert.NoError(t, err)

		// Find the auth token by email
		foundAuthToken, err := repo.GetAuthTokenByEmail(context.Background(), "findme@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, foundAuthToken)
		assert.Equal(t, "findme@example.com", foundAuthToken.Email)
	})

	t.Run("AuthTokenNotFound", func(t *testing.T) {
		// Try to find a non-existent auth token
		authToken, err := repo.GetAuthTokenByEmail(context.Background(), "nonexistent@example.com")
		assert.NoError(t, err)
		assert.Nil(t, authToken)
	})
}
