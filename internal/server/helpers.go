package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"regexp"

	"github.com/google/generative-ai-go/genai"
	"github.com/harshavmb/nannyapi/internal/user"
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

func (s *Server) getMaskedAuthToken(r *http.Request, userEmail, encryptionKey string) (*user.AuthToken, error) {

	authToken, err := s.userService.GetAuthToken(r.Context(), userEmail, encryptionKey)
	if err != nil {
		if err == context.Canceled {
			log.Printf("Failed to get auth token: context canceled")
			return nil, nil // Return nil error and nil auth token
		}
		return nil, err // Return nil error as it could be that token is not created yet
	}

	return authToken, nil // Return nil error as it could be that token is not created yet
}

// GetUserInfoFromCookie retrieves user information from the "userinfo" cookie.
func GetUserInfoFromCookie(r *http.Request) (*user.User, error) {
	userCookie, err := r.Cookie("userinfo")
	if err != nil {
		return nil, fmt.Errorf("user not authenticated")
	}

	decodedValue, err := url.QueryUnescape(userCookie.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to URL unescape user info: %v", err)
	}

	var user user.User
	err = json.Unmarshal([]byte(decodedValue), &user)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal user info: %v", err)
	}

	return &user, nil
}

// IsSessionValid checks if the user session is valid.
func IsSessionValid(r *http.Request) bool {
	_, err := r.Cookie("userinfo")
	return err == nil
}

// IsValidEmail checks if a string is a valid email address
func IsValidEmail(email string) bool {
	// Use a regular expression to validate the email format
	emailRegex := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
	re := regexp.MustCompile(emailRegex)
	return re.MatchString(email)
}
