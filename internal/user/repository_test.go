package user

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func TestFindUser(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewUserRepository(client.Database(testDBName))

	t.Run("ValidUser", func(t *testing.T) {
		// Create a valid user
		newUser := &User{
			Email:     "test@example.com",
			Name:      "Test User",
			AvatarURL: "https://example.com/avatar.png",
		}

		// create the user
		insertedResult, err := repo.CreateUser(context.Background(), newUser)
		assert.NoError(t, err)

		// Verify the user was inserted
		createdUser, err := repo.FindUserByID(context.Background(), insertedResult.InsertedID.(bson.ObjectID))
		assert.NoError(t, err)
		assert.NotNil(t, createdUser)
		assert.Equal(t, "test@example.com", createdUser.Email)
		assert.Equal(t, "Test User", createdUser.Name)
		assert.Equal(t, "https://example.com/avatar.png", createdUser.AvatarURL)

		// Verify the user was inserted by email
		createdUserEmail, err := repo.FindUserByEmail(context.Background(), createdUser.Email)
		assert.NoError(t, err)
		assert.NotNil(t, createdUserEmail)
		assert.Equal(t, createdUser.ID, createdUserEmail.ID)
		assert.Equal(t, "Test User", createdUserEmail.Name)
		assert.Equal(t, "https://example.com/avatar.png", createdUserEmail.AvatarURL)
	})

	// Uncomment the following test cases after implementing the duplicate user check
	// t.Run("DuplicateUser", func(t *testing.T) {
	// 	// Create a user that already exists
	// 	existingUser := &User{
	// 		Email:     "duplicate@example.com",
	// 		Name:      "Duplicate User",
	// 		AvatarURL: "https://example.com/avatar.png",
	// 	}

	// 	_, err := repo.CreateUser(context.Background(), existingUser)
	// 	assert.NoError(t, err)

	// 	// Attempt to create the same user again
	// 	_, err = repo.CreateUser(context.Background(), existingUser)
	// 	assert.Error(t, err)
	// 	assert.Contains(t, err.Error(), "user already exists")
	// })

	// t.Run("InvalidUser", func(t *testing.T) {
	// 	// Create a user with missing required fields
	// 	invalidUser := &User{
	// 		Email: "", // Missing email
	// 	}

	// 	_, err := repo.CreateUser(context.Background(), invalidUser)
	// 	assert.Error(t, err)
	// 	assert.Contains(t, err.Error(), "email is required")
	// })

	// t.Run("NilUser", func(t *testing.T) {
	// 	// Pass a nil user
	// 	_, err := repo.CreateUser(context.Background(), nil)
	// 	assert.Error(t, err)
	// 	assert.Contains(t, err.Error(), "user cannot be nil")
	// })

	// t.Run("NilContext", func(t *testing.T) {
	// 	// Pass a nil context
	// 	newUser := &User{
	// 		Email:     "test2@example.com",
	// 		Name:      "Test User 2",
	// 		AvatarURL: "https://example.com/avatar2.png",
	// 	}

	// 	_, err := repo.CreateUser(nil, newUser)
	// 	assert.Error(t, err)
	// 	assert.Contains(t, err.Error(), "context is nil")
	// })
}
