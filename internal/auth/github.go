package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"log"
	"math/big"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	githubOAuth2 "golang.org/x/oauth2/github"
)

type GitHubAuth struct {
	oauthConf *oauth2.Config
	randSrc   io.Reader
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
func NewGitHubAuth(clientID, clientSecret, redirectURL string) *GitHubAuth {
	return &GitHubAuth{
		oauthConf: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email"},
			Endpoint:     githubOAuth2.Endpoint,
		},
		randSrc: rand.Reader,
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

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(userInfo)
	}
}
