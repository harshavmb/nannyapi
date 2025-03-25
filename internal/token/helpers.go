package token

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"math"
	"time"

	"github.com/golang-jwt/jwt"
)

const (
	// Alphanumeric characters for token generation
	alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	Issuer       = "https://nannyai.harshanu.space"
)

// GenerateRandomString generates a random string of the specified length using alphanumeric characters.
// from https://stackoverflow.com/questions/22892120/how-to-generate-a-random-string-of-a-fixed-length-in-go
func GenerateRandomString(length int) string {
	buff := make([]byte, int(math.Ceil(float64(length)/float64(1.33333333333))))
	rand.Read(buff)
	str := base64.RawURLEncoding.EncodeToString(buff)
	return str[:length] // strip 1 extra character we get from odd length results
}

// hashToken hashes the token using SHA-256.
func HashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.StdEncoding.EncodeToString(hash[:])
}

func Encrypt(stringToEncrypt, encryptionKey string) (string, error) {

	// Base64 decode the encryption key
	key, err := base64.StdEncoding.DecodeString(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode encryption key: %v", err)
	}

	// Check if the key size is valid for AES-256
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key size %d, key size must be 32 bytes for AES-256", len(key))
	}

	plaintext := []byte(stringToEncrypt)

	//Create a new Cipher Block from the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	//Create a new GCM - Galois Counter Mode - authentication mode
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	//Create a nonce. Nonce should be from GCM
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	//Encrypt the data using aesGCM.Seal
	ciphertext := aesGCM.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt method
func Decrypt(encryptedString string, encryptionKey string) (string, error) {
	// Base64 decode the encryption key
	key, err := base64.StdEncoding.DecodeString(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode encryption key: %v", err)
	}

	// Check if the key size is valid for AES-256
	if len(key) != 32 {
		return "", fmt.Errorf("invalid key size %d, key size must be 32 bytes for AES-256", len(key))
	}

	// Base64 decode the encrypted string
	enc, err := base64.StdEncoding.DecodeString(encryptedString)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode encrypted string: %v", err)
	}

	//Create a new Cipher Block from the key
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	//Create a new GCM - Galois Counter Mode - authentication mode
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	//Get the nonce size
	nonceSize := aesGCM.NonceSize()

	//Extract the nonce from the encrypted data
	nonce, ciphertext := enc[:nonceSize], enc[nonceSize:]

	//Decrypt the data
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// generateJWT generates both JWT Access & refresh tokens with the given claims
func GenerateJWT(UserID string, duration time.Duration, tokenType, jwtSecret string) (string, error) {
	claims := Claims{
		UserID: UserID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Unix() + int64(duration.Seconds()),
			IssuedAt:  time.Now().Unix(),
			Issuer:    Issuer,
			Subject:   tokenType, // "access" or "refresh"
		},
	}

	if tokenType == "" || jwtSecret == "" {
		return "", fmt.Errorf("tokenType and jwtSecrets shouldn't be empty")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		return "", fmt.Errorf("error generating jwt token of type %s for user %s: %v", tokenType, UserID, err)
	}
	return tokenString, nil
}

// validates the JWT token
func ValidateJWTToken(tokenString, jwtSecret string) (*Claims, error) {
	claims := &Claims{}

	if tokenString == "" {
		return nil, fmt.Errorf("token string is empty")
	}

	jwtToken, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Validate signing method
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Method.Alg())
		}

		return []byte(jwtSecret), nil
	})

	if err != nil {
		log.Printf("jwt token validation failed: %v", err)
		return nil, fmt.Errorf("invalid token")
	}

	if !jwtToken.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	// Type-assert the token.Claims to *Claims
	if parsedClaims, ok := jwtToken.Claims.(*Claims); ok {
		return parsedClaims, nil
	}

	// If type assertion fails, return an error
	return nil, fmt.Errorf("failed to parse claims")
}
