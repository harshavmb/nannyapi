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
	"reflect"
	"strings"
	"time"

	"golang.org/x/oauth2"
	githubOAuth2 "golang.org/x/oauth2/github"

	"github.com/harshavmb/nannyapi/internal/token"
	"github.com/harshavmb/nannyapi/internal/user"
)

type GitHubAuth struct {
	oauthConf           *oauth2.Config
	randSrc             io.Reader
	userService         *user.UserService
	refreshTokenService *token.RefreshTokenService
	nannyEncryptionKey  string
	jwtSecret           string
	frontEndHost        string
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
func NewGitHubAuth(clientID, clientSecret, redirectURL string, userService *user.UserService, refreshTokenService *token.RefreshTokenService, nannyEncryptionKey, jwtSecret, frontEndHost string) *GitHubAuth {
	return &GitHubAuth{
		oauthConf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email"},
			Endpoint:     githubOAuth2.Endpoint,
		},
		randSrc:             rand.Reader,
		userService:         userService,
		refreshTokenService: refreshTokenService,
		nannyEncryptionKey:  nannyEncryptionKey,
		jwtSecret:           jwtSecret,
		frontEndHost:        frontEndHost,
	}
}

func (g *GitHubAuth) HandleGitHubLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state, err := g.generateStateString()
		if err != nil {
			http.Error(w, "Failed to generate state", http.StatusInternalServerError)
			return
		}

		// set secure flag to true if the request is coming from https
		secure := r.TLS != nil

		http.SetCookie(w, &http.Cookie{
			Name:     "oauthstate",
			Value:    state,
			Expires:  time.Now().Add(1 * time.Hour),
			HttpOnly: true,
			Path:     "/", // Ensure the cookie is sent with the callback request
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})
		url := g.oauthConf.AuthCodeURL(state, oauth2.AccessTypeOffline)
		http.Redirect(w, r, url, http.StatusSeeOther)
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

		// set secure flag to true if the request is coming from https
		secure := r.TLS != nil

		// Store the token in a cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "GH_Authorization",
			Value:    token.AccessToken,
			Expires:  time.Now().Add(time.Hour),
			HttpOnly: true,
			Path:     "/",
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})

		// Redirect to the profile page
		// http.Redirect(w, r, "/github/profile", http.StatusSeeOther)
		http.Redirect(w, r, fmt.Sprintf("%s/%s", g.frontEndHost, "dashboard"), http.StatusSeeOther)
	}
}

func (g *GitHubAuth) HandleGitHubProfile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		tokenCookie, err := r.Cookie("GH_Authorization")
		if err != nil {
			http.Error(w, "GitHub Authorization cookie missing", http.StatusUnauthorized)
			return
		}

		// Check for the presence of the refresh_token cookie
		refreshTokenCookie, err := r.Cookie("refresh_token")
		if err == nil {
			// Validate the existing refresh token
			_, err := token.ValidateJWTToken(refreshTokenCookie.Value, g.jwtSecret)
			if err == nil {
				// Reuse the existing refresh token if valid
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Return tokens and user info
				refreshTokenResponse := map[string]interface{}{
					"refresh_token": refreshTokenCookie.Value,
				}
				if err := json.NewEncoder(w).Encode(refreshTokenResponse); err != nil {
					log.Printf("Failed to encode refresh token response: %v", err)
					http.Error(w, "Failed to encode response", http.StatusInternalServerError)
					return
				}
				return // we don't execute further to avoid more DB calls
			} else {
				log.Printf("Existing refresh token is invalid: %v", err)
			}
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
		// TO-DO, find a better way to set user than reflection
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

		// Save user information to the database
		if err := g.userService.SaveUser(r.Context(), userInfo); err != nil {
			http.Error(w, "Failed to save user info: "+err.Error(), http.StatusInternalServerError)
		}

		// Fetch the userID
		userByEmail, err := g.userService.GetUserByEmail(context.Background(), user.Email)
		if err != nil {
			http.Error(w, "Failed to fetch user info by email: "+err.Error(), http.StatusInternalServerError)
		}
		userID := userByEmail.ID.Hex()

		// Generate Acccess and Refresh Tokens
		refreshToken, err := token.GenerateJWT(userID, 7*24*time.Hour, "refresh", g.jwtSecret)
		if err != nil {
			http.Error(w, "Failed to generate refresh token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ipAddress := strings.Split(r.RemoteAddr, ":")

		// Save the new refresh token in database
		refreshTokenData := &token.RefreshToken{
			Token:     refreshToken,
			UserID:    userID,
			UserAgent: r.UserAgent(),
			IPAddress: ipAddress[0],
		}
		_, err = g.refreshTokenService.CreateRefreshToken(context.Background(), *refreshTokenData, g.nannyEncryptionKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		accessToken, err := token.GenerateJWT(userID, 15*time.Minute, "access", g.jwtSecret)
		if err != nil {
			http.Error(w, "Failed to generate access token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Return tokens and user info
		response := map[string]interface{}{
			"user":          user,
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		}

		// set secure flag to true if the request is coming from https
		secure := r.TLS != nil

		// Store the refresh_token in a http-only cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "refresh_token",
			Value:    refreshToken,
			Expires:  time.Now().Add(7 * 24 * time.Hour),
			HttpOnly: true,
			Path:     "/",
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("Failed to encode auth response: %v", err)
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
			return
		}
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
