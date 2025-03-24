package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/chat"
	"github.com/harshavmb/nannyapi/internal/token"
)

const (
	Issuer = "https://nannyai.harshanu.space"
)

// chatRequest represents the request payload for the chat handler
type chatRequest struct {
	Chat    string    `json:"chat"`
	History []content `json:"history"`
}

// content represents the content of a chat message
type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

// part represents a part of a chat message
type part struct {
	Text string `json:"text"`
}

// parseRequestJSON populates the target with the fields of the JSON-encoded value in the request
// body. It expects the request to have the Content-Type header set to JSON and a body with a
// JSON-encoded value complying with the underlying type of target.
func parseRequestJSON(r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return err
	}
	if mediaType != "application/json" {
		return fmt.Errorf("expecting application/json Content-Type. Got %s", mediaType)
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	return dec.Decode(target)
}

// transform converts []content to []*genai.Content that is accepted by the model's chat session.
func transform(cs []content) []*genai.Content {
	gcs := make([]*genai.Content, len(cs))
	for i, c := range cs {
		gcs[i] = c.transform()
	}
	return gcs
}

// transform converts content to genai.Content that is accepted by the model's chat session.
func (c *content) transform() *genai.Content {
	gc := &genai.Content{}
	gc.Role = c.Role
	ps := make([]genai.Part, len(c.Parts))
	for i, p := range c.Parts {
		ps[i] = genai.Text(p.Text)
	}
	gc.Parts = ps
	return gc
}

func extractCommands(res *genai.GenerateContentResponse) ([]string, error) {
	var recipes []string
	for _, part := range res.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			if err := json.Unmarshal([]byte(txt), &recipes); err != nil {
				return nil, err // Return error if unmarshalling fails
			}
		}
	}
	return recipes, nil
}

func sendCommandsToAgent(w http.ResponseWriter, commands []string) {
	// Send commands to agent (e.g., write JSON to the response)
	json.NewEncoder(w).Encode(map[string][]string{"commands": commands})
}

func generateHistory(prompts, responses, types []string) []chat.PromptResponse {
	history := make([]chat.PromptResponse, len(prompts))
	for i := range prompts {
		history[i] = chat.PromptResponse{
			Prompt:   prompts[i],
			Response: responses[i],
			Type:     types[i],
		}
	}
	return history
}

func getAgentResponse(r *http.Request) string {
	// Read JSON from the request body (agent's output)
	var agentOutput struct {
		Output string `json:"output"`
	}

	if err := json.NewDecoder(r.Body).Decode(&agentOutput); err != nil {
		//Handle error appropriately
		log.Printf("Error decoding agent's response: %v", err)
		return "" // Or an error message
	}
	return agentOutput.Output
}

func extractGeminiFeedback(res *genai.GenerateContentResponse) string {
	// Extract the Gemini's feedback from the response
	var geminiFeedback string
	for _, candidate := range res.Candidates {
		for _, part := range candidate.Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				geminiFeedback += string(txt)
			}
		}
	}
	return geminiFeedback
}

func sendGeminiFeedbackToClient(w http.ResponseWriter, feedback string) {
	// Send Gemini's feedback back to the client (as JSON)
	json.NewEncoder(w).Encode(map[string]string{"feedback": feedback})
}

// IsValidEmail checks if a string is a valid email address
func IsValidEmail(email string) bool {
	// Updated regular expression to handle IP addresses in square brackets
	emailRegex := `^(?i)[a-zA-Z0-9._%+-]+@([a-zA-Z0-9.-]+\.[a-zA-Z]{2,}|(\[[0-9]{1,3}(\.[0-9]{1,3}){3}\]))$`
	re := regexp.MustCompile(emailRegex)

	// Check if the email matches the regex
	if !re.MatchString(email) {
		return false
	}

	// Additional checks for edge cases
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	// Validate the domain part
	domain := parts[1]
	if strings.HasPrefix(domain, "-") || strings.HasSuffix(domain, "-") {
		return false
	}
	if strings.Contains(domain, "..") {
		return false
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}

	return true
}

func generateRefreshToken(userID, jwtSecret string) (string, error) {
	duration := 7 * 24 * time.Hour
	refreshToken, err := token.GenerateJWT(userID, duration, "refresh", jwtSecret)
	if err != nil {
		return "", err
	}
	return refreshToken, nil
}

func generateAccessToken(userID, jwtSecret string) (string, error) {
	duration := 1 * 15 * time.Minute // 15 minutes
	accessToken, err := token.GenerateJWT(userID, duration, "access", jwtSecret)
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

func (s *Server) deleteRefreshToken(ctx context.Context, tokenString string) error {
	hashedToken := token.HashToken(tokenString)
	return s.refreshTokenservice.DeleteRefreshToken(ctx, hashedToken)
}

func (s *Server) validateRefreshToken(ctx context.Context, tokenString, jwtSecret string) (bool, *token.Claims, error) {
	hashedToken := token.HashToken(tokenString)

	// Validate the refresh token
	claims, err := token.ValidateJWTToken(tokenString, jwtSecret)
	if err != nil {
		return false, nil, err
	}
	// Check if the token exists in the database and is not revoked
	refreshToken, err := s.refreshTokenservice.GetRefreshTokenByHashedToken(ctx, hashedToken)
	if err != nil {
		return false, claims, err
	}
	if refreshToken != nil {
		// Check if the token is revoked
		if refreshToken.Revoked {
			return false, claims, fmt.Errorf("refresh token revoked")
		}

		// Check if the token has expired
		if time.Now().After(refreshToken.ExpiresAt) {
			return false, claims, fmt.Errorf("refresh token expired")
		}

		return true, claims, nil
	}
	return false, nil, nil
}
