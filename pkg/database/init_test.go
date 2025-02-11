package database

import (
	"testing"
)

func TestInitDB(t *testing.T) {

	// Call the InitDB function
	client, err := InitDB()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check if the client is not nil
	if client == nil {
		t.Fatalf("Expected client to be non-nil")
	}
}
