package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/harshavmb/nannyapi/internal/user"
	"golang.org/x/oauth2"
)

type mockUserService struct {
	user.UserService
	saveUserErr error
}

func (m *mockUserService) SaveUser(ctx context.Context, userInfo map[string]interface{}) error {
	return m.saveUserErr
}

func TestHandleGitHubProfile(t *testing.T) {
	tests := []struct {
		name            string
		authCookie      *http.Cookie
		userInfoCookie  *http.Cookie
		mockUserService *mockUserService
		expectedStatus  int
	}{
		{
			name:            "Missing authorization cookie",
			authCookie:      nil,
			userInfoCookie:  nil,
			mockUserService: &mockUserService{},
			expectedStatus:  http.StatusUnauthorized,
		},
		{
			name:            "Valid authorization cookie but missing user info cookie",
			authCookie:      &http.Cookie{Name: "Authorization", Value: "valid-token"},
			userInfoCookie:  nil,
			mockUserService: &mockUserService{},
			expectedStatus:  http.StatusUnauthorized,
		},
		{
			name:            "Valid authorization and user info cookies",
			authCookie:      &http.Cookie{Name: "Authorization", Value: "valid-token"},
			userInfoCookie:  &http.Cookie{Name: "userinfo", Value: `{"email":"test@example.com","name":"Test User"}`},
			mockUserService: &mockUserService{},
			expectedStatus:  http.StatusSeeOther,
		},
		{
			name:            "Error fetching user info from GitHub",
			authCookie:      &http.Cookie{Name: "Authorization", Value: "invalid-token"},
			userInfoCookie:  nil,
			mockUserService: &mockUserService{},
			expectedStatus:  http.StatusUnauthorized,
		},
		{
			name:            "Error saving user info to the database",
			authCookie:      &http.Cookie{Name: "Authorization", Value: "valid-token"},
			userInfoCookie:  nil,
			mockUserService: &mockUserService{saveUserErr: fmt.Errorf("database error")},
			expectedStatus:  http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &GitHubAuth{
				oauthConf:   &oauth2.Config{},
				userService: &tt.mockUserService.UserService,
			}

			req := httptest.NewRequest("GET", "/github/profile", nil)
			if tt.authCookie != nil {
				req.AddCookie(tt.authCookie)
			}
			if tt.userInfoCookie != nil {
				// Properly encode the userinfo cookie value
				userInfoJSON, _ := json.Marshal(tt.userInfoCookie)
				req.AddCookie(&http.Cookie{
					Name:  "userinfo",
					Value: string(userInfoJSON),
				})
			}

			rr := httptest.NewRecorder()
			handler := g.HandleGitHubProfile()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
		})
	}
}
