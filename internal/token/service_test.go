package token

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	testDBName         = "test_db2"
	testCollectionName = "users"
	encryptionKey      = "T3byOVRJGt/25v6c6GC3wWkNKtL1WPuW5yVjCEnaHA8=" // Base64 encoded 32-byte key
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

func TestTokenService(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewTokenRepository(client.Database(testDBName))
	service := NewTokenService(repo)

	t.Run("CreateToken", func(t *testing.T) {
		token := Token{
			Email: "test@example.com",
		}

		result, err := service.CreateToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify the token was inserted
		insertedToken, err := service.GetTokenByHashedToken(context.Background(), result.HashedToken)
		assert.NoError(t, err)
		assert.NotNil(t, insertedToken)
		assert.Equal(t, token.Email, insertedToken.Email)
	})

	t.Run("GetTokenByEmail", func(t *testing.T) {
		// Insert a token
		token := Token{
			Email: "findme@example.com",
		}

		_, err := service.CreateToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)

		// Find tokens by email
		foundToken, err := service.GetAllTokens(context.Background(), token.Email)
		assert.NoError(t, err)
		assert.NotNil(t, foundToken)
		assert.Equal(t, token.Email, foundToken[0].Email)
	})

	t.Run("TokenNotFound", func(t *testing.T) {
		// Try to find a non-existent token
		tokens, err := service.GetAllTokens(context.Background(), "nonexistent@example.com")
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("DeleteToken", func(t *testing.T) {
		// Insert a token
		token := Token{
			Email: "emaildeleted@example.com",
		}

		result, err := service.CreateToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)

		// Delete tokens by hashedToken
		err = service.DeleteToken(context.Background(), result.HashedToken)
		assert.NoError(t, err)

		// Try to find a non-existent token
		tokens, err := service.GetAllTokens(context.Background(), token.Email)
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})
}

func TestRefreshTokenService(t *testing.T) {
	client, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRefreshTokenRepository(client.Database(testDBName))
	service := NewRefreshTokenService(repo)

	t.Run("CreateRefreshToken", func(t *testing.T) {
		token := RefreshToken{
			Email:     "test@example.com",
			UserAgent: "test/nannyapi",
			IPAddress: "1.1.1.1",
		}

		result, err := service.CreateRefreshToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		// Verify the refresh token was inserted
		insertedRefreshToken, err := service.GetRefreshTokenByHashedToken(context.Background(), result.HashedToken)
		assert.NoError(t, err)
		assert.NotNil(t, insertedRefreshToken)
		assert.Equal(t, token.Email, insertedRefreshToken.Email)
	})

	t.Run("GetRefreshTokensByEmail", func(t *testing.T) {
		// Insert a refresh token
		token := RefreshToken{
			Email:     "findme@example.com",
			UserAgent: "tests/nannyapi",
			IPAddress: "1.1.1.1",
		}

		_, err := service.CreateRefreshToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)

		// Find the refresh tokens by email
		foundRefreshTokens, err := repo.GetRefreshTokensByUser(context.Background(), token.Email)
		assert.NoError(t, err)
		assert.NotNil(t, foundRefreshTokens)
		assert.Equal(t, token.Email, foundRefreshTokens[0].Email)
	})

	t.Run("RefreshTokenNotFound", func(t *testing.T) {
		// Try to find a non-existent refresh token
		tokens, err := repo.GetRefreshTokensByUser(context.Background(), "nonexistent@example.com")
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("DeleteRefreshToken", func(t *testing.T) {
		// Insert a refresh token
		token := RefreshToken{
			Email:     "emaildeleted@example.com",
			UserAgent: "tests/nannyapi",
			IPAddress: "1.1.1.1",
		}

		result, err := service.CreateRefreshToken(context.Background(), token, encryptionKey)
		assert.NoError(t, err)

		// Delete tokens by hashedToken
		err = service.DeleteRefreshToken(context.Background(), result.HashedToken)
		assert.NoError(t, err)

		// Try to find a non-existent token
		tokens, err := repo.GetRefreshTokensByUser(context.Background(), token.Email)
		assert.NoError(t, err)
		assert.Nil(t, tokens)
	})

	t.Run("RevokeAllRefreshTokens", func(t *testing.T) {
		// Insert refresh tokens
		tokens := []RefreshToken{
			{Email: "emaildeleted@example.com", UserAgent: "tests/nannyapi", IPAddress: "1.1.1.1"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36", IPAddress: "192.168.1.100"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0 Safari/605.1.15", IPAddress: "10.0.0.1"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:89.0) Gecko/20100101 Firefox/89.0", IPAddress: "8.8.8.8"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:89.0) Gecko/20100101 Firefox/89.0", IPAddress: "172.16.0.1"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (iPad; CPU OS 13_5_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.1 Mobile/15E148 Safari/604.1", IPAddress: "192.168.0.50"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 14_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.0.3 Mobile/15E148 Safari/604.1", IPAddress: "10.0.1.20"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Linux; Android 10; SM-A205U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.210 Mobile Safari/537.36", IPAddress: "8.8.4.4"},
			{Email: "emaildeleted@example.com", UserAgent: "Mozilla/5.0 (Linux; Android 10; SM-G960U) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/90.0.4430.210 Mobile Safari/537.36", IPAddress: "172.16.1.10"},
			{Email: "user9@example.com", UserAgent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", IPAddress: "66.249.66.1"},
			{Email: "user10@example.com", UserAgent: "Mozilla/5.0 (compatible; Bingbot/2.0; +http://www.bing.com/bingbot.htm)", IPAddress: "157.55.39.1"},
		}

		for _, token := range tokens {
			_, err := service.CreateRefreshToken(context.Background(), token, encryptionKey)
			assert.NoError(t, err)
		}

		// Delete tokens by hashedToken

		err := service.RevokeAllRefreshTokens(context.Background(), "emaildeleted@example.com")
		assert.NoError(t, err)

		// Try to find a non-existent token
		deletedTokens, err := repo.GetRefreshTokensByUser(context.Background(), "emaildeleted@example.com")
		assert.NoError(t, err)
		assert.Nil(t, deletedTokens)

		// Other user tokens must exist
		otherUserTokens, err := repo.GetRefreshTokensByUser(context.Background(), "user9@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, otherUserTokens)
	})
}
