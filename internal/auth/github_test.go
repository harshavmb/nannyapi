package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"golang.org/x/oauth2"
	githubOAuth2 "golang.org/x/oauth2/github"
)

// MockOAuth2Config is a mock implementation of oauth2.Config
type MockOAuth2Config struct {
	oauth2.Config
	ExchangeFunc func(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error)
}

// Override the Exchange method for mocking
func (m *MockOAuth2Config) Exchange(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if m.ExchangeFunc != nil {
		return m.ExchangeFunc(ctx, code, opts...)
	}
	return nil, fmt.Errorf("mock Exchange not implemented")
}

// Helper function to check if a string contains a substring
func contains(str, substr string) bool {
	return len(str) >= len(substr) && str[:len(substr)] == substr
}

// Test for HandleGitHubLogin
func TestHandleGitHubLogin(t *testing.T) {
	// Initialize GitHubAuth with environment variables
	githubAuth := &GitHubAuth{
		oauthConf: &oauth2.Config{
			ClientID:     os.Getenv("GH_CLIENT_ID"),
			ClientSecret: os.Getenv("GH_CLIENT_SECRET"),
			RedirectURL:  "http://localhost:8080/github/callback",
			Scopes:       []string{"user:email"},
			Endpoint:     githubOAuth2.Endpoint,
		},
	}

	// Create a test HTTP request and response recorder
	req := httptest.NewRequest(http.MethodGet, "/github/login", nil)
	rec := httptest.NewRecorder()

	// Call the handler
	handler := githubAuth.HandleGitHubLogin()
	handler.ServeHTTP(rec, req)

	// Assert the response status code
	if rec.Code != http.StatusSeeOther {
		t.Errorf("Expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}

	// Assert the `oauthstate` cookie is set
	cookies := rec.Result().Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "oauthstate" {
			found = true
			if cookie.Expires.Before(time.Now()) {
				t.Errorf("oauthstate cookie has expired")
			}
			break
		}
	}
	if !found {
		t.Errorf("oauthstate cookie not set")
	}

	// Assert the redirect URL
	location := rec.Header().Get("Location")
	if location == "" {
		t.Errorf("Redirect URL not set")
	}
	if !contains(location, "https://github.com/login/oauth/authorize") {
		t.Errorf("Redirect URL does not contain GitHub authorization endpoint")
	}
}

// TO-DO to be added later (no iead how to do that)
// Test for HandleGitHubCallback
// func TestHandleGitHubCallback(t *testing.T) {
// 	// Mock the OAuth2 Config
// 	mockConfig := &MockOAuth2Config{
// 		Config: oauth2.Config{
// 			ClientID:     os.Getenv("GH_CLIENT_ID"),
// 			ClientSecret: os.Getenv("GH_CLIENT_SECRET"),
// 			RedirectURL:  "http://localhost:8080/github/callback",
// 			Scopes:       []string{"user:email"},
// 			Endpoint:     githubOAuth2.Endpoint,
// 		},
// 		ExchangeFunc: func(ctx context.Context, code string, opts ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
// 			if code != "mock-code" {
// 				return nil, fmt.Errorf("invalid code")
// 			}
// 			return &oauth2.Token{
// 				AccessToken: "mock-access-token",
// 				Expiry:      time.Now().Add(1 * time.Hour),
// 			}, nil
// 		},
// 	}

// 	// Initialize GitHubAuth with the mock config
// 	githubAuth := &GitHubAuth{
// 		oauthConf: &mockConfig.Config, // Correctly assign the mock config
// 	}

// 	githubAuth.oauthConf = &mockConfig.Config // Correctly assign the mock config

// 	// Create a test HTTP request with a valid state and code
// 	req := httptest.NewRequest(http.MethodGet, "/github/callback?code=mock-code", nil)
// 	req.AddCookie(&http.Cookie{
// 		Name:     "oauthstate",
// 		Value:    "mock-state",
// 		Expires:  time.Now().Add(1 * time.Hour),
// 		HttpOnly: true,
// 	})

// 	// Create a response recorder
// 	rec := httptest.NewRecorder()

// 	// Call the handler
// 	handler := githubAuth.HandleGitHubCallback()
// 	handler.ServeHTTP(rec, req)

// 	// Assert the response status code
// 	if rec.Code != http.StatusSeeOther {
// 		t.Errorf("Expected status %d, got %d", http.StatusSeeOther, rec.Code)
// 	}

// 	// Assert the `Authorization` cookie is set
// 	cookies := rec.Result().Cookies()
// 	found := false
// 	for _, cookie := range cookies {
// 		if cookie.Name == "Authorization" {
// 			found = true
// 			if cookie.Expires.Before(time.Now()) {
// 				t.Errorf("Authorization cookie has expired")
// 			}
// 			break
// 		}
// 	}
// 	if !found {
// 		t.Errorf("Authorization cookie not set")
// 	}

// 	// Assert the redirect URL
// 	location := rec.Header().Get("Location")
// 	if location != "/github/profile" {
// 		t.Errorf("Expected redirect to /github/profile, got %s", location)
// 	}
// }
