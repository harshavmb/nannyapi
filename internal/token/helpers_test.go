package token

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/stretchr/testify/assert"
)

func TestEncryptDecrypt(t *testing.T) {
	encryptionKey := "T3byOVRJGt/25v6c6GC3wWkNKtL1WPuW5yVjCEnaHA8=" // Base64 encoded 32-byte key

	t.Run("EncryptDecrypt", func(t *testing.T) {
		originalText := "Hello, World!"
		encryptedText, err := Encrypt(originalText, encryptionKey)
		assert.NoError(t, err)
		assert.NotEmpty(t, encryptedText)

		decryptedText, err := Decrypt(encryptedText, encryptionKey)
		assert.NoError(t, err)
		assert.Equal(t, originalText, decryptedText)
	})

	t.Run("InvalidKeySize", func(t *testing.T) {
		invalidKey := "aGVsbG93b3JsZGhlbGxvd29ybGRoZWxsb3dvcmxk" // Base64 encoded 24-byte key
		_, err := Encrypt("Hello, World!", invalidKey)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key size")
	})
}

func TestHashToken(t *testing.T) {
	testCases := []struct {
		name     string
		token    string
		expected string
	}{
		{
			name:     "empty string",
			token:    "",
			expected: "47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=",
		},
		{
			name:     "simple string",
			token:    "test token",
			expected: "eydOAbdhAmjdywvVscPyuf+Fef/LQL3dFe769BnsD5Y=",
		},
		{
			name:     "long string",
			token:    "This is a very long string that we want to hash to ensure that the function works correctly with long inputs.",
			expected: "9H+l2j3HbSSdCJeli12aDuBaEIL1LJIpoZ7v38knyEY=",
		},
		{
			name:     "special characters",
			token:    "!@#$%^&*()_+=-`~[]\\{}|;':\",./<>?",
			expected: "w78OFByJffDkYQnmcc3FHUdAWhL5rCUwOl8TMv8SPa0=",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := HashToken(tc.token)
			if actual != tc.expected {
				t.Errorf("HashToken(%q) = %q, expected %q", tc.token, actual, tc.expected)
			}
		})
	}
}

func TestGenerateJWT(t *testing.T) {
	jwtSecret := "test-secret"
	userID := "test-user"
	duration := 1 * time.Hour

	t.Run("AccessToken", func(t *testing.T) {
		token, err := GenerateJWT(userID, duration, "access", jwtSecret)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		claims := &Claims{}
		parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, "access", claims.Subject)
		assert.Equal(t, Issuer, claims.Issuer)
		assert.True(t, claims.ExpiresAt > time.Now().Unix())
	})

	t.Run("RefreshToken", func(t *testing.T) {
		token, err := GenerateJWT(userID, duration, "refresh", jwtSecret)
		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		claims := &Claims{}
		parsedToken, err := jwt.ParseWithClaims(token, claims, func(token *jwt.Token) (interface{}, error) {
			return []byte(jwtSecret), nil
		})

		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, "refresh", claims.Subject)
		assert.Equal(t, Issuer, claims.Issuer)
		assert.True(t, claims.ExpiresAt > time.Now().Unix())
	})

	t.Run("InvalidSecret", func(t *testing.T) {
		_, err := GenerateJWT(userID, duration, "access", "")
		assert.Error(t, err)
	})
}

func TestValidateJWTToken(t *testing.T) {
	jwtSecret := "test-secret"
	userID := "test-user"
	duration := 1 * time.Hour

	t.Run("ValidToken", func(t *testing.T) {
		tokenString, err := GenerateJWT(userID, duration, "access", jwtSecret)
		assert.NoError(t, err)

		claims, err := ValidateJWTToken(tokenString, jwtSecret)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, "access", claims.Subject)
		assert.Equal(t, Issuer, claims.Issuer)
		// Use VerifyExpiresAt to check expiration
		assert.True(t, claims.VerifyExpiresAt(time.Now().Unix(), true), "token should not be expired yet")

		// Simulate token expiration by advancing time
		expiredTime := time.Now().Add(duration + time.Second).Unix() // 1 hour and 1 second later
		assert.False(t, claims.VerifyExpiresAt(expiredTime, true), "token should be expired")
	})

	t.Run("ValidRefreshToken", func(t *testing.T) {
		expirationDuration := 7 * 24 * time.Hour
		tokenString, err := GenerateJWT(userID, expirationDuration, "refresh", jwtSecret)
		assert.NoError(t, err)

		claims, err := ValidateJWTToken(tokenString, jwtSecret)
		assert.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, userID, claims.UserID)
		assert.Equal(t, "refresh", claims.Subject)
		assert.Equal(t, Issuer, claims.Issuer)
		// Use VerifyExpiresAt to check expiration
		assert.True(t, claims.VerifyExpiresAt(time.Now().Unix(), true), "token should not be expired yet")

		// Simulate token expiration by advancing time
		expiredTime := time.Now().Add(expirationDuration + time.Second).Unix() // 7 days and 1 second later
		assert.False(t, claims.VerifyExpiresAt(expiredTime, true), "token should be expired")
	})

	t.Run("InvalidToken_Expired", func(t *testing.T) {
		// Generate a token that has already expired
		expiredDuration := -1 * time.Hour
		tokenString, err := GenerateJWT(userID, expiredDuration, "access", jwtSecret)
		assert.NoError(t, err)

		_, err = ValidateJWTToken(tokenString, jwtSecret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("InvalidToken_WrongSecret", func(t *testing.T) {
		tokenString, err := GenerateJWT(userID, duration, "access", jwtSecret)
		assert.NoError(t, err)

		_, err = ValidateJWTToken(tokenString, "wrong-secret")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("InvalidToken_Malformed", func(t *testing.T) {
		_, err := ValidateJWTToken("malformed-token", jwtSecret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token")
	})

	t.Run("InvalidToken_EmptyToken", func(t *testing.T) {
		_, err := ValidateJWTToken("", jwtSecret)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "token string is empty")
	})
}
