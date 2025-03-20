package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidEmail(t *testing.T) {
	t.Run("ValidEmails", func(t *testing.T) {
		validEmails := []string{
			"test@example.com",
			"user.name+tag+sorting@example.com",
			"another.email@subdomain.example.com",
			"email@[123.123.123.123]",
			"1234567890@example.com",
			"email@example-one.com",
			"firstname-lastname@example.com",
		}

		for _, email := range validEmails {
			assert.True(t, IsValidEmail(email), "Expected valid email: %s", email)
		}
	})

	t.Run("InvalidEmails", func(t *testing.T) {
		invalidEmails := []string{
			"plainaddress",               // Missing @ symbol
			"@missingusername.com",       // Missing username
			"username@.com",              // Missing domain name
			"username@com",               // Missing top-level domain
			"username@.com.",             // Trailing dot
			"username@-example.com",      // Invalid domain
			"username@example..com",      // Double dot in domain
			"username@example.com (Joe)", // Invalid characters
			"username@.example.com",      // Leading dot in domain
			"username@.123",              // Invalid numeric domain
		}

		for _, email := range invalidEmails {
			assert.False(t, IsValidEmail(email), "Expected invalid email: %s", email)
		}
	})
}
