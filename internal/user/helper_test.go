package user

import (
	"testing"

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
