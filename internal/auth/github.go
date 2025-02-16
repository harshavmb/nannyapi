package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"reflect"
	"time"

	"github.com/harshavmb/nannyapi/internal/user"
	"golang.org/x/oauth2"
	githubOAuth2 "golang.org/x/oauth2/github"
)

type GitHubAuth struct {
	oauthConf   *oauth2.Config
	randSrc     io.Reader
	userService *user.UserService
}

func (g *GitHubAuth) generateStateString() (string, error) {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 16)

	for i := range b {
		randomIndex, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		b[i] = letters[randomIndex.Int64()]
	}

	return string(b), nil
}

// creating a new OAuth App at https://github.com/settings/applications/new
// The "Authorization callback URL" you set there must match the redirect URL
// you use in your code.  For local testing, something like
// "http://localhost:8080/github/callback" is typical.
func NewGitHubAuth(clientID, clientSecret, redirectURL string, userService *user.UserService) *GitHubAuth {
	return &GitHubAuth{
		oauthConf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email"},
			Endpoint:     githubOAuth2.Endpoint,
		},
		randSrc:     rand.Reader,
		userService: userService,
	}
}

func (g *GitHubAuth) HandleGitHubLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := g.generateStateString()
		if err != nil {
			http.Error(w, "Failed to generate state", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "oauthstate",
			Value:    state,
			Expires:  time.Now().Add(1 * time.Hour),
			HttpOnly: true,
			Path:     "/", // Ensure the cookie is sent with the callback request
			SameSite: http.SameSiteLaxMode,
		})
		url := g.oauthConf.AuthCodeURL(state, oauth2.AccessTypeOffline)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	}
}

func (g *GitHubAuth) HandleGitHubCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := r.Cookie("oauthstate")
		if err != nil {
			log.Printf("State cookie not found: %v", err)
			http.Error(w, "State cookie not found", http.StatusBadRequest)
			return
		}

		code := r.FormValue("code")
		token, err := g.oauthConf.Exchange(context.Background(), code)
		if err != nil {
			http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Store the token in a cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "Authorization",
			Value:    token.AccessToken,
			Expires:  time.Now().Add(time.Hour),
			HttpOnly: true,
			Path:     "/",
			SameSite: http.SameSiteLaxMode,
		})

		// Redirect to the profile page
		http.Redirect(w, r, "/github/profile", http.StatusSeeOther)
	}
}

func (g *GitHubAuth) HandleGitHubProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenCookie, err := r.Cookie("Authorization")
		if err != nil {
			http.Error(w, "Authorization cookie missing", http.StatusUnauthorized)
			return
		}

		// Check if user info is already in the cookie
		userCookie, err := r.Cookie("userinfo")
		if err == nil && userCookie.Value != "" {
			// User info found in cookie, redirect to index
			redirectURL := "/"
			http.Redirect(w, r, redirectURL, http.StatusSeeOther)
			return
		}

		client := g.oauthConf.Client(context.Background(), &oauth2.Token{AccessToken: tokenCookie.Value})
		resp, err := client.Get("https://api.github.com/user")
		if err != nil {
			http.Error(w, "Failed to get user info: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			http.Error(w, "Failed to get user info: "+resp.Status, resp.StatusCode)
			return
		}

		var userInfo map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
			http.Error(w, "Failed to decode user info: "+err.Error(), http.StatusInternalServerError)
			return
		}

		user := user.User{}

		// Use reflection to dynamically map fields
		userValue := reflect.ValueOf(&user).Elem()
		userType := userValue.Type()

		for i := 0; i < userType.NumField(); i++ {
			field := userType.Field(i)
			fieldName := field.Name
			jsonTag := field.Tag.Get("json")

			if jsonTag == "" {
				continue // Skip fields without a json tag
			}

			if value, ok := userInfo[jsonTag]; ok {
				fieldValue := userValue.Field(i)

				if fieldValue.IsValid() && fieldValue.CanSet() && fieldName != "ID" {
					switch fieldValue.Kind() {
					case reflect.String:
						if strValue, ok := value.(string); ok {
							fieldValue.SetString(strValue)
						} else {
							log.Printf("Expected string for field %s, got %T", fieldName, value)
						}
					// Add other type conversions as needed (int, bool, etc.)
					default:
						log.Printf("Unsupported type for field %s", fieldName)
					}
				}
			}
		}

		// Fetch email from GitHub API if not already set
		if user.Email == "" {
			email, err := fetchEmailFromGitHubAPI(w, client)
			if err != nil {
				log.Printf("Failed to fetch email from GitHub API: %v", err)
			}
			if email != "" {
				user.Email = email
				userInfo["email"] = email
			} else {
				log.Printf("No email found for user: %d", user.ID)
			}
		}

		// Store user info in a cookie
		userJSON, err := json.Marshal(user)
		if err != nil {
			log.Printf("Failed to marshal user info: %v", err)
			http.Error(w, "Failed to store user info", http.StatusInternalServerError)
			return
		}

		// URL-encode the JSON string
		encodedUserJSON := url.QueryEscape(string(userJSON))

		userCookie = &http.Cookie{
			Name:     "userinfo",
			Value:    encodedUserJSON,
			Path:     "/",
			Secure:   true,                    // Only send over HTTPS
			SameSite: http.SameSiteStrictMode, // Mitigate CSRF attacks
			Expires:  time.Now().Add(24 * time.Hour),
		}
		http.SetCookie(w, userCookie)

		// Save user information to the database
		if err := g.userService.SaveUser(r.Context(), userInfo); err != nil {
			http.Error(w, "Failed to save user info: "+err.Error(), http.StatusInternalServerError)
		}

		// Redirect to index
		redirectURL := "/"
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	}
}

// fetchEmailFromGitHubAPI fetches the email from the GitHub API
// and sets it in the user struct
// This is the case when the email is marked private in the GitHub settings
// More details :: https://stackoverflow.com/questions/35373995/github-user-email-is-null-despite-useremail-scope
func fetchEmailFromGitHubAPI(w http.ResponseWriter, client *http.Client) (string, error) {
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		http.Error(w, "Failed to get user info: "+err.Error(), http.StatusInternalServerError)
		return "", fmt.Errorf("failed to get user email info: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "Failed to get user email info: "+resp.Status, resp.StatusCode)
		return "", fmt.Errorf("failed to get user email info: %s", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read user email response body: "+err.Error(), http.StatusInternalServerError)
		return "", fmt.Errorf("failed to read user email response body: %s", err)
	}

	var emails []user.GitHubEmail
	if err := json.Unmarshal(body, &emails); err != nil {
		http.Error(w, "Failed to unmarshal email info: "+err.Error(), http.StatusInternalServerError)
		return "", fmt.Errorf("failed to unmarshal email info: %s", err)
	}

	for _, email := range emails {
		if email.Primary {
			return email.Email, nil
		}
	}

	return "", fmt.Errorf("no primary email found")
}
